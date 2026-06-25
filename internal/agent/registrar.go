package agent

import (
	"nexusagent/internal/acp"
)

// Registrar 组合 Registry 与 acp.Service 的后端注册能力，
// 实现 handlers.AgentRegistrar 接口，用于动态管理 agent 配置。
type Registrar struct {
	*Registry
	backendRegistrar backendRegistrar
}

// backendRegistrar 是 acp.Service 暴露的后端注册子集。
type backendRegistrar interface {
	RegisterBackend(b acp.Backend)
	ReplaceBackend(b acp.Backend)
	UnregisterBackend(name string)
}

// NewRegistrar 创建组合 registrar。
func NewRegistrar(registry *Registry, service *acp.Service) *Registrar {
	return &Registrar{Registry: registry, backendRegistrar: service}
}

// RegisterBackend 委托给后端注册器。
func (r *Registrar) RegisterBackend(b acp.Backend) {
	r.backendRegistrar.RegisterBackend(b)
}

// ReplaceBackend 委托给后端注册器。
func (r *Registrar) ReplaceBackend(b acp.Backend) {
	r.backendRegistrar.ReplaceBackend(b)
}

// UnregisterBackend 委托给后端注册器。
func (r *Registrar) UnregisterBackend(name string) {
	r.backendRegistrar.UnregisterBackend(name)
}
