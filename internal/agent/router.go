package agent

import (
	"context"
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"

	"nexusagent/internal/acp"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

// Router 路由用户请求到对应的 agent 后端，委托 acp.Service 执行。
type Router struct {
	registry *Registry
	service  *acp.Service
}

// NewRouter 创建新的 Router。
func NewRouter(registry *Registry, service *acp.Service) *Router {
	return &Router{
		registry: registry,
		service:  service,
	}
}

// CreateSession 创建会话：校验 agentType 后委托 service。
// modelValue 非空时在会话创建后立即设置该模型。
func (r *Router) CreateSession(ctx context.Context, agentType string, workspaceID uint, userID uint, modelValue string) (*models.Session, error) {
	return r.CreateSessionWithSource(ctx, agentType, workspaceID, userID, models.SessionSourceManual, modelValue)
}

// CreateSessionWithSource 创建会话并指定来源（manual/scheduled）。
// modelValue 非空时在会话创建后立即设置该模型。
func (r *Router) CreateSessionWithSource(ctx context.Context, agentType string, workspaceID uint, userID uint, source, modelValue string) (*models.Session, error) {
	if _, err := r.registry.Get(agentType); err != nil {
		return nil, err
	}
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.CreateSessionWithSource(ctx, agentType, workspaceID, userID, source, modelValue)
}

// ResumeSession 恢复或重开会话，委托 service。
func (r *Router) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ResumeSession(ctx, sessionID)
}

// ListAgents 返回所有已注册的 agent。
func (r *Router) ListAgents() []*AgentDescriptor {
	return r.registry.List()
}

// ListCommands 返回会话缓存的可用 slash command 列表，委托 service。
func (r *Router) ListCommands(sessionID string) ([]acpsdk.AvailableCommand, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListCommands(sessionID)
}

// ListConfigOptions 返回会话缓存的 config option 列表，委托 service。
func (r *Router) ListConfigOptions(sessionID string) ([]acpsdk.SessionConfigOption, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListConfigOptions(sessionID)
}

// CachedModelOptions 返回指定 agent 类型的可用模型 config option，委托 service。
func (r *Router) CachedModelOptions(agentType string) []acpsdk.SessionConfigOption {
	if r.service == nil {
		return nil
	}
	return r.service.CachedModelOptions(agentType)
}

// CachedCommands 返回指定 agent 类型缓存的 slash command，委托 service。
func (r *Router) CachedCommands(agentType string, cwd string) []acpsdk.AvailableCommand {
	if r.service == nil {
		return nil
	}
	return r.service.CachedCommands(agentType, cwd)
}

// CachedModes 返回指定 agent 类型缓存的 session mode，委托 service。
func (r *Router) CachedModes(agentType string) []acpsdk.SessionMode {
	if r.service == nil {
		return nil
	}
	return r.service.CachedModes(agentType)
}

// ProbeConfigOptions 创建临时会话探测指定 agent 类型的 config options，随后删除，委托 service。
func (r *Router) ProbeConfigOptions(ctx context.Context, agentType string, userID uint) ([]acpsdk.SessionConfigOption, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ProbeConfigOptions(ctx, agentType, userID)
}

// ListModes 返回会话可用的 mode 列表（agent skill/模式），委托 service。
func (r *Router) ListModes(sessionID string) ([]acpsdk.SessionMode, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListModes(sessionID)
}

// ListSkills 扫描会话工作目录下的 Agent Skills（agentskills.io 规范），委托 service。
func (r *Router) ListSkills(sessionID string) ([]acp.Skill, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListSkills(sessionID)
}

// SetConfigOption 设置会话的 config option 值，委托 service。
func (r *Router) SetConfigOption(ctx context.Context, sessionID, configID, value string) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.SetConfigOption(ctx, sessionID, configID, value)
}

// UpdateTitle 更新会话标题，委托 service。
func (r *Router) UpdateTitle(dbSessionID uint, title string) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.UpdateTitle(dbSessionID, title)
}

// ListAgentStatus 返回所有 agent 类型的连接状态，委托 service。
func (r *Router) ListAgentStatus() []acp.AgentStatus {
	if r.service == nil {
		return nil
	}
	return r.service.ListAgentStatus()
}

// ====== SessionStore delegation methods ======

func (r *Router) Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.Prompt(ctx, sessionID, prompt)
}

func (r *Router) PromptWithExecution(ctx context.Context, sessionID, prompt string, executionID *uint) (<-chan models.Message, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.PromptWithExecution(ctx, sessionID, prompt, executionID)
}

func (r *Router) ListSessionsBySource(userID uint, source string) ([]models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListSessionsBySource(userID, source)
}

func (r *Router) ListExecutions(sessionID string) ([]repository.ExecutionAggregate, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListExecutions(sessionID)
}

func (r *Router) NextExecutionID(sessionID string) (uint, error) {
	if r.service == nil {
		return 0, errors.New("service 未配置")
	}
	return r.service.NextExecutionID(sessionID)
}

func (r *Router) CancelSession(ctx context.Context, sessionID string) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.CancelSession(ctx, sessionID)
}

func (r *Router) DeleteSession(ctx context.Context, sessionID string) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.DeleteSession(ctx, sessionID)
}

func (r *Router) ListSessions(userID uint) ([]models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListSessions(userID)
}

func (r *Router) GetSessionByDBID(id uint) (*models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.GetSessionByDBID(id)
}

func (r *Router) ListMessages(sessionID string) ([]models.Message, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListMessages(sessionID)
}

func (r *Router) GetWorkspaceCwd(workspaceID uint) (string, error) {
	if r.service == nil {
		return "", errors.New("service 未配置")
	}
	return r.service.GetWorkspaceCwd(workspaceID)
}

// ====== WorkspaceStore delegation methods ======

func (r *Router) CreateWorkspace(ws *models.Workspace) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.CreateWorkspace(ws)
}

func (r *Router) FindWorkspaceByID(id uint) (*models.Workspace, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.FindWorkspaceByID(id)
}

func (r *Router) FindWorkspacesByUserID(userID uint) ([]models.Workspace, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.FindWorkspacesByUserID(userID)
}

func (r *Router) FindWorkspaceByUserIDAndCwd(userID uint, cwd string) (*models.Workspace, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.FindWorkspaceByUserIDAndCwd(userID, cwd)
}

func (r *Router) FindDefaultWorkspaceByUserID(userID uint) (*models.Workspace, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.FindDefaultWorkspaceByUserID(userID)
}

func (r *Router) UpdateWorkspace(id uint, updates map[string]interface{}) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.UpdateWorkspace(id, updates)
}

func (r *Router) DeleteWorkspace(id uint) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.DeleteWorkspace(id)
}

func (r *Router) WorkspaceSessionCount(workspaceID uint) (int64, error) {
	if r.service == nil {
		return 0, errors.New("service 未配置")
	}
	return r.service.WorkspaceSessionCount(workspaceID)
}

func (r *Router) FindSessionsByWorkspaceID(workspaceID uint) ([]models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.FindSessionsByWorkspaceID(workspaceID)
}

func (r *Router) DeleteSessionWithMessages(session *models.Session) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.DeleteSessionWithMessages(session)
}
