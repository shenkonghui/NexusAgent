package acp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"nexusagent/internal/config"
	"nexusagent/internal/logging"
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
// 连接池模型：每个 agent 类型 + 工作目录共享一条 ACP 连接（一个 agent 进程），
// 多个会话通过 SessionId 在同一条连接上多路复用。
// sessionPoolKey 维护 sessionID → 连接池键 的路由，用于定位所属连接。
//
// 连接生命周期由后台健康检查 goroutine 管理：
//   - 进程退出后标记 disconnected，定时任务自动重连
//   - 重连带指数退避，避免频繁重试
type Service struct {
	sessions   *repository.SessionRepository
	messages   *repository.MessageRepository
	workspaces *repository.WorkspaceRepository
	backends   map[string]Backend
	// pool 按 agentType+cwd 共享一条 Connection（进程 cwd 与 ACP session cwd 对齐）。
	pool map[string]*Connection
	// states 记录每个连接池键的状态（connecting/connected/disconnected）。
	states map[string]string
	// connectDone 在 connecting 期间供其他 goroutine 等待，完成后 close。
	connectDone map[string]chan struct{}
	// sessionPoolKey 记录 sessionID → 连接池键，用于定位所属连接。
	sessionPoolKey map[string]string
	commands       map[string][]acp.AvailableCommand
	configs        map[string][]acp.SessionConfigOption
	modes          map[string][]acp.SessionMode
	// probeCache 缓存探测结果，按 agentType 存储，避免重复创建临时会话探测。
	probeCache map[string][]acp.SessionConfigOption
	// agentCommands / agentModes 按 agentType 缓存，供新建任务页使用（无会话时）。
	agentCommands      map[string][]acp.AvailableCommand
	agentModes         map[string][]acp.SessionMode
	probeLock          sync.Mutex // 缓存未命中时串行探测，避免并发重复建临时 session
	mu                 sync.RWMutex
	wsConfig           config.WorkspaceConfig
	skillUserDirs      []string
	skillProjectDirs   []string
	noteSettings       *repository.NoteSettingsRepository
	publicBaseURL      string
	mcpConfigPath      string
	commandUserDirs    []string
	commandProjectDirs []string
	ruleUserDirs       []string
	ruleProjectDirs    []string

	// 健康检查与自动重连控制
	hcCtx        context.Context
	hcCancel     context.CancelFunc
	hcWG         sync.WaitGroup
	shuttingDown atomic.Bool

	// activePrompts 记录每个会话正在进行的 prompt 广播器，支持多客户端订阅（断点续传重连）。
	activePrompts map[string]*msgBroadcaster
	// runningTasks 记录进行中任务，用于服务重启后的中断恢复。
	runningTasks *repository.RunningTaskRepository

	// taskMetaTrigger 可选：发起任务时异步触发自动打标签 / 标题生成。nil 则跳过。
	taskMetaTrigger TaskMetaTrigger

	// dbg 可选：ACP 协议调试捕获器。nil 或未启用时零开销。
	dbg *ACPDebugger
}

// TaskMetaTrigger 由任务元数据服务实现，发起任务时异步调用以打标签和生成标题。
type TaskMetaTrigger interface {
	ProcessTask(userID, dbSessionID uint, prompt string)
}

// SetTaskMetaTrigger 注入任务元数据触发器。
func (s *Service) SetTaskMetaTrigger(t TaskMetaTrigger) {
	s.taskMetaTrigger = t
}

// NewService 创建新的 Service。
func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig, skillsConfig config.SkillsConfig, commandsConfig config.CommandsConfig, rulesConfig config.RulesConfig) *Service {
	return &Service{
		sessions:           repository.NewSessionRepository(db),
		messages:           repository.NewMessageRepository(db),
		workspaces:         repository.NewWorkspaceRepository(db),
		backends:           make(map[string]Backend),
		pool:               make(map[string]*Connection),
		states:             make(map[string]string),
		connectDone:        make(map[string]chan struct{}),
		sessionPoolKey:     make(map[string]string),
		commands:           make(map[string][]acp.AvailableCommand),
		configs:            make(map[string][]acp.SessionConfigOption),
		modes:              make(map[string][]acp.SessionMode),
		probeCache:         make(map[string][]acp.SessionConfigOption),
		agentCommands:      make(map[string][]acp.AvailableCommand),
		agentModes:         make(map[string][]acp.SessionMode),
		activePrompts:      make(map[string]*msgBroadcaster),
		runningTasks:       repository.NewRunningTaskRepository(db),
		wsConfig:           wsConfig,
		skillUserDirs:      append([]string(nil), skillsConfig.UserDirs...),
		skillProjectDirs:   append([]string(nil), skillsConfig.ProjectDirs...),
		commandUserDirs:    append([]string(nil), commandsConfig.UserDirs...),
		commandProjectDirs: append([]string(nil), commandsConfig.ProjectDirs...),
		ruleUserDirs:       append([]string(nil), rulesConfig.UserDirs...),
		ruleProjectDirs:    append([]string(nil), rulesConfig.ProjectDirs...),
	}
}

// SetNotesMCP 注入笔记 MCP 设置仓库与对外 Base URL（供 NewSession 注入）。
func (s *Service) SetNotesMCP(settings *repository.NoteSettingsRepository, publicBaseURL string) {
	s.noteSettings = settings
	s.publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
}

// SetMCPConfigPath 注入全局共享 MCP server 配置文件路径（供 NewSession 注入）。
// 该文件（标准 mcpServers 格式）中的 server 会注入给所有 agent 会话。
func (s *Service) SetMCPConfigPath(path string) {
	s.mcpConfigPath = strings.TrimSpace(path)
}

// configuredMCPServers 读取全局共享 MCP 配置文件并转换为 ACP server 列表。
// 文件不存在或解析失败时返回 nil 并记日志，不影响会话创建。
func (s *Service) configuredMCPServers() []acp.McpServer {
	if s.mcpConfigPath == "" {
		return nil
	}
	servers, err := LoadMCPServers(s.mcpConfigPath)
	if err != nil {
		slog.Warn("加载全局 MCP 配置失败，跳过注入", "path", s.mcpConfigPath, "err", err)
		return nil
	}
	return servers
}

// sessionMCPServers 汇总注入给指定会话的全部 MCP server：全局共享 + 笔记 MCP。
//
// 去重：若全局 mcp.json 已含 nexus-notes 条目（生成 token 时自动写入），
// 则不再追加按用户 token 动态注入的笔记 MCP，避免同名 server 重复注入。
func (s *Service) sessionMCPServers(userID uint) []acp.McpServer {
	configured := s.configuredMCPServers()
	if hasServerNamed(configured, notesMCPName) {
		return configured
	}
	return append(configured, s.notesMCPServers(userID)...)
}

// notesMCPName 是笔记 MCP server 的固定名称（与 mcp.json 中写入的条目名一致）。
const notesMCPName = "nexus-notes"

// hasServerNamed 判断 server 列表中是否存在指定名称的条目（任意传输类型）。
func hasServerNamed(servers []acp.McpServer, name string) bool {
	for _, s := range servers {
		switch {
		case s.Stdio != nil && s.Stdio.Name == name:
			return true
		case s.Http != nil && s.Http.Name == name:
			return true
		case s.Sse != nil && s.Sse.Name == name:
			return true
		case s.Acp != nil && s.Acp.Name == name:
			return true
		}
	}
	return false
}

// SetDebugConfig 注入 ACP 调试配置；enabled=false 时清空 debugger。
func (s *Service) SetDebugConfig(cfg config.DebugConfig) {
	if !cfg.ACP.Enabled {
		s.dbg = nil
		return
	}
	s.dbg = NewACPDebugger(DebugConfig{Enabled: true, Dir: cfg.ACP.Dir})
}

// Debugger 返回 ACP 调试器（可能为 nil）。
func (s *Service) Debugger() *ACPDebugger {
	return s.dbg
}

func (s *Service) debugLog(dbSessionID uint, event, acpSessionID string, detail any) {
	if s.dbg == nil {
		return
	}
	s.dbg.LogEvent(fmt.Sprintf("%d", dbSessionID), event, acpSessionID, detail)
}

func (s *Service) debugRegister(acpSessionID string, dbSessionID uint) {
	if s.dbg == nil {
		return
	}
	s.dbg.RegisterSession(acpSessionID, fmt.Sprintf("%d", dbSessionID))
}

func (s *Service) debugUnregister(acpSessionID string) {
	if s.dbg == nil {
		return
	}
	s.dbg.Unregister(acpSessionID)
}

func (s *Service) debugCleanup(dbSessionID uint) {
	if s.dbg == nil || dbSessionID == 0 {
		return
	}
	s.dbg.CleanupSession(fmt.Sprintf("%d", dbSessionID))
}

func (s *Service) debugBindPending(agentType string, dbSessionID uint) {
	if s.dbg == nil {
		return
	}
	s.dbg.BindPending(agentType, fmt.Sprintf("%d", dbSessionID))
}

func (s *Service) debugClearPending(agentType string) {
	if s.dbg == nil {
		return
	}
	s.dbg.ClearPending(agentType)
}

// agentSessionID 返回调 ACP 用的 sessionId；优先 AgentSessionID，兼容旧数据。
func agentSessionID(session *models.Session) string {
	if session.AgentSessionID != "" {
		return session.AgentSessionID
	}
	return session.SessionID
}

func (s *Service) notesMCPServers(userID uint) []acp.McpServer {
	if s.noteSettings == nil || userID == 0 || s.publicBaseURL == "" {
		return nil
	}
	st, err := s.noteSettings.FindByUserID(userID)
	if err != nil || strings.TrimSpace(st.McpToken) == "" {
		return nil
	}
	return []acp.McpServer{{
		Http: &acp.McpServerHttpInline{
			Name: "nexus-notes",
			Type: "http",
			Url:  s.publicBaseURL + "/mcp/notes",
			Headers: []acp.HttpHeader{{
				Name:  "Authorization",
				Value: "Bearer " + st.McpToken,
			}},
		},
	}}
}

// connectionKey 生成 agent 连接池键（agentType + 绝对 cwd）。
func connectionKey(agentType, cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil || abs == "" {
		abs = cwd
	}
	return agentType + "\x00" + abs
}

func splitConnectionKey(key string) (agentType, cwd string) {
	if i := strings.IndexByte(key, '\x00'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return key, ""
}

func (s *Service) agentTypeForSession(sessionID string) (string, bool) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return "", false
	}
	return session.AgentType, true
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
	s.closeConnectionsForAgentLocked(b.Name())
	s.backends[b.Name()] = b
	delete(s.probeCache, b.Name())
	delete(s.agentCommands, b.Name())
	delete(s.agentModes, b.Name())
	s.mu.Unlock()
}

// UnregisterBackend 注销一个 agent 后端，并关闭对应的共享连接。
func (s *Service) UnregisterBackend(name string) {
	s.mu.Lock()
	s.closeConnectionsForAgentLocked(name)
	delete(s.backends, name)
	delete(s.probeCache, name)
	delete(s.agentCommands, name)
	delete(s.agentModes, name)
	s.mu.Unlock()
}

func (s *Service) closeConnectionsForAgentLocked(agentType string) {
	var toClose []*Connection
	for key, conn := range s.pool {
		at, _ := splitConnectionKey(key)
		if at != agentType {
			continue
		}
		delete(s.pool, key)
		delete(s.states, key)
		toClose = append(toClose, conn)
	}
	for _, conn := range toClose {
		_ = conn.Close()
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

// ensureConnection 获取或创建指定 agentType + cwd 的共享连接。
// 若池中连接已断开（Done() 已关闭），自动清理并重建。
// 状态流转：disconnected → connecting → connected（或 connecting → disconnected 失败）。
func (s *Service) ensureConnection(ctx context.Context, agentType, cwd string) (*Connection, error) {
	if s.shuttingDown.Load() {
		return nil, context.Canceled
	}
	key := connectionKey(agentType, cwd)
	// 快速路径：池中有存活连接
	s.mu.RLock()
	conn, ok := s.pool[key]
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
	if existing, ok2 := s.pool[key]; ok2 {
		select {
		case <-existing.Done():
		default:
			s.mu.Unlock()
			return existing, nil
		}
	}
	// 已有 connecting 状态的 goroutine 在跑：等待完成后重试，避免重复建连或返回错误。
	if s.states[key] == connStateConnecting {
		done := s.connectDone[key]
		s.mu.Unlock()
		if done == nil {
			return nil, fmt.Errorf("agent %s 正在连接中，请稍后重试", agentType)
		}
		select {
		case <-done:
			return s.ensureConnection(ctx, agentType, cwd)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	doneCh := make(chan struct{})
	s.connectDone[key] = doneCh
	s.states[key] = connStateConnecting
	// 清理已断开的旧连接
	if oldConn, ok2 := s.pool[key]; ok2 {
		delete(s.pool, key)
		s.markSessionsErrorForPoolKeyLocked(key)
		_ = oldConn.Close()
	}
	s.mu.Unlock()

	// 在锁外执行耗时的进程启动与握手
	conn, err := s.buildConnection(ctx, agentType, cwd)
	s.mu.Lock()
	close(doneCh)
	delete(s.connectDone, key)
	if err != nil {
		s.states[key] = connStateDisconnected
		s.mu.Unlock()
		return nil, err
	}
	if s.shuttingDown.Load() {
		s.states[key] = connStateDisconnected
		s.mu.Unlock()
		_ = conn.Close()
		return nil, context.Canceled
	}
	// 并发场景：可能已有其他 goroutine 先建好，复用之并关闭多余的
	if existing, ok2 := s.pool[key]; ok2 {
		select {
		case <-existing.Done():
		default:
			s.mu.Unlock()
			_ = conn.Close()
			return existing, nil
		}
	}
	s.pool[key] = conn
	s.states[key] = connStateConnected
	s.mu.Unlock()

	go s.watchConnection(key, conn)
	return conn, nil
}

// buildConnection 执行实际的进程启动与 ACP 握手（无锁，可阻塞）。
func (s *Service) buildConnection(ctx context.Context, agentType, cwd string) (*Connection, error) {
	backend, err := s.GetBackend(agentType)
	if err != nil {
		return nil, err
	}
	// 若后端需要预处理（如 BinaryBackend 下载二进制），在启动进程前执行。
	// 失败时透传错误，让用户看到真正的失败原因（如下载失败）而非误导性的 PATH 错误。
	if p, ok := backend.(Preparable); ok {
		if err := p.Prepare(); err != nil {
			slog.Error("建立 agent 连接失败：准备后端失败",
				"agent", agentType, "err", err)
			return nil, fmt.Errorf("准备 agent 后端: %w", err)
		}
	}
	slog.Info("开始建立 agent 连接",
		"agent", agentType,
		"cwd", cwd,
		"command", backend.Command(),
		"args", backend.Args(),
	)
	if cwd != "" {
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			slog.Error("建立 agent 连接失败：创建工作目录失败",
				"agent", agentType, "cwd", cwd, "err", err)
			return nil, fmt.Errorf("创建工作目录 %s: %w", cwd, err)
		}
	}
	newConn, err := NewConnection(backend, cwd, s.dbg)
	if err != nil {
		slog.Error("建立 agent 连接失败：启动 agent 进程失败",
			"agent", agentType,
			"cwd", cwd,
			"command", backend.Command(),
			"args", backend.Args(),
			"err", err)
		return nil, fmt.Errorf("建立共享连接: %w", err)
	}
	initResp, err := newConn.Initialize(ctx)
	if err != nil {
		_ = newConn.Close()
		slog.Error("建立 agent 连接失败：ACP 握手失败",
			"agent", agentType,
			"command", backend.Command(),
			"err", err)
		return nil, fmt.Errorf("ACP 握手: %w", err)
	}
	if err := newConn.AuthenticateIfRequired(ctx, initResp); err != nil {
		_ = newConn.Close()
		slog.Error("建立 agent 连接失败：ACP 认证失败",
			"agent", agentType, "err", err)
		return nil, err
	}
	slog.Debug("建立 agent 连接成功",
		"agent", agentType, "cwd", cwd, "protocol", initResp.ProtocolVersion)
	return newConn, nil
}

// watchConnection 监控共享连接，进程退出时标记 disconnected。
// 不立即将会话标记为 error——由健康检查任务自动重连，重连成功后会话可继续使用。
// 仅当重连持续失败超过阈值时，才将会话标记为 error。
func (s *Service) watchConnection(poolKey string, conn *Connection) {
	<-conn.Done()
	agentType, _ := splitConnectionKey(poolKey)
	s.logWarn("agent 进程退出，标记为 disconnected", agentType)

	s.mu.Lock()
	// 仅当池中仍是该连接时才清理（避免清理已被重建替换的新连接）
	if cur, ok := s.pool[poolKey]; ok && cur == conn {
		delete(s.pool, poolKey)
		s.states[poolKey] = connStateDisconnected
		// 连接断开后清空探测缓存，重连时重新探测
		delete(s.probeCache, agentType)
	}
	s.mu.Unlock()
	// 健康检查 goroutine 会自动尝试重连
}

// markSessionsErrorForPoolKeyLocked 将指定连接池键下所有活跃会话标记为 error，
// 并清理对应的 sessionPoolKey 路由。调用方需持有 s.mu 写锁。
func (s *Service) markSessionsErrorForPoolKeyLocked(poolKey string) {
	for sid, key := range s.sessionPoolKey {
		if key != poolKey {
			continue
		}
		delete(s.sessionPoolKey, sid)
		delete(s.commands, sid)
		delete(s.configs, sid)
		delete(s.modes, sid)
		if sess, err := s.sessions.FindBySessionID(sid); err == nil {
			_ = s.sessions.UpdateStatus(sess.ID, models.SessionStatusError, nil)
		}
	}
}

// PreconnectAsync 异步为指定 agent + cwd 预建立共享连接，供新建会话页提前预热。
func (s *Service) PreconnectAsync(agentType, cwd string) {
	if cwd == "" {
		cwd = s.probeCwd()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if _, err := s.ensureConnection(ctx, agentType, cwd); err != nil {
			slog.Debug("工作区预连接失败", "agent", agentType, "cwd", cwd, "err", err)
		}
	}()
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
			cwd := s.probeCwd()
			slog.Info("开始预连接 agent", "agent", at, "cwd", cwd)
			if _, err := s.ensureConnection(s.hcCtx, at, cwd); err != nil {
				slog.Error("预连接 agent 失败",
					"agent", at,
					"cwd", cwd,
					"command", s.backendCommandSafe(at),
					"err", err)
				return
			}
			slog.Info("预连接 agent 成功", "agent", at, "cwd", cwd)
			s.prefetchProbeConfig(s.hcCtx, at)
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
	s.shuttingDown.Store(true)
	if s.hcCancel != nil {
		s.hcCancel()
	}

	// 立即终止 agent 子进程，不等待健康检查循环结束
	s.mu.Lock()
	conns := make([]*Connection, 0, len(s.pool))
	for _, conn := range s.pool {
		conns = append(conns, conn)
	}
	s.pool = make(map[string]*Connection)
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}

	done := make(chan struct{})
	go func() {
		s.hcWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		slog.Warn("健康检查 goroutine 退出超时，继续关闭")
	}
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

// checkAndReconnect 检查所有连接池键的状态，对断开的尝试重连。
func (s *Service) checkAndReconnect(delays map[string]time.Duration) {
	s.mu.RLock()
	keys := make([]string, 0, len(s.states))
	for key := range s.states {
		keys = append(keys, key)
	}
	s.mu.RUnlock()

	for _, key := range keys {
		if s.hcCtx.Err() != nil {
			return
		}
		s.checkConnectionKey(key, delays)
	}
}

// checkConnectionKey 检查单个连接池键的状态，必要时重连。
func (s *Service) checkConnectionKey(poolKey string, delays map[string]time.Duration) {
	agentType, cwd := splitConnectionKey(poolKey)
	s.mu.RLock()
	state := s.states[poolKey]
	conn, hasConn := s.pool[poolKey]
	s.mu.RUnlock()

	// 已连接且存活 → 重置退避
	if hasConn && state == connStateConnected {
		select {
		case <-conn.Done():
			// 连接已断开但状态未更新（watchConnection 可能还没跑到）
			s.mu.Lock()
			if cur, ok := s.pool[poolKey]; ok && cur == conn {
				delete(s.pool, poolKey)
				s.states[poolKey] = connStateDisconnected
			}
			s.mu.Unlock()
		default:
			delays[poolKey] = 0
			return
		}
	}

	// connecting 状态 → 跳过，等它完成
	if state == connStateConnecting {
		return
	}

	// disconnected → 尝试重连（带退避）
	delay, ok := delays[poolKey]
	if !ok || delay == 0 {
		delay = reconnectBaseDelay
	}

	slog.Info("尝试重连 agent",
		"agent", agentType, "cwd", cwd,
		"poolKey", poolKey, "prevState", state, "delay", delay)
	select {
	case <-s.hcCtx.Done():
		return
	case <-time.After(delay):
	}

	if _, err := s.ensureConnection(s.hcCtx, agentType, cwd); err != nil {
		// 指数退避，上限 reconnectMaxDelay
		next := delay * 2
		if next > reconnectMaxDelay {
			next = reconnectMaxDelay
		}
		delays[poolKey] = next
		slog.Error("重连 agent 失败",
			"agent", agentType,
			"cwd", cwd,
			"poolKey", poolKey,
			"prevState", state,
			"delay", delay,
			"nextDelay", next,
			"command", s.backendCommandSafe(agentType),
			"err", err)
		return
	}

	slog.Info("重连 agent 成功", "agent", agentType, "cwd", cwd, "delay", delay)
	delays[poolKey] = 0
	s.prefetchProbeConfig(s.hcCtx, agentType)
}

// CreateSession 创建新的 ACP 会话。
// workspaceID 非 0 时使用指定 workspace 的 cwd，否则自动创建默认 workspace。
// modelValue 非空时在会话创建后立即设置该模型。
func (s *Service) CreateSession(ctx context.Context, agentType string, workspaceID uint, userID uint, modelValue string) (*models.Session, error) {
	return s.CreateSessionWithSource(ctx, agentType, workspaceID, userID, models.SessionSourceManual, modelValue)
}

// CreateSessionWithSource 创建会话并指定来源（manual/scheduled）。
// workspaceID 非 0 时使用指定 workspace，否则查找或创建默认 temporary workspace。
// modelValue 非空时将在首次 Prompt 时应用。
// 为提升页面跳转速度，不再同步创建 ACP 会话，而是返回 pending 状态；
// ACP 连接与会话创建延迟到首次 Prompt（PromptWithExecution）时完成。
func (s *Service) CreateSessionWithSource(ctx context.Context, agentType string, workspaceID uint, userID uint, source, modelValue string) (*models.Session, error) {
	if _, err := s.GetBackend(agentType); err != nil {
		return nil, err
	}

	var ws *Workspace
	var dbWS *models.Workspace
	createdDefaultWS := false
	if workspaceID > 0 {
		var err error
		dbWS, err = s.workspaces.FindByID(workspaceID)
		if err != nil {
			return nil, fmt.Errorf("工作区不存在: %w", err)
		}
		if dbWS.UserID != userID {
			return nil, errors.New("无权访问该工作区")
		}
		ws = &Workspace{Mode: dbWS.Mode, Cwd: dbWS.Cwd, TempDir: dbWS.TempDir, Directories: dbWS.Directories}
		if err := EnsureWorkspaceDir(dbWS.Mode, dbWS.Cwd); err != nil {
			return nil, err
		}
	} else {
		var err error
		dbWS, err = s.workspaces.FindDefaultByUserID(userID)
		if err != nil {
			tempWs, tErr := NewTemporaryWorkspace(s.wsConfig.SessionDir, s.wsConfig.TempDirPrefix)
			if tErr != nil {
				return nil, tErr
			}
			newWS := &models.Workspace{
				UserID:  userID,
				Name:    "默认工作区",
				Cwd:     tempWs.Cwd,
				Mode:    models.WorkspaceModeTemporary,
				TempDir: tempWs.TempDir,
			}
			if cErr := s.workspaces.Create(newWS); cErr != nil {
				_ = tempWs.Cleanup()
				return nil, fmt.Errorf("创建默认工作区: %w", cErr)
			}
			dbWS = newWS
			ws = tempWs
			createdDefaultWS = true
		} else {
			ws = &Workspace{Mode: dbWS.Mode, Cwd: dbWS.Cwd, TempDir: dbWS.TempDir, Directories: dbWS.Directories}
			if err := EnsureWorkspaceDir(dbWS.Mode, dbWS.Cwd); err != nil {
				return nil, err
			}
		}
		workspaceID = dbWS.ID
	}

	rollbackNewDefaultWS := func() {
		if !createdDefaultWS || dbWS == nil {
			return
		}
		_ = s.workspaces.Delete(dbWS.ID)
		_ = ws.Cleanup()
	}

	// 生成稳定 SessionID，不创建 ACP 会话，快速返回 pending 状态
	tempSessionID := uuid.New().String()
	wid := dbWS.ID
	session := &models.Session{
		SessionID:   tempSessionID,
		AgentType:   agentType,
		Cwd:         ws.Cwd,
		Status:      models.SessionStatusPending,
		UserID:      userID,
		WorkspaceID: &wid,
		Source:      source,
		ModelValue:  modelValue,
	}
	if err := s.sessions.Create(session); err != nil {
		rollbackNewDefaultWS()
		return nil, fmt.Errorf("会话落库: %w", err)
	}
	session.Workspace = *dbWS

	slog.Info("会话已创建（pending）", "agent", agentType, "session", tempSessionID, "cwd", ws.Cwd)
	return session, nil
}

// connForSession 通过 sessionPoolKey 路由查找 session 所属的共享连接。
func (s *Service) connForSession(sessionID string) (*Connection, bool) {
	s.mu.RLock()
	poolKey, ok := s.sessionPoolKey[sessionID]
	if !ok {
		s.mu.RUnlock()
		return nil, false
	}
	conn, ok := s.pool[poolKey]
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
// pending 状态的会话在首次调用时自动完成连接建立与 ACP 会话创建。
func (s *Service) PromptWithExecution(ctx context.Context, sessionID, prompt string, executionID *uint) (<-chan models.Message, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.SessionStatusActive && session.Status != models.SessionStatusPending {
		return nil, ErrSessionNotActive
	}

	// 延迟激活：pending 会话在首次 Prompt 时创建 ACP 会话
	if session.Status == models.SessionStatusPending {
		cwd := sessionCwd(session, s.workspaces)
		conn, actErr := s.ensureConnection(ctx, session.AgentType, cwd)
		if actErr != nil {
			return nil, fmt.Errorf("激活会话-建立连接: %w", actErr)
		}
		s.debugBindPending(session.AgentType, session.ID)
		newAgentSID, configOptions, modes, actErr := conn.NewSession(ctx, cwd, s.sessionAdditionalDirs(session, cwd), s.sessionMCPServers(session.UserID), s.rulesSystemPrompt(cwd))
		s.debugClearPending(session.AgentType)
		if actErr != nil {
			return nil, fmt.Errorf("激活会话-创建 ACP 会话: %w", actErr)
		}

		// 写入 agent_session_id，不改稳定 session_id
		if actErr = s.sessions.UpdateAgentSessionID(session.ID, newAgentSID); actErr != nil {
			_ = conn.CloseSessionByID(ctx, newAgentSID)
			return nil, fmt.Errorf("激活会话-更新 agent_session_id: %w", actErr)
		}
		if actErr = s.sessions.UpdateStatus(session.ID, models.SessionStatusActive, nil); actErr != nil {
			_ = conn.CloseSessionByID(ctx, newAgentSID)
			return nil, fmt.Errorf("激活会话-更新状态: %w", actErr)
		}

		// 内存路由以稳定 session_id 为键
		poolKey := connectionKey(session.AgentType, cwd)
		s.mu.Lock()
		s.sessionPoolKey[sessionID] = poolKey
		if len(configOptions) > 0 {
			s.configs[sessionID] = configOptions
		}
		if len(modes) > 0 {
			s.modes[sessionID] = modes
			s.agentModes[session.AgentType] = modes
		}
		s.mu.Unlock()
		s.debugRegister(newAgentSID, session.ID)
		s.debugLog(session.ID, "new_session", newAgentSID, map[string]any{
			"agent": session.AgentType, "cwd": cwd,
		})
		slog.Info("agent 会话已激活", "agent", session.AgentType, "session", sessionID, "agent_session", newAgentSID, "cwd", cwd)

		session.AgentSessionID = newAgentSID
		session.Status = models.SessionStatusActive

		// 应用用户在创建会话时选择的模型（configOptions 在激活时才可用，故延迟到此设置）
		if modelValue := strings.TrimSpace(session.ModelValue); modelValue != "" {
			if err := s.applyModelValue(ctx, sessionID, newAgentSID, configOptions, modelValue); err != nil {
				slog.Warn("激活会话-应用用户模型失败", "agent", session.AgentType, "session", sessionID, "model", modelValue, "err", err)
			}
		}
	}

	conn, ok := s.connForSession(sessionID)
	if !ok {
		// 共享连接已断开但会话 DB 状态仍为 active：尝试自动恢复连接
		cwd := sessionCwd(session, s.workspaces)
		slog.Warn("会话连接丢失，尝试自动恢复", "session", sessionID, "agent", session.AgentType, "cwd", cwd)
		if recConn, recErr := s.ensureConnection(ctx, session.AgentType, cwd); recErr == nil {
			// 恢复会话路由
			s.mu.Lock()
			s.sessionPoolKey[sessionID] = connectionKey(session.AgentType, cwd)
			s.mu.Unlock()
			conn = recConn
		} else {
			slog.Error("自动恢复会话连接失败", "session", sessionID, "err", recErr)
			return nil, ErrSessionNotActive
		}
	}

	acpSID := agentSessionID(session)
	agentPrompt := s.expandPrompt(sessionID, session, prompt)

	// 注入工作区附加目录上下文，让 AI 知晓可访问的额外目录
	if dirCtx := s.workspaceDirContext(session); dirCtx != "" {
		agentPrompt = dirCtx + "\n" + agentPrompt
	}
	slog.Debug("发送 agent prompt",
		"session", sessionID,
		"agent_session", acpSID,
		"agent", session.AgentType,
		"prompt_chars", len(prompt),
		"expanded_chars", len(agentPrompt),
		"preview", logging.Preview(prompt, 120),
	)
	s.debugLog(session.ID, "prompt", acpSID, map[string]any{
		"chars": len(prompt), "preview": logging.Preview(prompt, 80),
	})
	updates, err := conn.Prompt(ctx, acpSID, agentPrompt)
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

	// 首次对话时异步触发任务自动打标签 / AI 标题生成（fire-and-forget，不阻塞主对话流）。
	// 仅在首次对话触发，避免每次追问都重复分类。
	if s.taskMetaTrigger != nil && session.Title == "" {
		uid := session.UserID
		dbID := session.ID
		p := prompt
		go s.taskMetaTrigger.ProcessTask(uid, dbID, p)
	}

	// 创建广播器：当前 prompt 的所有消息经广播器分发，支持多客户端订阅（断点续传重连）。
	startSeq := s.getNextSequence(session.ID)
	bc := newMsgBroadcaster(startSeq)
	s.registerBroadcaster(sessionID, bc)

	// 创建 running_task 记录，用于服务重启后的中断恢复。
	task := &models.RunningTask{
		DBSessionID: session.ID,
		UserID:      session.UserID,
		Prompt:      prompt,
		Status:      models.RunningTaskStatusRunning,
		LastSeq:     startSeq,
		ExecutionID: executionID,
		StartedAt:   time.Now(),
	}
	if err := s.runningTasks.Create(task); err != nil {
		slog.Warn("创建 running_task 记录失败", "session", sessionID, "err", err)
	}
	finishTask := func(status string) {
		if task.ID == 0 {
			return
		}
		now := time.Now()
		if err := s.runningTasks.UpdateStatus(task.ID, status, &now); err != nil {
			slog.Warn("更新 running_task 状态失败", "task", task.ID, "status", status, "err", err)
		}
	}

	// 主订阅者：发起 prompt 的原始请求消费此 channel。
	// prompt 前快照工作目录，用于结束后对比文件改动。
	snapshotCwd := sessionCwd(session, s.workspaces)
	snapshotBefore := takeSnapshot(snapshotCwd)
	out := make(chan models.Message, 256)
	go func() {
		seq := startSeq

		defer func() {
			// prompt 结束后快照对比，生成文件改动摘要消息
			snapshotAfter := takeSnapshot(snapshotCwd)
			diffs := compareSnapshots(snapshotBefore, snapshotAfter)
			if len(diffs) > 0 {
				seq++
				fileMsg := MapFileWriteBatch(sessionID, session.ID, seq, diffs)
				fileMsg.ExecutionID = executionID
				if err := s.messages.Create(&fileMsg); err != nil {
					slog.Error("持久化文件改动摘要失败", "session", sessionID, "sequence", fileMsg.Sequence, "err", err)
				} else {
					bc.broadcast(fileMsg)
					out <- fileMsg
					if task.ID != 0 {
						_ = s.runningTasks.UpdateLastSeq(task.ID, fileMsg.Sequence)
					}
				}
			}
			close(out)
			bc.close()
			s.unregisterBroadcaster(sessionID)
			finishTask(models.RunningTaskStatusDone)
		}()

		sid := acp.SessionId(acpSID)
		permCh := conn.Client().RegisterPermissionWaiter(sid)
		defer conn.Client().UnregisterPermissionWaiter(sid)
		fileCh := conn.Client().RegisterFileWaiter(sid)
		defer conn.Client().UnregisterFileWaiter(sid)

		// persistMsg 持久化消息并广播，同步更新 running_task 的 LastSeq。
		persistMsg := func(msg models.Message) {
			if err := s.messages.Create(&msg); err != nil {
				slog.Error("持久化消息失败", "session", sessionID, "sequence", msg.Sequence, "err", err)
			}
			bc.broadcast(msg)
			out <- msg
			if task.ID != 0 {
				_ = s.runningTasks.UpdateLastSeq(task.ID, msg.Sequence)
			}
		}

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
		persistMsg(userMsg)

		// 读取 agent 返回的 update 流与权限请求，逐条持久化并转发
		for {
			select {
			case u, ok := <-updates:
				if !ok {
					slog.Debug("agent prompt 流结束", "session", sessionID)
					return
				}
				s.captureCommands(sessionID, u)
				seq++
				msg := MapUpdate(sessionID, session.ID, seq, u)
				msg.ExecutionID = executionID
				persistMsg(msg)
			case pn, ok := <-permCh:
				if !ok {
					continue
				}
				slog.Debug("agent 权限请求", "session", sessionID, "request_id", pn.RequestID)
				seq++
				msg := MapPermissionRequest(sessionID, session.ID, seq, pn)
				msg.ExecutionID = executionID
				persistMsg(msg)
			case fw, ok := <-fileCh:
				if !ok {
					continue
				}
				seq++
				fileMsg := MapFileWrite(sessionID, session.ID, seq, fw)
				fileMsg.ExecutionID = executionID
				persistMsg(fileMsg)
			}
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
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}
	acpSID := agentSessionID(sess)
	s.debugLog(sess.ID, "cancel", acpSID, nil)
	conn.Client().CancelPermissions(acp.SessionId(acpSID))
	return conn.Cancel(ctx, acpSID)
}

// registerBroadcaster 注册指定会话的活跃 prompt 广播器。
func (s *Service) registerBroadcaster(sessionID string, bc *msgBroadcaster) {
	s.mu.Lock()
	s.activePrompts[sessionID] = bc
	s.mu.Unlock()
}

// unregisterBroadcaster 移除指定会话的活跃 prompt 广播器。
func (s *Service) unregisterBroadcaster(sessionID string) {
	s.mu.Lock()
	delete(s.activePrompts, sessionID)
	s.mu.Unlock()
}

// SubscribeSession 订阅指定会话当前进行中的 prompt 流，用于断点续传。
// lastSeq 为客户端最后收到的 message sequence；返回值：
//   - missed: DB 中 sequence > lastSeq 的遗漏消息（需先补发给客户端）
//   - ch: 实时消息 channel（若无进行中的 prompt 则为 nil）
//   - 若会话当前无活跃 prompt，返回 missed（补齐尾部）+ nil channel
func (s *Service) SubscribeSession(sessionID string, lastSeq int) (missed []models.Message, ch <-chan models.Message, err error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, nil, err
	}

	// 先从 DB 补齐 lastSeq 之后的遗漏消息
	missed, dbErr := s.messages.FindByDBSessionIDAfter(session.ID, lastSeq)
	if dbErr != nil {
		return nil, nil, dbErr
	}

	// 检查是否有活跃 prompt 广播器
	s.mu.RLock()
	bc, ok := s.activePrompts[sessionID]
	s.mu.RUnlock()

	if !ok || bc == nil {
		// 无活跃 prompt：仅返回 DB 补齐的消息，channel 为 nil
		return missed, nil, nil
	}

	// 订阅广播器，获取订阅时刻的 currentSeq
	subCh, curSeq := bc.subscribe(256)

	// 从订阅时刻的 currentSeq 之后去重 missed，避免与广播器即将推送的消息重复
	// （广播器 currentSeq 之后的实时消息会经 channel 推送，missed 只取到 currentSeq）
	if len(missed) > 0 && curSeq > lastSeq {
		filtered := missed[:0]
		for _, m := range missed {
			if m.Sequence <= curSeq {
				filtered = append(filtered, m)
			}
		}
		missed = filtered
	}

	return missed, subCh, nil
}

// HasActivePrompt 判断指定会话是否有进行中的 prompt。
func (s *Service) HasActivePrompt(sessionID string) bool {
	s.mu.RLock()
	_, ok := s.activePrompts[sessionID]
	s.mu.RUnlock()
	return ok
}

func (s *Service) detachSession(sessionID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	poolKey, ok := s.sessionPoolKey[sessionID]
	if ok {
		delete(s.sessionPoolKey, sessionID)
	}
	delete(s.commands, sessionID)
	delete(s.configs, sessionID)
	delete(s.modes, sessionID)
	return poolKey, ok
}

// hasActiveSessionForPoolKey 判断指定连接池键是否还有活跃 session 路由。
func (s *Service) hasActiveSessionForPoolKey(poolKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, key := range s.sessionPoolKey {
		if key == poolKey {
			return true
		}
	}
	return false
}

// DeleteSession 彻底删除会话：释放连接、删除消息/记录，并清理 debug 日志与孤儿 temporary 工作区。
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}
	wsID := session.WorkspaceID

	s.debugUnregister(agentSessionID(session))
	poolKey, hadConn := s.detachSession(sessionID)
	if hadConn {
		conn, connOK := s.pool[poolKey]
		if connOK {
			_ = conn.CloseSessionByID(ctx, agentSessionID(session))
			if !s.hasActiveSessionForPoolKey(poolKey) {
				s.mu.Lock()
				delete(s.pool, poolKey)
				delete(s.states, poolKey)
				s.mu.Unlock()
				_ = conn.Close()
			}
		}
	}

	// 先删消息再删会话，避免孤儿消息
	if err := s.messages.DeleteByDBSessionID(session.ID); err != nil {
		return fmt.Errorf("删除会话消息: %w", err)
	}
	if err := s.sessions.Delete(session.ID); err != nil {
		return fmt.Errorf("删除会话记录: %w", err)
	}
	s.debugCleanup(session.ID)
	s.maybeCleanupOrphanTemporaryWorkspace(wsID)
	return nil
}

// maybeCleanupOrphanTemporaryWorkspace 在 temporary 工作区已无会话时删除目录与记录。
func (s *Service) maybeCleanupOrphanTemporaryWorkspace(wsID *uint) {
	if wsID == nil || *wsID == 0 {
		return
	}
	ws, err := s.workspaces.FindByID(*wsID)
	if err != nil || ws == nil || ws.Mode != models.WorkspaceModeTemporary {
		return
	}
	count, err := s.workspaces.SessionCount(*wsID)
	if err != nil || count > 0 {
		return
	}
	_ = s.workspaces.Delete(*wsID)
	if err := (&Workspace{Mode: ws.Mode, Cwd: ws.Cwd, TempDir: ws.TempDir}).Cleanup(); err != nil {
		slog.Warn("清理 temporary 工作区失败", "workspaceID", *wsID, "err", err)
	}
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

// RecoverActiveSessions 在服务启动时调用：
// 1. 将所有 running 状态的 running_task 标记为 interrupted（服务重启导致 prompt 中断）。
// 2. 将所有 active 状态的会话标记为 error（agent 进程已随重启终止，内存态丢失）。
// 用户可通过 ListInterruptedTasks 查看中断任务并手动重发。
func (s *Service) RecoverActiveSessions() {
	if err := s.runningTasks.MarkRunningAsInterrupted(); err != nil {
		slog.Warn("标记中断任务失败", "err", err)
	}
	if err := s.sessions.MarkActiveAsError(); err != nil {
		slog.Warn("标记活跃会话为 error 失败", "err", err)
	}
}

// ListInterruptedTasks 返回指定会话下所有 interrupted 状态的任务。
func (s *Service) ListInterruptedTasks(dbSessionID uint) ([]models.RunningTask, error) {
	return s.runningTasks.FindInterruptedByDBSessionID(dbSessionID)
}

// ListRunningDBSessionIDs 返回指定用户下所有 status=running 的 db_session_id，
// 供侧边栏展示「哪些会话正在运行」。
func (s *Service) ListRunningDBSessionIDs(userID uint) ([]uint, error) {
	return s.runningTasks.FindRunningDBSessionIDsByUser(userID)
}

// ResumeInterruptedTask 恢复中断的任务：ResumeSession 后重新发送原 prompt。
// 返回的消息流与普通 Prompt 一致（含 id: sequence，经广播器分发）。
func (s *Service) ResumeInterruptedTask(ctx context.Context, taskID uint) (<-chan models.Message, error) {
	task, err := s.runningTasks.FindByID(taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != models.RunningTaskStatusInterrupted {
		return nil, fmt.Errorf("任务状态不是 interrupted（当前: %s），无法恢复", task.Status)
	}

	session, err := s.sessions.FindByID(task.DBSessionID)
	if err != nil {
		return nil, err
	}

	// 恢复会话（error/closed 状态会重开 ACP session）
	if _, err := s.ResumeSession(ctx, session.SessionID); err != nil {
		return nil, fmt.Errorf("恢复会话失败: %w", err)
	}

	// 重新发送原 prompt（使用重开后的 sessionID）
	ch, err := s.PromptWithExecution(ctx, session.SessionID, task.Prompt, task.ExecutionID)
	if err != nil {
		return nil, fmt.Errorf("重发 prompt 失败: %w", err)
	}

	// 标记任务已完成（重发后会创建新的 running_task 记录）
	now := time.Now()
	_ = s.runningTasks.UpdateStatus(taskID, models.RunningTaskStatusDone, &now)

	return ch, nil
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
		if agentType, ok := s.agentTypeForSession(sessionID); ok && len(cmds) > 0 {
			s.agentCommands[agentType] = cmds
		}
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

// ListCommands 返回会话可用的 slash command（Agent 原生 + 配置的 Claude Code commands）。
func (s *Service) ListCommands(sessionID string) ([]acp.AvailableCommand, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	cwd := sessionCwd(session, s.workspaces)
	s.mu.RLock()
	agentCmds := s.commands[sessionID]
	s.mu.RUnlock()
	return s.mergeCommands(agentCmds, cwd), nil
}

func sessionCwd(session *models.Session, workspaces *repository.WorkspaceRepository) string {
	cwd := session.Cwd
	if session.WorkspaceID != nil {
		if ws, err := workspaces.FindByID(*session.WorkspaceID); err == nil {
			cwd = ws.Cwd
		}
	}
	return cwd
}

// sessionAdditionalDirs 返回 ACP 会话的 additionalDirectories：
// 工作区附加目录（次级）+ skills/commands/rules 目录。
func (s *Service) sessionAdditionalDirs(session *models.Session, cwd string) []string {
	var wsDirs []string
	if session.WorkspaceID != nil {
		if ws, err := s.workspaces.FindByID(*session.WorkspaceID); err == nil {
			wsDirs = ws.Directories
			slog.Info("工作区附加目录", "workspaceID", *session.WorkspaceID, "directories", wsDirs)
		} else {
			slog.Warn("查找工作区附加目录失败", "workspaceID", *session.WorkspaceID, "err", err)
		}
	} else {
		slog.Warn("会话无 WorkspaceID，无法获取附加目录", "sessionID", session.SessionID)
	}
	result := MergeAdditionalDirectories(
		wsDirs,
		s.skillAdditionalDirs(cwd),
	)
	slog.Info("会话 AdditionalDirectories", "sessionID", session.SessionID, "total", len(result), "dirs", result)
	return result
}

// workspaceDirContext 返回工作区附加目录的 prompt 上下文文本。
// 仅包含用户配置的工作区次级目录（不含 skills/commands/rules），以便 AI 知晓可访问的额外文件目录。
func (s *Service) workspaceDirContext(session *models.Session) string {
	if session.WorkspaceID == nil {
		return ""
	}
	ws, err := s.workspaces.FindByID(*session.WorkspaceID)
	if err != nil || len(ws.Directories) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<workspace_directories>\n以下目录在当前工作区中可访问：\n")
	for _, d := range ws.Directories {
		sb.WriteString(fmt.Sprintf("- %s\n", d))
	}
	sb.WriteString("你可以读取和操作这些目录中的文件。\n")
	sb.WriteString("</workspace_directories>")
	return sb.String()
}

func (s *Service) expandPrompt(sessionID string, session *models.Session, prompt string) string {
	cwd := sessionCwd(session, s.workspaces)
	s.mu.RLock()
	agentCmds := s.commands[sessionID]
	modes := s.modes[sessionID]
	s.mu.RUnlock()
	_, expanded := ExpandPrompt(ExpandPromptInput{
		Prompt:             prompt,
		Cwd:                cwd,
		SkillUserDirs:      s.skillUserDirs,
		SkillProjectDirs:   s.skillProjectDirs,
		CommandUserDirs:    s.commandUserDirs,
		CommandProjectDirs: s.commandProjectDirs,
		AgentCommands:      agentCmds,
		Modes:              modes,
	})
	if expanded != prompt {
		slog.Info("slash 调用已展开", "session", sessionID, "chars", len(expanded))
	}
	return expanded
}

// rulesSystemPrompt 汇总 alwaysApply 规则，注入 session/new 的 _meta.systemPrompt。
func (s *Service) rulesSystemPrompt(cwd string) string {
	return AlwaysApplySystemPrompt(cwd, s.ruleUserDirs, s.ruleProjectDirs)
}

func (s *Service) mergeCommands(agentCmds []acp.AvailableCommand, cwd string) []acp.AvailableCommand {
	configured := SlashCommandsToAvailable(ScanSlashCommands(cwd, s.commandUserDirs, s.commandProjectDirs))
	return MergeAvailableCommands(agentCmds, configured)
}

// ListConfiguredCommands 扫描配置的 slash command（Claude Code commands 目录）。
func (s *Service) ListConfiguredCommands(cwd string) []SlashCommand {
	return ScanSlashCommands(cwd, s.commandUserDirs, s.commandProjectDirs)
}

// ListConfiguredCommandsForSession 扫描会话工作区下的配置 slash command。
func (s *Service) ListConfiguredCommandsForSession(sessionID string) ([]SlashCommand, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	cwd := sessionCwd(session, s.workspaces)
	return ScanSlashCommands(cwd, s.commandUserDirs, s.commandProjectDirs), nil
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

// CachedCommands 返回指定 agent 类型的 slash command（Agent 原生 + 配置的 commands）。
// cwd 非空时一并扫描项目级 commands 目录。
func (s *Service) CachedCommands(agentType string, cwd string) []acp.AvailableCommand {
	s.mu.RLock()
	agentCmds := s.agentCommands[agentType]
	s.mu.RUnlock()
	return s.mergeCommands(agentCmds, cwd)
}

// CachedModes 返回指定 agent 类型缓存的 session mode（来自探测或已有会话）。
func (s *Service) CachedModes(agentType string) []acp.SessionMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	modes := s.agentModes[agentType]
	if len(modes) == 0 {
		return nil
	}
	out := make([]acp.SessionMode, len(modes))
	copy(out, modes)
	return out
}

// ProbeConfigOptions 返回指定 agent 类型的 config options。
// 结果在 agent 首次连接时预探测并缓存在内存中；缓存未命中时轻量探测（仅 NewSession，不发送 prompt）。
func (s *Service) ProbeConfigOptions(ctx context.Context, agentType string, userID uint) ([]acp.SessionConfigOption, error) {
	_ = userID
	return s.fetchProbeConfig(ctx, agentType)
}

func (s *Service) prefetchProbeConfig(ctx context.Context, agentType string) {
	if _, err := s.fetchProbeConfig(ctx, agentType); err != nil {
		slog.Warn("预探测 agent 配置失败", "agent", agentType, "err", err)
	}
}

func (s *Service) fetchProbeConfig(ctx context.Context, agentType string) ([]acp.SessionConfigOption, error) {
	s.mu.RLock()
	if cached, ok := s.probeCache[agentType]; ok {
		out := make([]acp.SessionConfigOption, len(cached))
		copy(out, cached)
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()

	s.probeLock.Lock()
	defer s.probeLock.Unlock()

	s.mu.RLock()
	if cached, ok := s.probeCache[agentType]; ok {
		out := make([]acp.SessionConfigOption, len(cached))
		copy(out, cached)
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()

	opts, err := s.probeConfigViaSession(ctx, agentType)
	if err != nil {
		return nil, err
	}
	out := make([]acp.SessionConfigOption, len(opts))
	copy(out, opts)
	return out, nil
}

func (s *Service) probeCwd() string {
	if s.wsConfig.SessionDir != "" {
		return s.wsConfig.SessionDir
	}
	return "/tmp"
}

// probeConfigViaSession 创建临时 ACP session 读取 ConfigOptions，不写入数据库、不发送 prompt。
func (s *Service) probeConfigViaSession(ctx context.Context, agentType string) ([]acp.SessionConfigOption, error) {
	probeCwd := s.probeCwd()
	conn, err := s.ensureConnection(ctx, agentType, probeCwd)
	if err != nil {
		return nil, err
	}
	sessionID, configOptions, modes, err := conn.NewSession(ctx, probeCwd, s.skillAdditionalDirs(probeCwd), nil, "")
	if err != nil {
		return nil, fmt.Errorf("探测 NewSession: %w", err)
	}
	defer func() { _ = conn.CloseSessionByID(ctx, sessionID) }()

	cmds := s.collectProbeCommands(ctx, conn, sessionID)

	out := make([]acp.SessionConfigOption, len(configOptions))
	copy(out, configOptions)

	s.mu.Lock()
	s.probeCache[agentType] = out
	if len(modes) > 0 {
		s.agentModes[agentType] = modes
	}
	if len(cmds) > 0 {
		s.agentCommands[agentType] = cmds
	}
	s.mu.Unlock()
	slog.Info("探测配置已缓存", "agent", agentType, "config_count", len(out), "modes", len(modes), "commands", len(cmds))
	return out, nil
}

// collectProbeCommands 在探测 session 上短暂等待 AvailableCommandsUpdate。
func (s *Service) collectProbeCommands(ctx context.Context, conn *Connection, sessionID string) []acp.AvailableCommand {
	sid := acp.SessionId(sessionID)
	ch := conn.Client().RegisterStream(sid, 8)
	defer conn.Client().UnregisterStream(sid)

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case u, ok := <-ch:
			if !ok {
				return nil
			}
			if u.AvailableCommandsUpdate != nil {
				return u.AvailableCommandsUpdate.AvailableCommands
			}
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
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

// skillAdditionalDirs 返回当前会话 cwd 应对 Agent 暴露的 skills/commands/rules 根目录。
func (s *Service) skillAdditionalDirs(cwd string) []string {
	return MergeAdditionalDirectories(
		SkillAdditionalDirectories(cwd, s.skillUserDirs, s.skillProjectDirs),
		SkillAdditionalDirectories(cwd, s.commandUserDirs, s.commandProjectDirs),
		RuleAdditionalDirectories(cwd, s.ruleUserDirs, s.ruleProjectDirs),
	)
}

// ListSkills 扫描会话工作目录和用户主目录下的 Agent Skills（agentskills.io 规范）。
func (s *Service) ListSkills(sessionID string) ([]Skill, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	cwd := session.Cwd
	if session.WorkspaceID != nil {
		if ws, wsErr := s.workspaces.FindByID(*session.WorkspaceID); wsErr == nil {
			cwd = ws.Cwd
		}
	}
	return ScanSkills(cwd, s.skillUserDirs, s.skillProjectDirs), nil
}

// SetConfigOption 设置会话的 config option 值（如切换模型）。
func (s *Service) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}
	conn, ok := s.connForSession(sessionID)
	if !ok {
		return ErrSessionNotActive
	}
	acpSID := agentSessionID(sess)
	s.debugLog(sess.ID, "set_config", acpSID, map[string]any{"config_id": configID, "value": value})
	return conn.SetConfigOption(ctx, acpSID, configID, value)
}

// SetSessionMode 切换会话模式（如 ask / agent / edit）。
func (s *Service) SetSessionMode(ctx context.Context, sessionID, modeID string) error {
	sess, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}
	conn, ok := s.connForSession(sessionID)
	if !ok {
		return ErrSessionNotActive
	}
	acpSID := agentSessionID(sess)
	s.debugLog(sess.ID, "set_mode", acpSID, map[string]any{"mode_id": modeID})
	return conn.SetSessionMode(ctx, acpSID, modeID)
}

// RespondPermission 提交用户对权限请求的响应。
func (s *Service) RespondPermission(sessionID, requestID, optionID string, cancelled bool) error {
	conn, ok := s.connForSession(sessionID)
	if !ok {
		return ErrSessionNotActive
	}
	return conn.Client().RespondPermission(requestID, optionID, cancelled)
}

// GetSessionByDBID 按数据库主键查询会话。
func (s *Service) GetSessionByDBID(id uint) (*models.Session, error) {
	sess, err := s.sessions.FindByID(id)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// ResumeSession 恢复或重开会话：在共享连接上新建 ACP session、注入历史上下文、更新 agent_session_id。
// active 且连接存在的会话直接返回；error 与 closed 状态均会尝试恢复。
func (s *Service) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error) {
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

	// 从 workspace 获取 cwd
	cwd := session.Cwd
	wsMode := ""
	if session.WorkspaceID != nil {
		if ws, wsErr := s.workspaces.FindByID(*session.WorkspaceID); wsErr == nil {
			cwd = ws.Cwd
			wsMode = ws.Mode
		}
	}
	if cwd == "" {
		return nil, errors.New("恢复会话需要工作目录，请提供有效的 workspace")
	}
	if err := EnsureWorkspaceDir(wsMode, cwd); err != nil {
		return nil, err
	}

	// 复用共享连接（不存在则自动建立）
	conn, err := s.ensureConnection(ctx, session.AgentType, cwd)
	if err != nil {
		return nil, fmt.Errorf("恢复会话-建立连接: %w", err)
	}

	oldAgentSID := session.AgentSessionID
	s.debugBindPending(session.AgentType, session.ID)
	newAgentSID, configOptions, modes, err := conn.NewSession(ctx, cwd, s.sessionAdditionalDirs(session, cwd), s.sessionMCPServers(session.UserID), s.rulesSystemPrompt(cwd))
	s.debugClearPending(session.AgentType)
	if err != nil {
		return nil, fmt.Errorf("恢复会话-创建 ACP 会话: %w", err)
	}

	// 查询历史消息并注入上下文
	history, _ := s.messages.FindByDBSessionID(session.ID)
	contextText := formatHistory(history)
	if contextText != "" {
		// 异步注入历史上下文，不等结果
		go func() {
			_, _ = conn.Prompt(ctx, newAgentSID, contextText)
		}()
	}

	// 更新 agent_session_id 和状态（closed_at 置空）；稳定 session_id 不变
	if err := s.sessions.UpdateAgentSessionID(session.ID, newAgentSID); err != nil {
		_ = conn.CloseSessionByID(ctx, newAgentSID)
		return nil, fmt.Errorf("恢复会话-更新 agent_session_id: %w", err)
	}
	if err := s.sessions.UpdateStatus(session.ID, models.SessionStatusActive, nil); err != nil {
		_ = conn.CloseSessionByID(ctx, newAgentSID)
		return nil, fmt.Errorf("恢复会话-更新状态: %w", err)
	}

	// 稳定 session_id 为键，更新路由与缓存
	poolKey := connectionKey(session.AgentType, cwd)
	s.mu.Lock()
	s.sessionPoolKey[sessionID] = poolKey
	if len(configOptions) > 0 {
		s.configs[sessionID] = configOptions
	}
	if len(modes) > 0 {
		s.modes[sessionID] = modes
	}
	s.mu.Unlock()
	if oldAgentSID != "" {
		s.debugUnregister(oldAgentSID)
	}
	s.debugRegister(newAgentSID, session.ID)
	s.debugLog(session.ID, "resume_session", newAgentSID, map[string]any{
		"agent": session.AgentType, "cwd": cwd,
	})

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
// status=connected 表示该 agent 类型至少有一条 ACP 连接已建立且进程存活。
func (s *Service) ListAgentStatus() []AgentStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int, len(s.backends))
	for _, poolKey := range s.sessionPoolKey {
		agentType, _ := splitConnectionKey(poolKey)
		counts[agentType]++
	}

	agentState := make(map[string]string, len(s.backends))
	for poolKey, state := range s.states {
		agentType, _ := splitConnectionKey(poolKey)
		if state == connStateConnected {
			if conn, ok := s.pool[poolKey]; ok {
				select {
				case <-conn.Done():
					continue
				default:
					agentState[agentType] = connStateConnected
				}
			}
			continue
		}
		if state == connStateConnecting && agentState[agentType] != connStateConnected {
			agentState[agentType] = connStateConnecting
		}
	}

	out := make([]AgentStatus, 0, len(s.backends))
	for name := range s.backends {
		state := agentState[name]
		if state == "" {
			state = connStateDisconnected
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
// sessionID 为稳定内部 ID（查连接路由），agentSID 为 ACP sessionId。
func (s *Service) applyModelValue(ctx context.Context, sessionID, agentSID string, opts []acp.SessionConfigOption, modelValue string) error {
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
		return conn.SetConfigOption(ctx, agentSID, string(opt.Select.Id), modelValue)
	}
	return nil // 该 agent 无 model config option，静默跳过
}

// logWarn 统一警告日志输出。
func (s *Service) logWarn(msg, agent string) {
	slog.Warn(msg, "agent", agent)
}

// backendCommandSafe 返回指定 agent 后端的命令字符串，仅用于日志展示。
// 后端不存在或读取失败时返回空串，绝不返回 error，避免污染调用方日志。
func (s *Service) backendCommandSafe(agentType string) string {
	b, err := s.GetBackend(agentType)
	if err != nil {
		return ""
	}
	return b.Command()
}

// Workspace delegation methods

func (s *Service) GetWorkspaceCwd(workspaceID uint) (string, error) {
	ws, err := s.workspaces.FindByID(workspaceID)
	if err != nil {
		return "", err
	}
	return ws.Cwd, nil
}

func (s *Service) CreateWorkspace(ws *models.Workspace) error {
	return s.workspaces.Create(ws)
}

func (s *Service) FindWorkspaceByID(id uint) (*models.Workspace, error) {
	return s.workspaces.FindByID(id)
}

func (s *Service) FindWorkspacesByUserID(userID uint) ([]models.Workspace, error) {
	return s.workspaces.FindByUserID(userID)
}

func (s *Service) FindWorkspaceByUserIDAndCwd(userID uint, cwd string) (*models.Workspace, error) {
	return s.workspaces.FindByUserIDAndCwd(userID, cwd)
}

func (s *Service) FindDefaultWorkspaceByUserID(userID uint) (*models.Workspace, error) {
	return s.workspaces.FindDefaultByUserID(userID)
}

func (s *Service) UpdateWorkspace(id uint, updates map[string]interface{}) error {
	return s.workspaces.Update(id, updates)
}

func (s *Service) DeleteWorkspace(id uint) error {
	ws, err := s.workspaces.FindByID(id)
	if err != nil {
		return err
	}
	if err := s.workspaces.Delete(id); err != nil {
		return err
	}
	return (&Workspace{Mode: ws.Mode, Cwd: ws.Cwd, TempDir: ws.TempDir}).Cleanup()
}

func (s *Service) WorkspaceSessionCount(workspaceID uint) (int64, error) {
	return s.workspaces.SessionCount(workspaceID)
}

func (s *Service) FindSessionsByWorkspaceID(workspaceID uint) ([]models.Session, error) {
	return s.sessions.FindByWorkspaceID(workspaceID)
}

func (s *Service) DeleteSessionWithMessages(session *models.Session) error {
	if err := s.messages.DeleteByDBSessionID(session.ID); err != nil {
		return err
	}
	if err := s.sessions.Delete(session.ID); err != nil {
		return err
	}
	s.debugCleanup(session.ID)
	return nil
}
