package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/coder/acp-go-sdk"

	"opennexus/internal/models"
)

var ErrPermissionNotFound = errors.New("权限请求不存在或已过期")

// PermissionNotify 推送给 Prompt 流的权限请求事件。
type PermissionNotify struct {
	RequestID string
	Request   acp.RequestPermissionRequest
}

type pendingPermission struct {
	sessionID acp.SessionId
	respCh    chan acp.RequestPermissionResponse
}

// permissionBroker 管理 ACP 权限请求与用户响应的桥接。
type permissionBroker struct {
	mu      sync.Mutex
	seq     atomic.Uint64
	waiters map[acp.SessionId]chan PermissionNotify
	pending map[string]pendingPermission
}

func newPermissionBroker() *permissionBroker {
	return &permissionBroker{
		waiters: make(map[acp.SessionId]chan PermissionNotify),
		pending: make(map[string]pendingPermission),
	}
}

func (b *permissionBroker) registerWaiter(sessionID acp.SessionId) chan PermissionNotify {
	ch := make(chan PermissionNotify, 64)
	b.mu.Lock()
	b.waiters[sessionID] = ch
	b.mu.Unlock()
	return ch
}

func (b *permissionBroker) unregisterWaiter(sessionID acp.SessionId) {
	b.mu.Lock()
	ch, ok := b.waiters[sessionID]
	if ok {
		delete(b.waiters, sessionID)
	}
	b.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (b *permissionBroker) cancelSession(sessionID acp.SessionId) {
	cancelled := acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
	}
	b.mu.Lock()
	for id, item := range b.pending {
		if item.sessionID != sessionID {
			continue
		}
		select {
		case item.respCh <- cancelled:
		default:
		}
		delete(b.pending, id)
	}
	b.mu.Unlock()
}

func (b *permissionBroker) request(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	// 内置 opennexus-notes MCP 由本服务注入，自动放行，避免并行 list_notes 把 UI 卡死。
	if isTrustedMCPTool(params) {
		return autoApprovePermission(params), nil
	}

	requestID := fmt.Sprintf("perm-%d", b.seq.Add(1))
	respCh := make(chan acp.RequestPermissionResponse, 1)

	b.mu.Lock()
	b.pending[requestID] = pendingPermission{sessionID: params.SessionId, respCh: respCh}
	waiter := b.waiters[params.SessionId]
	b.mu.Unlock()

	if waiter != nil {
		select {
		case waiter <- PermissionNotify{RequestID: requestID, Request: params}:
		case <-ctx.Done():
			b.removePending(requestID)
			return acp.RequestPermissionResponse{
				Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
			}, nil
		}
	} else {
		b.removePending(requestID)
		return autoApprovePermission(params), nil
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		b.removePending(requestID)
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
		}, nil
	}
}

func (b *permissionBroker) respond(requestID, optionID string, cancelled bool) error {
	b.mu.Lock()
	item, ok := b.pending[requestID]
	if !ok {
		b.mu.Unlock()
		return ErrPermissionNotFound
	}
	delete(b.pending, requestID)
	// allow-always：同会话其余挂起权限一并放行，避免并行工具调用逐条卡死。
	var batch []pendingPermission
	if !cancelled && optionID == "allow-always" {
		for id, p := range b.pending {
			if p.sessionID != item.sessionID {
				continue
			}
			batch = append(batch, p)
			delete(b.pending, id)
		}
	}
	b.mu.Unlock()

	send := func(p pendingPermission) {
		if cancelled {
			p.respCh <- acp.RequestPermissionResponse{
				Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
			}
			return
		}
		p.respCh <- acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Selected: &acp.RequestPermissionOutcomeSelected{OptionId: acp.PermissionOptionId(optionID)},
			},
		}
	}
	send(item)
	for _, p := range batch {
		send(p)
	}
	return nil
}

func isTrustedMCPTool(params acp.RequestPermissionRequest) bool {
	if params.ToolCall.Title == nil {
		return false
	}
	return strings.HasPrefix(*params.ToolCall.Title, "opennexus-notes")
}

func (b *permissionBroker) removePending(requestID string) {
	b.mu.Lock()
	delete(b.pending, requestID)
	b.mu.Unlock()
}

func autoApprovePermission(params acp.RequestPermissionRequest) acp.RequestPermissionResponse {
	for _, o := range params.Options {
		if o.Kind == acp.PermissionOptionKindAllowOnce || o.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.RequestPermissionOutcome{
					Selected: &acp.RequestPermissionOutcomeSelected{OptionId: o.OptionId},
				},
			}
		}
	}
	if len(params.Options) > 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Selected: &acp.RequestPermissionOutcomeSelected{OptionId: params.Options[0].OptionId},
			},
		}
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
	}
}

// MapPermissionRequest 将权限请求映射为 Message。
func MapPermissionRequest(sessionID string, dbSessionID uint, seq int, notify PermissionNotify) models.Message {
	payload := map[string]any{
		"request_id": notify.RequestID,
		"tool_call":  notify.Request.ToolCall,
		"options":    notify.Request.Options,
	}
	raw, _ := json.Marshal(payload)
	title := ""
	if notify.Request.ToolCall.Title != nil {
		title = *notify.Request.ToolCall.Title
	}
	return models.Message{
		SessionID:   sessionID,
		DBSessionID: dbSessionID,
		Role:        models.MessageRoleAssistant,
		Kind:        models.MessageKindPermissionRequest,
		Content:     title,
		RawJSON:     string(raw),
		Sequence:    seq,
	}
}
