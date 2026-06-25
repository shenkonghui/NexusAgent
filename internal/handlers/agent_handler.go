package handlers

import (
	"net/http"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
)

// AgentLister 暴露 agent 列表查询能力（*agent.Router 实现该接口）。
type AgentLister interface {
	ListAgents() []*agent.AgentDescriptor
}

// AgentModelProber 返回指定 agent 类型的可用模型 config option（从已有会话缓存获取）。
type AgentModelProber interface {
	CachedModelOptions(agentType string) []acp.SessionConfigOption
}

// AgentHandler 处理 agent 列表相关请求。
type AgentHandler struct {
	lister AgentLister
	prober AgentModelProber
}

// NewAgentHandler 创建 AgentHandler。prober 可为 nil（不提供模型查询）。
func NewAgentHandler(lister AgentLister, prober AgentModelProber) *AgentHandler {
	return &AgentHandler{lister: lister, prober: prober}
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

// modelOptionItem 是对外暴露的模型 config option 描述。
type modelOptionItem struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	CurrentValue string              `json:"current_value"`
	Options      []configOptionValue `json:"options"`
}

// Models GET /api/v1/agents/:type/models — 返回指定 agent 类型的可用模型列表。
// 从已有会话缓存获取；若该 agent 类型尚无会话则返回空列表。
func (h *AgentHandler) Models(c *gin.Context) {
	agentType := strings.TrimSpace(c.Param("type"))
	if agentType == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "缺少 agent 类型")
		return
	}
	if h.prober == nil {
		Success(c, http.StatusOK, gin.H{"model_options": []modelOptionItem{}})
		return
	}
	opts := h.prober.CachedModelOptions(agentType)
	items := make([]modelOptionItem, 0, len(opts))
	for _, opt := range opts {
		if opt.Select == nil {
			continue
		}
		item := modelOptionItem{
			ID:           string(opt.Select.Id),
			Name:         opt.Select.Name,
			CurrentValue: string(opt.Select.CurrentValue),
		}
		if opt.Select.Options.Ungrouped != nil {
			for _, o := range *opt.Select.Options.Ungrouped {
				desc := ""
				if o.Description != nil {
					desc = *o.Description
				}
				item.Options = append(item.Options, configOptionValue{
					Value:       string(o.Value),
					Name:        o.Name,
					Description: desc,
				})
			}
		}
		if opt.Select.Options.Grouped != nil {
			for _, g := range *opt.Select.Options.Grouped {
				for _, o := range g.Options {
					desc := ""
					if o.Description != nil {
						desc = *o.Description
					}
					item.Options = append(item.Options, configOptionValue{
						Value:       string(o.Value),
						Name:        o.Name,
						Description: desc,
					})
				}
			}
		}
		items = append(items, item)
	}
	Success(c, http.StatusOK, gin.H{"model_options": items})
}
