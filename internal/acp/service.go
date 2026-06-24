package acp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"nexusagent/internal/config"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

var (
	ErrBackendNotFound  = errors.New("后端未注册")
	ErrSessionNotFound  = errors.New("会话不存在")
	ErrSessionNotActive = errors.New("会话不在活跃状态")
)

// Service 是 ACP 客户端高层服务，串联后端、连接、工作区与持久化。
type Service struct {
	sessions *repository.SessionRepository
	backends map[string]Backend
	conns    map[string]*Connection
	mu       sync.RWMutex
	wsConfig config.WorkspaceConfig
}

// NewService 创建新的 Service。
func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig) *Service {
	return &Service{
		sessions: repository.NewSessionRepository(db),
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

// Prompt 向会话发送 prompt，返回流式 update channel。
func (s *Service) Prompt(ctx context.Context, sessionID, prompt string) (<-chan interface{}, error) {
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

	out := make(chan interface{}, 256)
	go func() {
		defer close(out)
		for u := range updates {
			out <- u
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
