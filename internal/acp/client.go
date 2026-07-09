package acp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/coder/acp-go-sdk"

	"nexusagent/internal/logging"
)

// subscriber 表示一个会话 update 流的订阅者，每个订阅者拥有独立的 buffered channel。
type subscriber struct {
	ch chan acp.SessionUpdate
}

// Client 实现 acp.Client 接口，处理权限交互并按 sessionID 路由 session update。
// streams 采用 fan-out 设计：每个会话可有多个订阅者，支持多客户端同时监听（如断点续传重连）。
type Client struct {
	mu      sync.RWMutex
	streams map[acp.SessionId]map[*subscriber]struct{}
	perm    *permissionBroker
}

// NewClient 创建一个新的 Client。
func NewClient() *Client {
	return &Client{
		streams: make(map[acp.SessionId]map[*subscriber]struct{}),
		perm:    newPermissionBroker(),
	}
}

// Subscribe 为指定 session 注册一个新订阅者，返回该订阅者。
// 多次调用会创建多个独立订阅者，SessionUpdate 会分发给所有订阅者。
func (c *Client) Subscribe(sessionID acp.SessionId, bufSize int) *subscriber {
	c.mu.Lock()
	defer c.mu.Unlock()
	sub := &subscriber{ch: make(chan acp.SessionUpdate, bufSize)}
	if c.streams[sessionID] == nil {
		c.streams[sessionID] = make(map[*subscriber]struct{})
	}
	c.streams[sessionID][sub] = struct{}{}
	return sub
}

// Unsubscribe 移除并关闭单个订阅者（不影响该会话的其他订阅者）。
func (c *Client) Unsubscribe(sessionID acp.SessionId, sub *subscriber) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if subs, ok := c.streams[sessionID]; ok {
		if _, exists := subs[sub]; exists {
			delete(subs, sub)
			close(sub.ch)
		}
		if len(subs) == 0 {
			delete(c.streams, sessionID)
		}
	}
}

// UnsubscribeAll 移除并关闭指定 session 的全部订阅者（会话关闭时调用）。
func (c *Client) UnsubscribeAll(sessionID acp.SessionId) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if subs, ok := c.streams[sessionID]; ok {
		for sub := range subs {
			close(sub.ch)
		}
		delete(c.streams, sessionID)
	}
}

// RegisterStream 为指定 session 注册一个 update stream，返回只读 channel。
// 兼容接口：内部转为单订阅者，新代码应直接使用 Subscribe。
func (c *Client) RegisterStream(sessionID acp.SessionId, bufSize int) chan acp.SessionUpdate {
	return c.Subscribe(sessionID, bufSize).ch
}

// UnregisterStream 注销并关闭指定 session 的全部 stream。
// 兼容接口：语义为清空该会话所有订阅者。
func (c *Client) UnregisterStream(sessionID acp.SessionId) {
	c.UnsubscribeAll(sessionID)
}

// RegisterPermissionWaiter 注册权限请求监听（Prompt 流期间调用）。
func (c *Client) RegisterPermissionWaiter(sessionID acp.SessionId) chan PermissionNotify {
	return c.perm.registerWaiter(sessionID)
}

// UnregisterPermissionWaiter 注销权限请求监听。
func (c *Client) UnregisterPermissionWaiter(sessionID acp.SessionId) {
	c.perm.unregisterWaiter(sessionID)
}

// RespondPermission 提交用户对权限请求的响应。
func (c *Client) RespondPermission(requestID, optionID string, cancelled bool) error {
	return c.perm.respond(requestID, optionID, cancelled)
}

// CancelPermissions 取消 session 所有挂起的权限请求。
func (c *Client) CancelPermissions(sessionID acp.SessionId) {
	c.perm.cancelSession(sessionID)
}

// RequestPermission 等待用户在前端选择权限选项；无活跃 Prompt 流时自动批准。
func (c *Client) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	toolTitle := ""
	if params.ToolCall.Title != nil {
		toolTitle = *params.ToolCall.Title
	}
	slog.Debug("ACP requestPermission", "session", params.SessionId, "tool", toolTitle)
	return c.perm.request(ctx, params)
}

// SessionUpdate 将 update 按 SessionId 分发给该会话的全部订阅者。
// 对 buffer 满的慢订阅者采用非阻塞丢弃（记日志），避免拖慢其他订阅者。
func (c *Client) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	c.mu.RLock()
	subs := c.streams[params.SessionId]
	// 复制订阅者集合，避免持锁发送
	snapshot := make([]*subscriber, 0, len(subs))
	for sub := range subs {
		snapshot = append(snapshot, sub)
	}
	c.mu.RUnlock()

	kind, _ := extractKindRole(params.Update)
	content := extractContent(params.Update)
	slog.Debug("ACP sessionUpdate", "session", params.SessionId, "kind", kind, "content_len", len(content), "preview", logging.Preview(content, 80), "subscribers", len(snapshot))

	for _, sub := range snapshot {
		select {
		case sub.ch <- params.Update:
		default:
			// 慢订阅者 buffer 满，丢弃本条避免阻塞其他订阅者
			slog.Warn("ACP sessionUpdate 订阅者 buffer 满，丢弃消息", "session", params.SessionId, "kind", kind)
		}
	}
	return nil
}

// WriteTextFile 将文件写入工作区。
func (c *Client) WriteTextFile(ctx context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	if !filepath.IsAbs(params.Path) {
		return acp.WriteTextFileResponse{}, fmt.Errorf("path must be absolute: %s", params.Path)
	}
	dir := filepath.Dir(params.Path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return acp.WriteTextFileResponse{}, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0o644); err != nil {
		return acp.WriteTextFileResponse{}, fmt.Errorf("write %s: %w", params.Path, err)
	}
	return acp.WriteTextFileResponse{}, nil
}

// ReadTextFile 从工作区读取文件。
func (c *Client) ReadTextFile(ctx context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	if !filepath.IsAbs(params.Path) {
		return acp.ReadTextFileResponse{}, fmt.Errorf("path must be absolute: %s", params.Path)
	}
	b, err := os.ReadFile(params.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, fmt.Errorf("read %s: %w", params.Path, err)
	}
	return acp.ReadTextFileResponse{Content: string(b)}, nil
}

// CreateTerminal 暂不实现，返回 no-op。
func (c *Client) CreateTerminal(ctx context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, nil
}

// TerminalOutput 暂不实现，返回 no-op。
func (c *Client) TerminalOutput(ctx context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, nil
}

// ReleaseTerminal 暂不实现，返回 no-op。
func (c *Client) ReleaseTerminal(ctx context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, nil
}

// WaitForTerminalExit 暂不实现，返回 no-op。
func (c *Client) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, nil
}

// KillTerminal 暂不实现，返回 no-op。
func (c *Client) KillTerminal(ctx context.Context, params acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, nil
}
