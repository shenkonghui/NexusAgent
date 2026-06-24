package agent

import (
	"errors"
	"sync"

	"nexusagent/internal/acp"
)

var ErrAgentNotFound = errors.New("agent 类型未注册")
var ErrAgentAlreadyRegistered = errors.New("agent 类型已注册")

// AgentDescriptor 描述一个已注册的 agent 类型。
type AgentDescriptor struct {
	Type        string
	DisplayName string
	Description string
	Backend     acp.Backend
}

// Registry 管理 agent 类型的注册与查询。
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentDescriptor
}

// NewRegistry 创建空的注册表。
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]*AgentDescriptor),
	}
}

// Register 注册一个 agent 类型。重复注册返回错误。
func (r *Registry) Register(desc *AgentDescriptor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[desc.Type]; exists {
		return ErrAgentAlreadyRegistered
	}
	r.agents[desc.Type] = desc
	return nil
}

// Get 查找指定类型的 agent 描述符。
func (r *Registry) Get(agentType string) (*AgentDescriptor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.agents[agentType]
	if !ok {
		return nil, ErrAgentNotFound
	}
	return desc, nil
}

// List 返回所有已注册的 agent 描述符。
func (r *Registry) List() []*AgentDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*AgentDescriptor, 0, len(r.agents))
	for _, desc := range r.agents {
		list = append(list, desc)
	}
	return list
}
