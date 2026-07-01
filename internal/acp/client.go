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

// Client 实现 acp.Client 接口，处理权限交互并按 sessionID 路由 session update。
type Client struct {
	mu      sync.Mutex
	streams map[acp.SessionId]chan acp.SessionUpdate
	perm    *permissionBroker
}

// NewClient 创建一个新的 Client。
func NewClient() *Client {
	return &Client{
		streams: make(map[acp.SessionId]chan acp.SessionUpdate),
		perm:    newPermissionBroker(),
	}
}

// RegisterStream 为指定 session 注册一个 update stream，返回只读 channel。
func (c *Client) RegisterStream(sessionID acp.SessionId, bufSize int) chan acp.SessionUpdate {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan acp.SessionUpdate, bufSize)
	c.streams[sessionID] = ch
	return ch
}

// UnregisterStream 注销并关闭指定 session 的 stream。
func (c *Client) UnregisterStream(sessionID acp.SessionId) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch, ok := c.streams[sessionID]; ok {
		delete(c.streams, sessionID)
		close(ch)
	}
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

// SessionUpdate 将 update 按 SessionId 路由到对应 session 的 stream。
func (c *Client) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	c.mu.Lock()
	ch, ok := c.streams[params.SessionId]
	c.mu.Unlock()
	if !ok {
		return nil
	}
	kind, _ := extractKindRole(params.Update)
	content := extractContent(params.Update)
	slog.Debug("ACP sessionUpdate", "session", params.SessionId, "kind", kind, "content_len", len(content), "preview", logging.Preview(content, 80))
	select {
	case ch <- params.Update:
	case <-ctx.Done():
		return ctx.Err()
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
