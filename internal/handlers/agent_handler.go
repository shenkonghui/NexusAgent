package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
)

// AgentLister 暴露 agent 列表查询能力（*agent.Router 实现该接口）。
type AgentLister interface {
	ListAgents() []*agent.AgentDescriptor
}

// AgentHandler 处理 agent 列表相关请求。
type AgentHandler struct {
	lister AgentLister
}

// NewAgentHandler 创建 AgentHandler。
func NewAgentHandler(lister AgentLister) *AgentHandler {
	return &AgentHandler{lister: lister}
}

// agentItem 是对外暴露的 agent 描述（隐藏 Backend 等内部字段）。
type agentItem struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

// List GET /api/v1/agents — 列出可用 agent 类型。
func (h *AgentHandler) List(c *gin.Context) {
	descs := h.lister.ListAgents()
	items := make([]agentItem, 0, len(descs))
	for _, d := range descs {
		items = append(items, agentItem{
			Type:        d.Type,
			DisplayName: d.DisplayName,
			Description: d.Description,
		})
	}
	Success(c, http.StatusOK, gin.H{"agents": items})
}
