package handlers

import (
	"context"
	"net/http"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/gin-gonic/gin"

	acplocal "nexusagent/internal/acp"
	"nexusagent/internal/agent"
)

// AgentLister 暴露 agent 列表查询能力（*agent.Router 实现该接口）。
type AgentLister interface {
	ListAgents() []*agent.AgentDescriptor
}

// AgentStatusLister 暴露 agent 连接状态查询能力。
type AgentStatusLister interface {
	ListAgentStatus() []acplocal.AgentStatus
}

// AgentModelProber 返回指定 agent 类型的可用模型 config option（从已有会话缓存获取）。
type AgentModelProber interface {
	CachedModelOptions(agentType string) []acpsdk.SessionConfigOption
}

// AgentConfigProber 创建临时会话探测指定 agent 类型的全部 config options，随后删除该会话。
type AgentConfigProber interface {
	ProbeConfigOptions(ctx context.Context, agentType string, userID uint) ([]acpsdk.SessionConfigOption, error)
}

// AgentCommandLister 返回指定 agent 类型缓存的 slash command。
type AgentCommandLister interface {
	CachedCommands(agentType string, cwd string) []acpsdk.AvailableCommand
	ListConfiguredCommands(cwd string) []acplocal.SlashCommand
}

// AgentModeLister 返回指定 agent 类型缓存的 session mode。
type AgentModeLister interface {
	CachedModes(agentType string) []acpsdk.SessionMode
}

// AgentHandler 处理 agent 列表相关请求。
type AgentHandler struct {
	lister         AgentLister
	prober         AgentModelProber
	cfgProber      AgentConfigProber
	cmdLister      AgentCommandLister
	modeLister     AgentModeLister
	statusLister   AgentStatusLister
}

// NewAgentHandler 创建 AgentHandler。各依赖可为 nil。
func NewAgentHandler(lister AgentLister, prober AgentModelProber, cfgProber AgentConfigProber, statusLister AgentStatusLister) *AgentHandler {
	h := &AgentHandler{lister: lister, prober: prober, cfgProber: cfgProber, statusLister: statusLister}
	if cl, ok := lister.(AgentCommandLister); ok {
		h.cmdLister = cl
	}
	if ml, ok := lister.(AgentModeLister); ok {
		h.modeLister = ml
	}
	return h
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

// Status GET /api/v1/agents/status — 返回所有 agent 类型的 ACP 连接状态。
func (h *AgentHandler) Status(c *gin.Context) {
	if h.statusLister == nil {
		Success(c, http.StatusOK, gin.H{"agents": []agentItem{}})
		return
	}
	statuses := h.statusLister.ListAgentStatus()
	Success(c, http.StatusOK, gin.H{"agents": statuses})
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

// Probe POST /api/v1/agents/:type/probe — 创建临时会话探测该 agent 的全部 config options，随后删除。
// 返回与 GET /sessions/:id/config-options 相同结构的 config_options 列表（含模型及其他配置）。
func (h *AgentHandler) Probe(c *gin.Context) {
	agentType := strings.TrimSpace(c.Param("type"))
	if agentType == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "缺少 agent 类型")
		return
	}
	if h.cfgProber == nil {
		Fail(c, http.StatusServiceUnavailable, "PROBE_UNAVAILABLE", "当前服务不支持配置探测")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	opts, err := h.cfgProber.ProbeConfigOptions(c.Request.Context(), agentType, uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "PROBE_FAILED", err.Error())
		return
	}
	items := make([]configOptionItem, 0, len(opts))
	for _, opt := range opts {
		item := configOptionItem{Type: "boolean"}
		if opt.Select != nil {
			item.ID = string(opt.Select.Id)
			item.Name = opt.Select.Name
			item.Type = "select"
			item.CurrentValue = string(opt.Select.CurrentValue)
			if opt.Select.Category != nil {
				item.Category = string(*opt.Select.Category)
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
		}
		items = append(items, item)
	}
	Success(c, http.StatusOK, gin.H{"config_options": items})
}

// Commands GET /api/v1/agents/:type/commands — 返回 agent 类型缓存的 slash command（新建任务页用）。
func (h *AgentHandler) Commands(c *gin.Context) {
	agentType := strings.TrimSpace(c.Param("type"))
	if agentType == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "缺少 agent 类型")
		return
	}
	if h.cmdLister == nil {
		Success(c, http.StatusOK, gin.H{"commands": []commandItem{}})
		return
	}
	cmds := h.cmdLister.CachedCommands(agentType, strings.TrimSpace(c.Query("path")))
	configured := h.cmdLister.ListConfiguredCommands(strings.TrimSpace(c.Query("path")))
	items := buildCommandItems(cmds, configured)
	Success(c, http.StatusOK, gin.H{"commands": items})
}

// Modes GET /api/v1/agents/:type/modes — 返回 agent 类型缓存的 session mode（新建任务页用）。
func (h *AgentHandler) Modes(c *gin.Context) {
	agentType := strings.TrimSpace(c.Param("type"))
	if agentType == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "缺少 agent 类型")
		return
	}
	if h.modeLister == nil {
		Success(c, http.StatusOK, gin.H{"modes": []modeItem{}})
		return
	}
	modes := h.modeLister.CachedModes(agentType)
	items := make([]modeItem, 0, len(modes))
	for _, m := range modes {
		desc := ""
		if m.Description != nil {
			desc = *m.Description
		}
		items = append(items, modeItem{
			ID:          string(m.Id),
			Name:        m.Name,
			Description: desc,
		})
	}
	Success(c, http.StatusOK, gin.H{"modes": items})
}
