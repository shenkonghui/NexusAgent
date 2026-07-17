package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

// AgentPrefsHandler 处理用户 agent 最近使用偏好。
type AgentPrefsHandler struct {
	repo *repository.UserAgentPrefsRepository
}

func NewAgentPrefsHandler(repo *repository.UserAgentPrefsRepository) *AgentPrefsHandler {
	return &AgentPrefsHandler{repo: repo}
}

type agentPrefsItem struct {
	LastAgentType string                       `json:"last_agent_type"`
	Prefs         map[string]map[string]string `json:"prefs"`
}

type agentPrefsPatchRequest struct {
	LastAgentType *string           `json:"last_agent_type"`
	AgentType     string            `json:"agent_type"`
	Configs       map[string]string `json:"configs"`
}

func (h *AgentPrefsHandler) toItem(s *models.UserAgentPrefs) agentPrefsItem {
	prefs := map[string]map[string]string{}
	raw := strings.TrimSpace(s.PrefsJSON)
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &prefs)
	}
	if prefs == nil {
		prefs = map[string]map[string]string{}
	}
	return agentPrefsItem{LastAgentType: s.LastAgentType, Prefs: prefs}
}

// Get GET /api/v1/agent-prefs
func (h *AgentPrefsHandler) Get(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	s, err := h.repo.FindByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询偏好失败")
		return
	}
	Success(c, http.StatusOK, h.toItem(s))
}

// Patch PATCH /api/v1/agent-prefs
func (h *AgentPrefsHandler) Patch(c *gin.Context) {
	var req agentPrefsPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	s, err := h.repo.Patch(uid, req.LastAgentType, req.AgentType, req.Configs)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "保存偏好失败")
		return
	}
	Success(c, http.StatusOK, h.toItem(s))
}
