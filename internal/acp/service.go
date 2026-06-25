package acp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
	"gorm.io/gorm"

	"nexusagent/internal/config"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

var (
	ErrBackendNotFound  = errors.New("后端未注册")
	ErrSessionNotFound  = errors.New("会话不存在")
	ErrSessionNotActive = errors.New("会话不在活跃状态")
	ErrSessionClosed    = errors.New("会话已关闭，无法恢复")
)

// Service 是 ACP 客户端高层服务，串联后端、连接、工作区与持久化。
type Service struct {
	sessions *repository.SessionRepository
	messages *repository.MessageRepository
	backends map[string]Backend
	conns    map[string]*Connection
	commands map[string][]acp.AvailableCommand
	configs  map[string][]acp.SessionConfigOption
	modes    map[string][]acp.SessionMode
	mu       sync.RWMutex
	wsConfig config.WorkspaceConfig
}

// NewService 创建新的 Service。
func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig) *Service {
	return &Service{
		sessions: repository.NewSessionRepository(db),
		messages: repository.NewMessageRepository(db),
		backends: make(map[string]Backend),
		conns:    make(map[string]*Connection),
		commands: make(map[string][]acp.AvailableCommand),
		configs:  make(map[string][]acp.SessionConfigOption),
		modes:    make(map[string][]acp.SessionMode),
		wsConfig: wsConfig,
	}
}

// RegisterBackend 注册一个 agent 后端。
func (s *Service) RegisterBackend(b Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends[b.Name()] = b
}

// ReplaceBackend 注册或覆盖一个 agent 后端（用于动态更新配置）。
func (s *Service) ReplaceBackend(b Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends[b.Name()] = b
}

// UnregisterBackend 注销一个 agent 后端。
func (s *Service) UnregisterBackend(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.backends, name)
}

// GetBackend 查找已注册的后端。
func (s *Service) GetBackend(name string) (Backend, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.backends[name]
	if !ok {
		return nil, ErrBackendNotFound
	}
	return b, nil
}

// CreateSession 创建新的 ACP 会话。
// cwd 为空时根据配置自动创建临时工作区。
func (s *Service) CreateSession(ctx context.Context, agentType, cwd string, userID uint) (*models.Session, error) {
	return s.CreateSessionWithSource(ctx, agentType, cwd, userID, models.SessionSourceManual)
}

// CreateSessionWithSource 创建会话并指定来源（manual/scheduled）。
func (s *Service) CreateSessionWithSource(ctx context.Context, agentType, cwd string, userID uint, source string) (*models.Session, error) {
	backend, err := s.GetBackend(agentType)
	if err != nil {
		return nil, err
	}

	var ws *Workspace
	if cwd != "" {
		ws = NewExternalWorkspace(cwd)
	} else if s.wsConfig.DefaultMode == "temporary" {
		ws, err = NewTemporaryWorkspace(s.wsConfig.SessionDir, s.wsConfig.TempDirPrefix)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("external 模式必须提供 cwd")
	}

	conn, err := NewConnection(backend)
	if err != nil {
		_ = ws.Cleanup()
		return nil, fmt.Errorf("建立连接: %w", err)
	}

	if _, err := conn.Initialize(ctx); err != nil {
		_ = conn.Close()
		_ = ws.Cleanup()
		return nil, fmt.Errorf("ACP 握手: %w", err)
	}

	sessionID, configOptions, modes, err := conn.NewSession(ctx, ws.Cwd)
	if err != nil {
		_ = conn.Close()
		_ = ws.Cleanup()
		return nil, fmt.Errorf("创建 ACP 会话: %w", err)
	}

	session := &models.Session{
		SessionID:     sessionID,
		AgentType:     agentType,
		Cwd:           ws.Cwd,
		Status:        models.SessionStatusActive,
		UserID:        userID,
		WorkspaceMode: ws.Mode,
		TempDir:       ws.TempDir,
		Source:        source,
	}
	if err := s.sessions.Create(session); err != nil {
		_ = conn.Close()
		_ = ws.Cleanup()
		return nil, fmt.Errorf("会话落库: %w", err)
	}

	s.mu.Lock()
	s.conns[sessionID] = conn
	if len(configOptions) > 0 {
		s.configs[sessionID] = configOptions
	}
	if len(modes) > 0 {
		s.modes[sessionID] = modes
	}
	s.mu.Unlock()

	return session, nil
}

// Prompt 向会话发送 prompt，返回流式 Message channel。
// 每条 SessionUpdate 会被映射为 models.Message 并持久化到数据库后转发给调用方。
func (s *Service) Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error) {
	return s.PromptWithExecution(ctx, sessionID, prompt, nil)
}

// PromptWithExecution 与 Prompt 相同，并为本次执行的所有消息写入 executionID。
// executionID 为 nil 时表示手动会话（不标记执行块）。
func (s *Service) PromptWithExecution(ctx context.Context, sessionID, prompt string, executionID *uint) (<-chan models.Message, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.SessionStatusActive {
		return nil, ErrSessionNotActive
	}

	s.mu.RLock()
	conn, ok := s.conns[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotActive
	}

	updates, err := conn.Prompt(ctx, sessionID, prompt)
	if err != nil {
		return nil, err
	}

	_ = s.sessions.UpdateLastPrompt(session.ID, prompt)

	// 首次对话时从 prompt 提取标题（仅当 title 为空时设置）
	if session.Title == "" {
		title := extractTitle(prompt)
		if title != "" {
			_ = s.sessions.UpdateTitle(session.ID, title)
		}
	}

	out := make(chan models.Message, 256)
	go func() {
		defer close(out)

		seq := s.getNextSequence(session.ID)

		// 持久化用户发送的 prompt 作为 user_message_chunk
		seq++
		userUpdate := acp.SessionUpdate{
			UserMessageChunk: &acp.SessionUpdateUserMessageChunk{
				Content: acp.ContentBlock{
					Text: &acp.ContentBlockText{Text: prompt, Type: "text"},
				},
				SessionUpdate: "user_message_chunk",
			},
		}
		userMsg := MapUpdate(sessionID, session.ID, seq, userUpdate)
		userMsg.ExecutionID = executionID
		_ = s.messages.Create(&userMsg)
		out <- userMsg

		// 读取 agent 返回的 update 流，逐条持久化并转发
		for u := range updates {
			s.captureCommands(sessionID, u)
			seq++
			msg := MapUpdate(sessionID, session.ID, seq, u)
			msg.ExecutionID = executionID
			_ = s.messages.Create(&msg)
			out <- msg
		}
	}()

	return out, nil
}

// CancelSession 取消正在进行的 prompt。
func (s *Service) CancelSession(ctx context.Context, sessionID string) error {
	s.mu.RLock()
	conn, ok := s.conns[sessionID]
	s.mu.RUnlock()
	if !ok {
		return ErrSessionNotFound
	}
	return conn.Cancel(ctx, sessionID)
}

// CloseSession 关闭会话，释放连接并清理工作区。
func (s *Service) CloseSession(ctx context.Context, sessionID string) error {
	_ = ctx // 保留参数，未来可能用于超时控制

	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	conn, ok := s.conns[sessionID]
	if ok {
		delete(s.conns, sessionID)
	}
	delete(s.commands, sessionID)
	delete(s.configs, sessionID)
	delete(s.modes, sessionID)
	s.mu.Unlock()

	if ok {
		_ = conn.Close()
	}

	ws := &Workspace{Mode: session.WorkspaceMode, TempDir: session.TempDir}
	_ = ws.Cleanup()

	now := time.Now()
	return s.sessions.UpdateStatus(session.ID, models.SessionStatusClosed, &now)
}

// DeleteSession 彻底删除会话：释放连接、清理工作区、删除消息与 会话记录。
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	_ = ctx

	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	conn, ok := s.conns[sessionID]
	if ok {
		delete(s.conns, sessionID)
	}
	delete(s.commands, sessionID)
	delete(s.configs, sessionID)
	delete(s.modes, sessionID)
	s.mu.Unlock()

	if ok {
		_ = conn.Close()
	}

	ws := &Workspace{Mode: session.WorkspaceMode, TempDir: session.TempDir}
	_ = ws.Cleanup()

	// 先删消息再删会话，避免孤儿消息
	if err := s.messages.DeleteByDBSessionID(session.ID); err != nil {
		return fmt.Errorf("删除会话消息: %w", err)
	}
	if err := s.sessions.Delete(session.ID); err != nil {
		return fmt.Errorf("删除会话记录: %w", err)
	}
	return nil
}

// ListSessions 列出指定用户的会话。
func (s *Service) ListSessions(userID uint) ([]models.Session, error) {
	return s.sessions.FindByUserID(userID)
}

// ListSessionsBySource 列出指定用户指定来源的会话。source 为空时返回全部。
func (s *Service) ListSessionsBySource(userID uint, source string) ([]models.Session, error) {
	return s.sessions.FindByUserIDAndSource(userID, source)
}

// ListExecutions 返回指定会话的定时执行块聚合（按 started_at 降序）。
func (s *Service) ListExecutions(sessionID string) ([]repository.ExecutionAggregate, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return s.messages.AggregateExecutions(session.ID)
}

// NextExecutionID 返回指定会话下一个可用的 execution_id（当前最大值 + 1）。
func (s *Service) NextExecutionID(sessionID string) (uint, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return 0, err
	}
	max, err := s.messages.MaxExecutionID(session.ID)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

// GetSession 查询会话。
func (s *Service) GetSession(sessionID string) (*models.Session, error) {
	sess, err := s.sessions.FindBySessionID(sessionID)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// UpdateTitle 更新会话标题。
func (s *Service) UpdateTitle(dbSessionID uint, title string) error {
	return s.sessions.UpdateTitle(dbSessionID, title)
}

// RecoverActiveSessions 在服务启动时调用，将所有 active 状态的会话标记为 error。
func (s *Service) RecoverActiveSessions() {
	_ = s.sessions.MarkActiveAsError()
}

// getNextSequence 获取指定会话当前最大 sequence 值（无消息时返回 0）。
func (s *Service) getNextSequence(dbSessionID uint) int {
	max, err := s.messages.MaxSequence(dbSessionID)
	if err != nil {
		return 0
	}
	return max
}

// ListMessages 查询会话的完整消息历史，按 sequence 升序返回。
func (s *Service) ListMessages(sessionID string) ([]models.Message, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return s.messages.FindByDBSessionID(session.ID)
}

// captureCommands 从 SessionUpdate 中提取 AvailableCommandsUpdate、ConfigOptionUpdate 和 CurrentModeUpdate 并缓存到会话。
func (s *Service) captureCommands(sessionID string, u acp.SessionUpdate) {
	if u.AvailableCommandsUpdate != nil {
		cmds := u.AvailableCommandsUpdate.AvailableCommands
		s.mu.Lock()
		s.commands[sessionID] = cmds
		s.mu.Unlock()
	}
	if u.ConfigOptionUpdate != nil {
		opts := u.ConfigOptionUpdate.ConfigOptions
		s.mu.Lock()
		s.configs[sessionID] = opts
		s.mu.Unlock()
	}
	// CurrentModeUpdate 仅更新当前选中的 mode ID，可用 mode 列表不变，无需重新缓存
}

// ListCommands 返回会话缓存的可用 slash command 列表。
func (s *Service) ListCommands(sessionID string) ([]acp.AvailableCommand, error) {
	if _, err := s.GetSession(sessionID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cmds := s.commands[sessionID]
	out := make([]acp.AvailableCommand, len(cmds))
	copy(out, cmds)
	return out, nil
}

// ListConfigOptions 返回会话缓存的 config option 列表（含模型选择等）。
func (s *Service) ListConfigOptions(sessionID string) ([]acp.SessionConfigOption, error) {
	if _, err := s.GetSession(sessionID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	opts := s.configs[sessionID]
	out := make([]acp.SessionConfigOption, len(opts))
	copy(out, opts)
	return out, nil
}

// CachedModelOptions 返回指定 agent 类型的可用模型 config option（从已有会话缓存获取）。
// 仅返回 category=model 的 select 类型 option。若该 agent 类型尚无会话缓存则返回 nil。
func (s *Service) CachedModelOptions(agentType string) []acp.SessionConfigOption {
	s.mu.RLock()
	sessionIDs := make([]string, 0, len(s.configs))
	for sid := range s.configs {
		sessionIDs = append(sessionIDs, sid)
	}
	s.mu.RUnlock()

	for _, sid := range sessionIDs {
		sess, err := s.GetSession(sid)
		if err != nil || sess.AgentType != agentType {
			continue
		}
		s.mu.RLock()
		opts := s.configs[sid]
		s.mu.RUnlock()
		for _, opt := range opts {
			if opt.Select == nil || opt.Select.Category == nil {
				continue
			}
			if string(*opt.Select.Category) == "model" {
				return []acp.SessionConfigOption{opt}
			}
		}
	}
	return nil
}

// ProbeConfigOptions 创建一个临时会话探测指定 agent 类型的 config options，随后删除该会话。
// 用于在不保留会话的情况下获取该 agent 支持的模型及其他配置。cwd 为空时使用临时工作区。
func (s *Service) ProbeConfigOptions(ctx context.Context, agentType string, userID uint) ([]acp.SessionConfigOption, error) {
	sess, err := s.CreateSessionWithSource(ctx, agentType, "", userID, models.SessionSourceManual)
	if err != nil {
		return nil, fmt.Errorf("创建探测会话: %w", err)
	}
	opts, err := s.ListConfigOptions(sess.SessionID)
	if err != nil {
		_ = s.DeleteSession(ctx, sess.SessionID)
		return nil, fmt.Errorf("查询探测会话配置: %w", err)
	}
	// 复制一份再删除，避免删除后引用被清空的缓存
	out := make([]acp.SessionConfigOption, len(opts))
	copy(out, opts)
	if err := s.DeleteSession(ctx, sess.SessionID); err != nil {
		// 删除失败不致命，仅记录
		_ = err
	}
	return out, nil
}

// ListModes 返回会话可用的 mode 列表（agent skill/模式，如 plan/act）。
func (s *Service) ListModes(sessionID string) ([]acp.SessionMode, error) {
	if _, err := s.GetSession(sessionID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	modes := s.modes[sessionID]
	out := make([]acp.SessionMode, len(modes))
	copy(out, modes)
	return out, nil
}

// ListSkills 扫描会话工作目录和用户主目录下的 Agent Skills（agentskills.io 规范）。
func (s *Service) ListSkills(sessionID string) ([]Skill, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return ScanSkills(session.Cwd), nil
}

// SetConfigOption 设置会话的 config option 值（如切换模型）。
func (s *Service) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	if _, err := s.GetSession(sessionID); err != nil {
		return err
	}
	s.mu.RLock()
	conn, ok := s.conns[sessionID]
	s.mu.RUnlock()
	if !ok {
		return ErrSessionNotActive
	}
	return conn.SetConfigOption(ctx, sessionID, configID, value)
}

// GetSessionByDBID 按数据库主键查询会话。
func (s *Service) GetSessionByDBID(id uint) (*models.Session, error) {
	sess, err := s.sessions.FindByID(id)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// ResumeSession 恢复或重开会话：重建 ACP session、注入历史上下文、更新 session_id。
// active 且连接存在的会话直接返回；error 与 closed 状态均会尝试恢复。
// cwdOverride 非空时使用该目录作为新工作区（用于已关闭会话的临时目录被清理的场景）。
func (s *Service) ResumeSession(ctx context.Context, sessionID, cwdOverride string) (*models.Session, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// active 且连接存在 → 直接返回
	if session.Status == models.SessionStatusActive {
		s.mu.RLock()
		_, ok := s.conns[sessionID]
		s.mu.RUnlock()
		if ok {
			return session, nil
		}
	}

	// 确定工作目录：优先使用覆盖值，否则复用原 cwd
	cwd := session.Cwd
	if cwdOverride != "" {
		cwd = cwdOverride
	}
	if cwd == "" {
		return nil, errors.New("恢复会话需要工作目录，请提供 cwd")
	}
	if !dirExists(cwd) {
		return nil, fmt.Errorf("工作目录不存在: %s，请提供有效的 cwd", cwd)
	}

	backend, err := s.GetBackend(session.AgentType)
	if err != nil {
		return nil, err
	}

	conn, err := NewConnection(backend)
	if err != nil {
		return nil, fmt.Errorf("恢复会话-建立连接: %w", err)
	}

	if _, err := conn.Initialize(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-ACP 握手: %w", err)
	}

	newSessionID, configOptions, modes, err := conn.NewSession(ctx, cwd)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-创建 ACP 会话: %w", err)
	}

	// 若提供了 cwd 覆盖，更新会话工作区为 external 模式
	if cwdOverride != "" && cwdOverride != session.Cwd {
		_ = s.sessions.UpdateWorkspace(session.ID, cwdOverride, models.WorkspaceModeExternal, "")
		session.Cwd = cwdOverride
		session.WorkspaceMode = models.WorkspaceModeExternal
		session.TempDir = ""
	}

	// 查询历史消息并注入上下文
	history, _ := s.messages.FindByDBSessionID(session.ID)
	contextText := formatHistory(history)
	if contextText != "" {
		// 异步注入历史上下文，不等结果
		go func() {
			_, _ = conn.Prompt(ctx, newSessionID, contextText)
		}()
	}

	// 更新 session_id 和状态（closed_at 置空）
	if err := s.sessions.UpdateSessionID(session.ID, newSessionID); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-更新 session_id: %w", err)
	}
	if err := s.sessions.UpdateStatus(session.ID, models.SessionStatusActive, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-更新状态: %w", err)
	}

	// 存入连接池
	s.mu.Lock()
	s.conns[newSessionID] = conn
	delete(s.commands, sessionID)
	delete(s.configs, sessionID)
	delete(s.modes, sessionID)
	if len(configOptions) > 0 {
		s.configs[newSessionID] = configOptions
	}
	if len(modes) > 0 {
		s.modes[newSessionID] = modes
	}
	s.mu.Unlock()

	// 返回更新后的 session
	return s.sessions.FindByID(session.ID)
}

// formatHistory 将历史消息格式化为对话上下文文本，最多取最近 50 条。
func formatHistory(messages []models.Message) string {
	if len(messages) == 0 {
		return ""
	}

	// 最多取最近 50 条
	const limit = 50
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	var sb strings.Builder
	sb.WriteString("以下是之前对话的历史记录，请基于这些上下文继续对话：\n\n")
	for _, m := range messages {
		switch m.Role {
		case models.MessageRoleUser:
			sb.WriteString("[User]: " + m.Content + "\n")
		case models.MessageRoleAssistant:
			sb.WriteString("[Assistant]: " + m.Content + "\n")
		case models.MessageRoleTool:
			sb.WriteString("[Tool]: " + m.Content + "\n")
		}
	}
	return sb.String()
}

// extractTitle 从用户 prompt 中提取会话标题。
// 取首行非空文本的前 30 个字符（rune），去除首尾空白和命令前缀。
const maxTitleLen = 30

func extractTitle(prompt string) string {
	// 去除首尾空白
	s := strings.TrimSpace(prompt)
	if s == "" {
		return ""
	}
	// 取首行（多行 prompt 只用第一行）
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	// 去除常见的 slash 命令前缀（如 /help、/plan 等）
	if strings.HasPrefix(s, "/") {
		// 跳过命令部分，取命令后的文本
		if idx := strings.IndexByte(s, ' '); idx >= 0 {
			s = strings.TrimSpace(s[idx+1:])
		}
	}
	if s == "" {
		return ""
	}
	// 按 rune 截断，避免截断多字节字符
	runes := []rune(s)
	if len(runes) > maxTitleLen {
		runes = runes[:maxTitleLen]
		return string(runes) + "..."
	}
	return string(runes)
}
