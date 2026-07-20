package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"opennexus/internal/acp"
)

// RegistryHandler 提供在线拉取 ACP 官方 registry 并合并到本地存储的能力。
type RegistryHandler struct {
	store acp.AgentConfigSyncer
}

// NewRegistryHandler 创建 RegistryHandler。store 通常为 *repository.AgentConfigRepository。
func NewRegistryHandler(store acp.AgentConfigSyncer) *RegistryHandler {
	return &RegistryHandler{store: store}
}

// registryRefreshResponse 是 Refresh 的返回结构。
type registryRefreshResponse struct {
	Version string `json:"version"` // registry 顶层 version
	Total   int    `json:"total"`   // registry 中 agent 总数
	Added   int    `json:"added"`   // 本次新增入库的 agent 数
	Updated int    `json:"updated"` // 本次更新（仅名称/描述）的 agent 数
}

// Refresh POST /api/v1/agent-configs/registry/refresh
// 从 https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json 拉取最新 registry，
// 合并到本地存储：
//   - 新 agent：以 enabled=false 创建（不自动注册，等待用户在设置中启用）
//   - 已有 agent：仅更新名称/描述，保留用户修改的 command/args/env/enabled
//
// 对正在运行的后端/registrar 零影响。
func (h *RegistryHandler) Refresh(c *gin.Context) {
	doc, err := acp.FetchRegistryFull()
	if err != nil {
		slog.Warn("在线拉取 registry 失败", "err", err)
		Fail(c, http.StatusBadGateway, "REGISTRY_FETCH_FAILED", "拉取注册表失败: "+err.Error())
		return
	}

	added, updated, err := acp.SyncRegistryToStore(doc.Agents, h.store)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "REGISTRY_SYNC_FAILED", err.Error())
		return
	}

	Success(c, http.StatusOK, registryRefreshResponse{
		Version: doc.Version,
		Total:   len(doc.Agents),
		Added:   added,
		Updated: updated,
	})
}
