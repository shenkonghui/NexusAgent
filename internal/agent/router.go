package agent

import (
	"context"
	"errors"

	"nexusagent/internal/acp"
	"nexusagent/internal/models"
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
func (r *Router) CreateSession(ctx context.Context, agentType, cwd string, userID uint) (*models.Session, error) {
	if _, err := r.registry.Get(agentType); err != nil {
		return nil, err
	}
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.CreateSession(ctx, agentType, cwd, userID)
}

// Prompt 发送 prompt，委托 service。
func (r *Router) Prompt(ctx context.Context, sessionID, prompt string) (<-chan interface{}, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.Prompt(ctx, sessionID, prompt)
}

// CancelSession 取消会话，委托 service。
func (r *Router) CancelSession(ctx context.Context, sessionID string) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.CancelSession(ctx, sessionID)
}

// CloseSession 关闭会话，委托 service。
func (r *Router) CloseSession(ctx context.Context, sessionID string) error {
	if r.service == nil {
		return errors.New("service 未配置")
	}
	return r.service.CloseSession(ctx, sessionID)
}

// ListSessions 列出用户会话，委托 service。
func (r *Router) ListSessions(userID uint) ([]models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListSessions(userID)
}

// ListAgents 返回所有已注册的 agent。
func (r *Router) ListAgents() []*AgentDescriptor {
	return r.registry.List()
}
