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
		wsConfig: wsConfig,
	}
}

// RegisterBackend 注册一个 agent 后端。
func (s *Service) RegisterBackend(b Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends[b.Name()] = b
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
	backend, err := s.GetBackend(agentType)
	if err != nil {
		return nil, err
	}

	var ws *Workspace
	if cwd != "" {
		ws = NewExternalWorkspace(cwd)
	} else if s.wsConfig.DefaultMode == "temporary" {
		ws, err = NewTemporaryWorkspace(s.wsConfig.TempDirPrefix)
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

	sessionID, err := conn.NewSession(ctx, ws.Cwd)
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
	}
	if err := s.sessions.Create(session); err != nil {
		_ = conn.Close()
		_ = ws.Cleanup()
		return nil, fmt.Errorf("会话落库: %w", err)
	}

	s.mu.Lock()
	s.conns[sessionID] = conn
	s.mu.Unlock()

	return session, nil
}

// Prompt 向会话发送 prompt，返回流式 Message channel。
// 每条 SessionUpdate 会被映射为 models.Message 并持久化到数据库后转发给调用方。
func (s *Service) Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error) {
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
		_ = s.messages.Create(&userMsg)
		out <- userMsg

		// 读取 agent 返回的 update 流，逐条持久化并转发
		for u := range updates {
			seq++
			msg := MapUpdate(sessionID, session.ID, seq, u)
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
	s.mu.Unlock()

	if ok {
		_ = conn.Close()
	}

	ws := &Workspace{Mode: session.WorkspaceMode, TempDir: session.TempDir}
	_ = ws.Cleanup()

	now := time.Now()
	return s.sessions.UpdateStatus(session.ID, models.SessionStatusClosed, &now)
}

// ListSessions 列出指定用户的会话。
func (s *Service) ListSessions(userID uint) ([]models.Session, error) {
	return s.sessions.FindByUserID(userID)
}

// GetSession 查询会话。
func (s *Service) GetSession(sessionID string) (*models.Session, error) {
	sess, err := s.sessions.FindBySessionID(sessionID)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
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

// GetSessionByDBID 按数据库主键查询会话。
func (s *Service) GetSessionByDBID(id uint) (*models.Session, error) {
	sess, err := s.sessions.FindByID(id)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// ResumeSession 恢复已失效（error 状态）的会话：重建 ACP session、注入历史上下文、更新 session_id。
// active 且连接存在的会话直接返回；closed 会话返回错误。
func (s *Service) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error) {
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

	// closed → 不可恢复
	if session.Status == models.SessionStatusClosed {
		return nil, ErrSessionClosed
	}

	// error 状态 → 尝试恢复
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

	newSessionID, err := conn.NewSession(ctx, session.Cwd)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-创建 ACP 会话: %w", err)
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

	// 更新 session_id 和状态
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
