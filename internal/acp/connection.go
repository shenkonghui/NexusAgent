package acp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/coder/acp-go-sdk"
)

// Connection 封装 acp.ClientSideConnection，管理 agent 进程与 ACP 连接。
type Connection struct {
	conn    *acp.ClientSideConnection
	process *Process
	client  *Client
}

// NewConnection 启动 agent 进程并建立 ACP 连接。
func NewConnection(backend Backend) (*Connection, error) {
	proc, err := NewProcess(backend)
	if err != nil {
		return nil, err
	}

	client := NewClient(256)
	conn := acp.NewClientSideConnection(client, proc.Stdin(), proc.Stdout())
	conn.SetLogger(slog.Default())

	return &Connection{
		conn:    conn,
		process: proc,
		client:  client,
	}, nil
}

// Initialize 执行 ACP 握手。
func (c *Connection) Initialize(ctx context.Context) (acp.InitializeResponse, error) {
	resp, err := c.conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
		},
	})
	if err != nil {
		return acp.InitializeResponse{}, fmt.Errorf("ACP initialize: %w", err)
	}
	return resp, nil
}

// NewSession 创建新的 ACP 会话，返回 session ID、初始 config options 和初始 modes。
func (c *Connection) NewSession(ctx context.Context, cwd string) (string, []acp.SessionConfigOption, []acp.SessionMode, error) {
	c.client.Reset(256)

	resp, err := c.conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return "", nil, nil, fmt.Errorf("ACP newSession: %w", err)
	}

	var modes []acp.SessionMode
	if resp.Modes != nil {
		modes = resp.Modes.AvailableModes
	}
	return string(resp.SessionId), resp.ConfigOptions, modes, nil
}

// Prompt 发送 prompt 并返回流式 update channel。
// channel 在 prompt turn 完成后关闭。
func (c *Connection) Prompt(ctx context.Context, sessionID, prompt string) (<-chan acp.SessionUpdate, error) {
	c.client.Reset(256)

	go func() {
		defer c.client.CloseUpdates()
		_, _ = c.conn.Prompt(ctx, acp.PromptRequest{
			SessionId: acp.SessionId(sessionID),
			Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
		})
	}()

	return c.client.Updates(), nil
}

// Cancel 取消正在进行的 prompt。
func (c *Connection) Cancel(ctx context.Context, sessionID string) error {
	return c.conn.Cancel(ctx, acp.CancelNotification{SessionId: acp.SessionId(sessionID)})
}

// SetConfigOption 设置会话的 config option（如模型选择）。
func (c *Connection) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	_, err := c.conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: acp.SessionId(sessionID),
			ConfigId:  acp.SessionConfigId(configID),
			Value:     acp.SessionConfigValueId(value),
		},
	})
	return err
}

// Close 关闭连接并停止 agent 进程。
func (c *Connection) Close() error {
	return c.process.Stop()
}

// Client 返回内部 Client（用于测试）。
func (c *Connection) Client() *Client {
	return c.client
}
