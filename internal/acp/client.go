package acp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/coder/acp-go-sdk"
)

// Client 实现 acp.Client 接口，自动批准权限请求并按 sessionID 路由 session update。
//
// 多 session 共享同一条 ACP 连接时，agent 推回的 SessionNotification 携带
// SessionId，Client 据此分发到对应 session 的 stream channel，
// 避免不同会话的流式输出互相串扰。
type Client struct {
	mu      sync.Mutex
	streams map[acp.SessionId]chan acp.SessionUpdate
}

// NewClient 创建一个新的 Client。
func NewClient() *Client {
	return &Client{
		streams: make(map[acp.SessionId]chan acp.SessionUpdate),
	}
}

// RegisterStream 为指定 session 注册一个 update stream，返回只读 channel。
// 同一 sessionID 重复注册会覆盖旧 stream（旧 stream 不会被关闭，调用方需自行处理）。
func (c *Client) RegisterStream(sessionID acp.SessionId, bufSize int) chan acp.SessionUpdate {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan acp.SessionUpdate, bufSize)
	c.streams[sessionID] = ch
	return ch
}

// UnregisterStream 注销并关闭指定 session 的 stream。
// 后续该 session 的 update 会被安全丢弃。
func (c *Client) UnregisterStream(sessionID acp.SessionId) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ch, ok := c.streams[sessionID]; ok {
		delete(c.streams, sessionID)
		// 安全关闭：仅在没有其他持有者时关闭。
		// 由于 UnregisterStream 只在 prompt goroutine 结束时调用，且 stream 仅由该 goroutine 持有，
		// 关闭是安全的。
		close(ch)
	}
}

// RequestPermission 自动批准所有权限请求。
func (c *Client) RequestPermission(ctx context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	for _, o := range params.Options {
		if o.Kind == acp.PermissionOptionKindAllowOnce || o.Kind == acp.PermissionOptionKindAllowAlways {
			return acp.RequestPermissionResponse{
				Outcome: acp.RequestPermissionOutcome{
					Selected: &acp.RequestPermissionOutcomeSelected{OptionId: o.OptionId},
				},
			}, nil
		}
	}
	if len(params.Options) > 0 {
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Selected: &acp.RequestPermissionOutcomeSelected{OptionId: params.Options[0].OptionId},
			},
		}, nil
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
	}, nil
}

// SessionUpdate 将 update 按 SessionId 路由到对应 session 的 stream。
// 若该 session 未注册 stream（未在 prompt 中或已关闭），update 被安全丢弃。
func (c *Client) SessionUpdate(ctx context.Context, params acp.SessionNotification) error {
	c.mu.Lock()
	ch, ok := c.streams[params.SessionId]
	c.mu.Unlock()
	if !ok {
		return nil
	}
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
