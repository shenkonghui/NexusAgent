package acp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/coder/acp-go-sdk"

	"nexusagent/internal/logging"
)

// Connection 封装 acp.ClientSideConnection，管理一个 agent 进程与一条 ACP 连接。
//
// 一条 Connection 可承载多个 ACP session（多路复用同一 agent 进程），
// 各 session 的 update 通过 Client 按 SessionId 路由分发。
type Connection struct {
	conn    *acp.ClientSideConnection
	process *Process
	client  *Client
}

// NewConnection 启动 agent 进程并建立 ACP 连接。
func NewConnection(backend Backend, workDir string) (*Connection, error) {
	proc, err := NewProcess(backend, workDir)
	if err != nil {
		return nil, err
	}

	client := NewClient()
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
	slog.Debug("ACP initialize")
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
	slog.Debug("ACP initialize 完成", "protocol", resp.ProtocolVersion, "auth_methods", len(resp.AuthMethods))
	return resp, nil
}

// authMethodID 从 Initialize 返回的认证方式中提取 methodId。
func authMethodID(m acp.AuthMethod) string {
	switch {
	case m.Agent != nil:
		return m.Agent.Id
	case m.EnvVar != nil:
		return m.EnvVar.Id
	case m.Terminal != nil:
		return m.Terminal.Id
	default:
		return ""
	}
}

// AuthenticateIfRequired 若 agent 在 initialize 中声明了 authMethods，则自动完成认证。
func (c *Connection) AuthenticateIfRequired(ctx context.Context, initResp acp.InitializeResponse) error {
	if len(initResp.AuthMethods) == 0 {
		return nil
	}
	methodID := authMethodID(initResp.AuthMethods[0])
	if methodID == "" {
		return fmt.Errorf("agent 未提供有效的认证方法 ID")
	}
	slog.Info("ACP authenticate", "method", methodID)
	if _, err := c.conn.Authenticate(ctx, acp.AuthenticateRequest{MethodId: methodID}); err != nil {
		return fmt.Errorf("ACP authenticate: %w", err)
	}
	slog.Info("ACP authenticate 完成", "method", methodID)
	return nil
}

// NewSession 在当前连接上创建新的 ACP session，返回 session ID、初始 config options 和初始 modes。
// additionalDirectories 为 ACP 额外可访问根目录（如 skills 目录），路径须为绝对路径。
// 同一 Connection 可多次调用，每次返回不同的 session ID。
func (c *Connection) NewSession(ctx context.Context, cwd string, additionalDirectories []string) (string, []acp.SessionConfigOption, []acp.SessionMode, error) {
	slog.Debug("ACP newSession", "cwd", cwd, "extra_dirs", len(additionalDirectories))
	resp, err := c.conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:                   cwd,
		AdditionalDirectories: additionalDirectories,
		McpServers:            []acp.McpServer{},
	})
	if err != nil {
		return "", nil, nil, fmt.Errorf("ACP newSession: %w", err)
	}

	var modes []acp.SessionMode
	if resp.Modes != nil {
		modes = resp.Modes.AvailableModes
	}
	slog.Debug("ACP newSession 完成", "session", resp.SessionId, "config_options", len(resp.ConfigOptions), "modes", len(modes))
	return string(resp.SessionId), resp.ConfigOptions, modes, nil
}

// Prompt 发送 prompt 并返回该 session 专属的流式 update channel。
// channel 在 prompt turn 完成后关闭（由内部 goroutine 注销 stream 时关闭）。
func (c *Connection) Prompt(ctx context.Context, sessionID, prompt string) (<-chan acp.SessionUpdate, error) {
	sid := acp.SessionId(sessionID)
	ch := c.client.RegisterStream(sid, 256)
	slog.Debug("ACP prompt", "session", sessionID, "chars", len(prompt), "preview", logging.Preview(prompt, 120))

	go func() {
		defer c.client.UnregisterStream(sid)
		_, err := c.conn.Prompt(ctx, acp.PromptRequest{
			SessionId: sid,
			Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
		})
		if err != nil {
			slog.Debug("ACP prompt 失败", "session", sessionID, "err", err)
		} else {
			slog.Debug("ACP prompt 完成", "session", sessionID)
		}
	}()

	return ch, nil
}

// Cancel 取消正在进行的 prompt。
func (c *Connection) Cancel(ctx context.Context, sessionID string) error {
	slog.Debug("ACP cancel", "session", sessionID)
	return c.conn.Cancel(ctx, acp.CancelNotification{SessionId: acp.SessionId(sessionID)})
}

// SetConfigOption 设置会话的 config option（如模型选择）。
func (c *Connection) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	slog.Debug("ACP setSessionConfigOption", "session", sessionID, "config", configID, "value", value)
	_, err := c.conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: acp.SessionId(sessionID),
			ConfigId:  acp.SessionConfigId(configID),
			Value:     acp.SessionConfigValueId(value),
		},
	})
	return err
}

// SetSessionMode 切换会话模式（如 ask / agent / edit）。
func (c *Connection) SetSessionMode(ctx context.Context, sessionID, modeID string) error {
	slog.Debug("ACP setSessionMode", "session", sessionID, "mode", modeID)
	_, err := c.conn.SetSessionMode(ctx, acp.SetSessionModeRequest{
		SessionId: acp.SessionId(sessionID),
		ModeId:    acp.SessionModeId(modeID),
	})
	return err
}

// CloseSessionByID 关闭单个 ACP session（释放 agent 侧该 session 的资源），
// 但不停止 agent 进程，连接可继续承载其他 session。
func (c *Connection) CloseSessionByID(ctx context.Context, sessionID string) error {
	slog.Debug("ACP closeSession", "session", sessionID)
	c.client.UnregisterStream(acp.SessionId(sessionID))
	_, err := c.conn.CloseSession(ctx, acp.CloseSessionRequest{
		SessionId: acp.SessionId(sessionID),
	})
	return err
}

// Done 返回连接关闭信号 channel。
// agent 进程退出或连接断开时该 channel 关闭。
func (c *Connection) Done() <-chan struct{} {
	return c.conn.Done()
}

// Close 关闭连接并停止 agent 进程。
// 用于彻底销毁该 Connection（不再承载任何 session）。
func (c *Connection) Close() error {
	return c.process.Stop()
}

// Client 返回内部 Client（用于测试）。
func (c *Connection) Client() *Client {
	return c.client
}
