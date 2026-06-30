package acp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// 连接状态常量。
const (
	connStateConnected    = "connected"
	connStateConnecting   = "connecting"
	connStateDisconnected = "disconnected"
)

// 健康检查与自动重连参数。
const (
	healthCheckInterval = 30 * time.Second // 健康检查周期
	reconnectBaseDelay  = 5 * time.Second  // 重连基础退避
	reconnectMaxDelay   = 60 * time.Second // 重连最大退避
)

// Service 是 ACP 客户端高层服务，串联后端、连接、工作区与持久化。
//
// 连接池模型：每个 agent 类型共享一条 ACP 连接（一个 agent 进程），
// 多个会话通过 SessionId 在同一条连接上多路复用。
// sessionAgent 维护 sessionID → agentType 的路由，用于定位所属连接。
//
// 连接生命周期由后台健康检查 goroutine 管理：
//   - 进程退出后标记 disconnected，定时任务自动重连
//   - 重连带指数退避，避免频繁重试
type Service struct {
	sessions *repository.SessionRepository
	messages *repository.MessageRepository
	backends map[string]Backend
	// pool 按 agentType 共享一条 Connection（一个 agent 进程承载多个 session）。
	pool map[string]*Connection
	// states 记录每个 agentType 的连接状态（connecting/connected/disconnected）。
	states map[string]string
	// sessionAgent 记录 sessionID → agentType 路由，用于定位所属连接。
	sessionAgent map[string]string
	commands     map[string][]acp.AvailableCommand
	configs      map[string][]acp.SessionConfigOption
	modes        map[string][]acp.SessionMode
	// probeCache 缓存探测结果，按 agentType 存储，避免重复创建临时会话探测。
	probeCache map[string][]acp.SessionConfigOption
	mu         sync.RWMutex
	wsConfig   config.WorkspaceConfig

	// 健康检查与自动重连控制
	hcCtx    context.Context
	hcCancel context.CancelFunc
	hcWG     sync.WaitGroup
}

// NewService 创建新的 Service。
func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig) *Service {
	return &Service{
		sessions:     repository.NewSessionRepository(db),
		messages:     repository.NewMessageRepository(db),
		backends:     make(map[string]Backend),
		pool:         make(map[string]*Connection),
		states:       make(map[string]string),
		sessionAgent: make(map[string]string),
		commands:     make(map[string][]acp.AvailableCommand),
		configs:      make(map[string][]acp.SessionConfigOption),
		modes:        make(map[string][]acp.SessionMode),
		probeCache:   make(map[string][]acp.SessionConfigOption),
		wsConfig:     wsConfig,
	}
}

// RegisterBackend 注册一个 agent 后端。
func (s *Service) RegisterBackend(b Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends[b.Name()] = b
}

// ReplaceBackend 注册或覆盖一个 agent 后端（用于动态更新配置）。
// 若该类型已有连接，先关闭旧连接（停止旧进程），下次使用时按新配置重建。
func (s *Service) ReplaceBackend(b Backend) {
	s.mu.Lock()
	oldConn, ok := s.pool[b.Name()]
	if ok {
		delete(s.pool, b.Name())
	}
	s.backends[b.Name()] = b
	s.states[b.Name()] = connStateDisconnected
	delete(s.probeCache, b.Name())
	s.mu.Unlock()

	if ok {
		_ = oldConn.Close()
	}
}

// UnregisterBackend 注销一个 agent 后端，并关闭对应的共享连接。
func (s *Service) UnregisterBackend(name string) {
	s.mu.Lock()
	oldConn, ok := s.pool[name]
	if ok {
		delete(s.pool, name)
	}
	delete(s.backends, name)
	delete(s.states, name)
	delete(s.probeCache, name)
	s.mu.Unlock()

	if ok {
		_ = oldConn.Close()
	}
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

// ensureConnection 获取或创建指定 agentType 的共享连接。
// 若池中连接已断开（Done() 已关闭），自动清理并重建。
// 状态流转：disconnected → connecting → connected（或 connecting → disconnected 失败）。
func (s *Service) ensureConnection(ctx context.Context, agentType string) (*Connection, error) {
	// 快速路径：池中有存活连接
	s.mu.RLock()
	conn, ok := s.pool[agentType]
	if ok {
		select {
		case <-conn.Done():
			conn = nil
		default:
			s.mu.RUnlock()
			return conn, nil
		}
	}
	s.mu.RUnlock()

	// 慢路径：需要建立连接。先抢锁设置 connecting 状态，避免并发重复建连。
	s.mu.Lock()
	// 再次检查：可能其他 goroutine 已建好
	if existing, ok2 := s.pool[agentType]; ok2 {
		select {
		case <-existing.Done():
		default:
			s.mu.Unlock()
			return existing, nil
		}
	}
	// 已有 connecting 状态的 goroutine 在跑？等待它完成（这里简单处理：直接返回错误让调用方重试）
	if s.states[agentType] == connStateConnecting {
		s.mu.Unlock()
		return nil, fmt.Errorf("agent %s 正在连接中，请稍后重试", agentType)
	}
	s.states[agentType] = connStateConnecting
	// 清理已断开的旧连接
	if oldConn, ok2 := s.pool[agentType]; ok2 {
		delete(s.pool, agentType)
		s.markAgentSessionsErrorLocked(agentType)
		_ = oldConn.Close()
	}
	s.mu.Unlock()

	// 在锁外执行耗时的进程启动与握手
	conn, err := s.buildConnection(ctx, agentType)
	if err != nil {
		s.mu.Lock()
		s.states[agentType] = connStateDisconnected
		s.mu.Unlock()
		return nil, err
	}

	s.mu.Lock()
	// 并发场景：可能已有其他 goroutine 先建好，复用之并关闭多余的
	if existing, ok2 := s.pool[agentType]; ok2 {
		select {
		case <-existing.Done():
		default:
			s.mu.Unlock()
			_ = conn.Close()
			return existing, nil
		}
	}
	s.pool[agentType] = conn
	s.states[agentType] = connStateConnected
	s.mu.Unlock()

	go s.watchConnection(agentType, conn)
	return conn, nil
}

// buildConnection 执行实际的进程启动与 ACP 握手（无锁，可阻塞）。
func (s *Service) buildConnection(ctx context.Context, agentType string) (*Connection, error) {
	backend, err := s.GetBackend(agentType)
	if err != nil {
		return nil, err
	}
	newConn, err := NewConnection(backend)
	if err != nil {
		return nil, fmt.Errorf("建立共享连接: %w", err)
	}
	if _, err := newConn.Initialize(ctx); err != nil {
		_ = newConn.Close()
		return nil, fmt.Errorf("ACP 握手: %w", err)
	}
	return newConn, nil
}

// watchConnection 监控共享连接，进程退出时标记 disconnected。
// 不立即将会话标记为 error——由健康检查任务自动重连，重连成功后会话可继续使用。
// 仅当重连持续失败超过阈值时，才将会话标记为 error。
func (s *Service) watchConnection(agentType string, conn *Connection) {
	<-conn.Done()
	s.logWarn("agent 进程退出，标记为 disconnected", agentType)

	s.mu.Lock()
	// 仅当池中仍是该连接时才清理（避免清理已被重建替换的新连接）
	if cur, ok := s.pool[agentType]; ok && cur == conn {
		delete(s.pool, agentType)
		s.states[agentType] = connStateDisconnected
		// 连接断开后清空探测缓存，重连时重新探测
		delete(s.probeCache, agentType)
	}
	s.mu.Unlock()
	// 健康检查 goroutine 会自动尝试重连
}

// markAgentSessionsErrorLocked 将指定 agentType 下所有活跃会话标记为 error，
// 并清理对应的 sessionAgent 路由。调用方需持有 s.mu 写锁。
func (s *Service) markAgentSessionsErrorLocked(agentType string) {
	for sid, at := range s.sessionAgent {
		if at != agentType {
			continue
		}
		delete(s.sessionAgent, sid)
		delete(s.commands, sid)
		delete(s.configs, sid)
		delete(s.modes, sid)
		if sess, err := s.sessions.FindBySessionID(sid); err == nil {
			_ = s.sessions.UpdateStatus(sess.ID, models.SessionStatusError, nil)
		}
	}
}

// PreconnectAllAsync 异步为所有已注册后端预建立共享连接。
// 每个 agent 独立 goroutine 连接，互不阻塞，立即返回。
// 连接失败的后端由健康检查任务自动重连。
func (s *Service) PreconnectAllAsync() {
	s.mu.RLock()
	types := make([]string, 0, len(s.backends))
	for name := range s.backends {
		types = append(types, name)
	}
	s.mu.RUnlock()

	for _, agentType := range types {
		go func(at string) {
			slog.Info("开始预连接 agent", "agent", at)
			if _, err := s.ensureConnection(s.hcCtx, at); err != nil {
				slog.Error("预连接 agent 失败", "agent", at, "err", err)
				return
			}
			slog.Info("预连接 agent 成功", "agent", at)
		}(agentType)
	}
}

// StartHealthCheck 启动后台健康检查与自动重连 goroutine。
// 定期检查所有已注册 backend 的连接状态，断开的自动重连（带指数退避）。
// 必须在所有 backend 注册完成后调用，且只能调用一次。
func (s *Service) StartHealthCheck() {
	s.hcCtx, s.hcCancel = context.WithCancel(context.Background())
	s.hcWG.Add(1)
	go s.healthCheckLoop()
}

// StopHealthCheck 停止健康检查 goroutine 并关闭所有共享连接。
func (s *Service) StopHealthCheck() {
	if s.hcCancel != nil {
		s.hcCancel()
	}
	s.hcWG.Wait()

	// 关闭所有共享连接
	s.mu.Lock()
	for _, conn := range s.pool {
		_ = conn.Close()
	}
	s.pool = make(map[string]*Connection)
	s.mu.Unlock()
}

// healthCheckLoop 定期检查连接状态并自动重连断开的 agent。
func (s *Service) healthCheckLoop() {
	defer s.hcWG.Done()

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	// 记录每个 agent 的重连退避延迟
	delays := make(map[string]time.Duration)

	for {
		select {
		case <-s.hcCtx.Done():
			return
		case <-ticker.C:
			s.checkAndReconnect(delays)
		}
	}
}

// checkAndReconnect 检查所有 backend 的连接状态，对断开的尝试重连。
func (s *Service) checkAndReconnect(delays map[string]time.Duration) {
	s.mu.RLock()
	types := make([]string, 0, len(s.backends))
	for name := range s.backends {
		types = append(types, name)
	}
	s.mu.RUnlock()

	for _, agentType := range types {
		if s.hcCtx.Err() != nil {
			return
		}
		s.checkAgent(agentType, delays)
	}
}

// checkAgent 检查单个 agent 的连接状态，必要时重连。
func (s *Service) checkAgent(agentType string, delays map[string]time.Duration) {
	s.mu.RLock()
	state := s.states[agentType]
	conn, hasConn := s.pool[agentType]
	s.mu.RUnlock()

	// 已连接且存活 → 重置退避
	if hasConn && state == connStateConnected {
		select {
		case <-conn.Done():
			// 连接已断开但状态未更新（watchConnection 可能还没跑到）
			s.mu.Lock()
			if cur, ok := s.pool[agentType]; ok && cur == conn {
				delete(s.pool, agentType)
				s.states[agentType] = connStateDisconnected
			}
			s.mu.Unlock()
		default:
			delays[agentType] = 0
			return
		}
	}

	// connecting 状态 → 跳过，等它完成
	if state == connStateConnecting {
		return
	}

	// disconnected → 尝试重连（带退避）
	delay, ok := delays[agentType]
	if !ok || delay == 0 {
		delay = reconnectBaseDelay
	}

	slog.Info("尝试重连 agent", "agent", agentType, "delay", delay)
	select {
	case <-s.hcCtx.Done():
		return
	case <-time.After(delay):
	}

	if _, err := s.ensureConnection(s.hcCtx, agentType); err != nil {
		slog.Error("重连 agent 失败", "agent", agentType, "err", err)
		// 指数退避，上限 reconnectMaxDelay
		next := delay * 2
		if next > reconnectMaxDelay {
			next = reconnectMaxDelay
		}
		delays[agentType] = next
		return
	}

	slog.Info("重连 agent 成功", "agent", agentType)
	delays[agentType] = 0
}

// CreateSession 创建新的 ACP 会话。
// cwd 为空时根据配置自动创建临时工作区。
// modelValue 非空时在会话创建后立即设置该模型（用于新建会话时选择模型）。
func (s *Service) CreateSession(ctx context.Context, agentType, cwd string, userID uint, modelValue string) (*models.Session, error) {
	return s.CreateSessionWithSource(ctx, agentType, cwd, userID, models.SessionSourceManual, modelValue)
}

// CreateSessionWithSource 创建会话并指定来源（manual/scheduled）。
// 复用 agentType 对应的共享连接，在其上新建 ACP session。
// modelValue 非空时在会话创建后立即设置该模型。
func (s *Service) CreateSessionWithSource(ctx context.Context, agentType, cwd string, userID uint, source, modelValue string) (*models.Session, error) {
	if _, err := s.GetBackend(agentType); err != nil {
		return nil, err
	}

	var ws *Workspace
	if cwd != "" {
		ws = NewExternalWorkspace(cwd)
	} else if s.wsConfig.DefaultMode == "temporary" {
		var err error
		ws, err = NewTemporaryWorkspace(s.wsConfig.SessionDir, s.wsConfig.TempDirPrefix)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("external 模式必须提供 cwd")
	}

	conn, err := s.ensureConnection(ctx, agentType)
	if err != nil {
		_ = ws.Cleanup()
		return nil, err
	}

	sessionID, configOptions, modes, err := conn.NewSession(ctx, ws.Cwd)
	if err != nil {
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
		_ = conn.CloseSessionByID(ctx, sessionID)
		_ = ws.Cleanup()
		return nil, fmt.Errorf("会话落库: %w", err)
	}

	s.mu.Lock()
	s.sessionAgent[sessionID] = agentType
	if len(configOptions) > 0 {
		s.configs[sessionID] = configOptions
	}
	if len(modes) > 0 {
		s.modes[sessionID] = modes
	}
	s.mu.Unlock()

	// 创建后立即设置模型（若指定且该 agent 支持 model config option）
	if modelValue != "" {
		if err := s.applyModelValue(ctx, sessionID, configOptions, modelValue); err != nil {
			// 模型设置失败不阻断会话创建，仅记录日志
			slog.Warn("创建会话后设置模型失败", "session", sessionID, "model", modelValue, "err", err)
		}
	}

	return session, nil
}

// connForSession 通过 sessionAgent 路由查找 session 所属的共享连接。
func (s *Service) connForSession(sessionID string) (*Connection, bool) {
	s.mu.RLock()
	agentType, ok := s.sessionAgent[sessionID]
	if !ok {
		s.mu.RUnlock()
		return nil, false
	}
	conn, ok := s.pool[agentType]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	select {
	case <-conn.Done():
		return nil, false
	default:
		return conn, true
	}
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

	conn, ok := s.connForSession(sessionID)
	if !ok {
		// 共享连接已断开但会话 DB 状态仍为 active：尝试自动恢复连接
		slog.Warn("会话连接丢失，尝试自动恢复", "session", sessionID, "agent", session.AgentType)
		if recConn, recErr := s.ensureConnection(ctx, session.AgentType); recErr == nil {
			// 恢复会话路由
			s.mu.Lock()
			s.sessionAgent[sessionID] = session.AgentType
			s.mu.Unlock()
			conn = recConn
		} else {
			slog.Error("自动恢复会话连接失败", "session", sessionID, "err", recErr)
			return nil, ErrSessionNotActive
		}
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
	conn, ok := s.connForSession(sessionID)
	if !ok {
		return ErrSessionNotFound
	}
	return conn.Cancel(ctx, sessionID)
}

// detachSession 从路由表与缓存中移除 session，返回 agentType 与是否存在连接。
func (s *Service) detachSession(sessionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	agentType, ok := s.sessionAgent[sessionID]
	if ok {
		delete(s.sessionAgent, sessionID)
	}
	delete(s.commands, sessionID)
	delete(s.configs, sessionID)
	delete(s.modes, sessionID)
	return agentType, ok
}

// hasActiveSession 判断指定 agentType 是否还有活跃 session 路由。
func (s *Service) hasActiveSession(agentType string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, at := range s.sessionAgent {
		if at == agentType {
			return true
		}
	}
	return false
}

// DeleteSession 彻底删除会话：释放 session 资源、清理工作区、删除消息与 会话记录。
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}

	agentType, hadConn := s.detachSession(sessionID)
	if hadConn {
		conn, connOK := s.pool[agentType]
		if connOK {
			_ = conn.CloseSessionByID(ctx, sessionID)
			if !s.hasActiveSession(agentType) {
				s.mu.Lock()
				delete(s.pool, agentType)
				s.mu.Unlock()
				_ = conn.Close()
			}
		}
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

// cleanupProbeSession 清理探测用临时会话，仅删除 ACP 会话、消息和数据库记录，
// 不关闭共享连接（与 DeleteSession 不同，后者在无活跃会话时会关闭整个 agent 进程）。
func (s *Service) cleanupProbeSession(ctx context.Context, sess *models.Session) {
	// 从路由表移除
	agentType, hadConn := s.detachSession(sess.SessionID)
	if hadConn {
		if conn, ok := s.pool[agentType]; ok {
			_ = conn.CloseSessionByID(ctx, sess.SessionID)
		}
	}
	// 清理工作区
	ws := &Workspace{Mode: sess.WorkspaceMode, TempDir: sess.TempDir}
	_ = ws.Cleanup()
	// 删除消息和会话记录
	_ = s.messages.DeleteByDBSessionID(sess.ID)
	_ = s.sessions.Delete(sess.ID)
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

// ProbeConfigOptions 返回指定 agent 类型的 config options。
// 首次调用时创建临时会话探测，结果缓存在内存中，后续调用直接返回缓存。
// 连接断开时缓存自动清空，重连后重新探测。
func (s *Service) ProbeConfigOptions(ctx context.Context, agentType string, userID uint) ([]acp.SessionConfigOption, error) {
	// 先查缓存
	s.mu.RLock()
	if cached, ok := s.probeCache[agentType]; ok {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	// 缓存未命中，创建临时会话探测
	sess, err := s.CreateSessionWithSource(ctx, agentType, "", userID, models.SessionSourceManual, "")
	if err != nil {
		return nil, fmt.Errorf("创建探测会话: %w", err)
	}

	// 发送简短的 prompt 触发 agent 返回 ConfigOptionUpdate
	promptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	msgChan, promptErr := s.Prompt(promptCtx, sess.SessionID, "ok")
	if promptErr == nil {
		for range msgChan {
		}
	} else {
		slog.Warn("探测 prompt 失败（非致命）", "agent", agentType, "err", promptErr)
	}
	cancel()

	opts, err := s.ListConfigOptions(sess.SessionID)
	if err != nil {
		_ = s.DeleteSession(ctx, sess.SessionID)
		return nil, fmt.Errorf("查询探测会话配置: %w", err)
	}
	slog.Info("探测配置结果", "agent", agentType, "config_count", len(opts))

	// 复制并缓存结果
	out := make([]acp.SessionConfigOption, len(opts))
	copy(out, opts)

	// 存入缓存
	s.mu.Lock()
	s.probeCache[agentType] = out
	s.mu.Unlock()

	// 清理临时会话（仅删除数据，不关闭连接）
	s.cleanupProbeSession(ctx, sess)
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
	conn, ok := s.connForSession(sessionID)
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

// ResumeSession 恢复或重开会话：在共享连接上新建 ACP session、注入历史上下文、更新 session_id。
// active 且连接存在的会话直接返回；error 与 closed 状态均会尝试恢复。
// cwdOverride 非空时使用该目录作为新工作区（用于已关闭会话的临时目录被清理的场景）。
func (s *Service) ResumeSession(ctx context.Context, sessionID, cwdOverride string) (*models.Session, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// active 且连接存在 → 直接返回
	if session.Status == models.SessionStatusActive {
		if _, ok := s.connForSession(sessionID); ok {
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

	// 复用共享连接（不存在则自动建立）
	conn, err := s.ensureConnection(ctx, session.AgentType)
	if err != nil {
		return nil, fmt.Errorf("恢复会话-建立连接: %w", err)
	}

	newSessionID, configOptions, modes, err := conn.NewSession(ctx, cwd)
	if err != nil {
		return nil, fmt.Errorf("恢复会话-创建 ACP 会话: %w", err)
	}

	// 若提供了 cwd 覆盖，更新会话工作区为 persistent 模式
	if cwdOverride != "" && cwdOverride != session.Cwd {
		// TODO(workspace): 后续迁移到 Workspace 模型后移除
		session.Cwd = cwdOverride
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
		_ = conn.CloseSessionByID(ctx, newSessionID)
		return nil, fmt.Errorf("恢复会话-更新 session_id: %w", err)
	}
	if err := s.sessions.UpdateStatus(session.ID, models.SessionStatusActive, nil); err != nil {
		_ = conn.CloseSessionByID(ctx, newSessionID)
		return nil, fmt.Errorf("恢复会话-更新状态: %w", err)
	}

	// 清理旧 sessionID 路由，建立新路由
	s.mu.Lock()
	delete(s.sessionAgent, sessionID)
	delete(s.commands, sessionID)
	delete(s.configs, sessionID)
	delete(s.modes, sessionID)
	s.sessionAgent[newSessionID] = session.AgentType
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

// AgentStatus 描述单个 agent 类型的连接状态，供前端侧边栏展示。
type AgentStatus struct {
	AgentType   string `json:"agent_type"`
	Status      string `json:"status"` // "connected" | "connecting" | "disconnected"
	ActiveCount int    `json:"active_count"`
}

// ListAgentStatus 返回所有已注册后端的连接状态与活跃会话数。
// status=connected 表示该 agent 类型的共享 ACP 连接已建立且进程存活。
func (s *Service) ListAgentStatus() []AgentStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 统计每个 agentType 的活跃 session 数
	counts := make(map[string]int, len(s.backends))
	for _, at := range s.sessionAgent {
		counts[at]++
	}

	out := make([]AgentStatus, 0, len(s.backends))
	for name := range s.backends {
		state := s.states[name]
		if state == "" {
			state = connStateDisconnected
		}
		// 若状态显示 connected 但实际连接已断开，修正为 disconnected
		if state == connStateConnected {
			if conn, ok := s.pool[name]; ok {
				select {
				case <-conn.Done():
					state = connStateDisconnected
				default:
				}
			} else {
				state = connStateDisconnected
			}
		}
		out = append(out, AgentStatus{
			AgentType:   name,
			Status:      state,
			ActiveCount: counts[name],
		})
	}
	return out
}

// applyModelValue 在指定 configOptions 中查找 category=model 的 select option，
// 若找到且 modelValue 在可选项中，则通过连接设置该值。
func (s *Service) applyModelValue(ctx context.Context, sessionID string, opts []acp.SessionConfigOption, modelValue string) error {
	for _, opt := range opts {
		if opt.Select == nil || opt.Select.Category == nil {
			continue
		}
		if string(*opt.Select.Category) != "model" {
			continue
		}
		// 校验 modelValue 是否在可选项中
		valid := false
		if opt.Select.Options.Ungrouped != nil {
			for _, o := range *opt.Select.Options.Ungrouped {
				if string(o.Value) == modelValue {
					valid = true
					break
				}
			}
		}
		if !valid && opt.Select.Options.Grouped != nil {
			for _, g := range *opt.Select.Options.Grouped {
				for _, o := range g.Options {
					if string(o.Value) == modelValue {
						valid = true
						break
					}
				}
				if valid {
					break
				}
			}
		}
		if !valid {
			return fmt.Errorf("模型值 %s 不在可用列表中", modelValue)
		}
		conn, ok := s.connForSession(sessionID)
		if !ok {
			return ErrSessionNotActive
		}
		return conn.SetConfigOption(ctx, sessionID, string(opt.Select.Id), modelValue)
	}
	return nil // 该 agent 无 model config option，静默跳过
}

// logWarn 统一警告日志输出。
func (s *Service) logWarn(msg, agent string) {
	slog.Warn(msg, "agent", agent)
}
