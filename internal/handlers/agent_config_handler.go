package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/acp"
	"nexusagent/internal/agent"
	"nexusagent/internal/models"
)

// AgentConfigStore 暴露 AgentConfig 持久化能力。
type AgentConfigStore interface {
	FindAll() ([]models.AgentConfig, error)
	FindByID(id uint) (*models.AgentConfig, error)
	FindByType(agentType string) (*models.AgentConfig, error)
	Create(cfg *models.AgentConfig) error
	Update(cfg *models.AgentConfig) error
	Delete(id uint) error
}

// AgentRegistrar 暴露 agent 动态注册/注销能力。
type AgentRegistrar interface {
	RegisterBackend(b acp.Backend)
	ReplaceBackend(b acp.Backend)
	UnregisterBackend(name string)
	RegisterAgent(desc *agent.AgentDescriptor) error
	ReplaceAgent(desc *agent.AgentDescriptor)
	UnregisterAgent(agentType string)
	PreconnectAgent(agentType string)
}

// AgentConfigHandler 处理 agent 配置的 CRUD 请求。
type AgentConfigHandler struct {
	store    AgentConfigStore
	registrar AgentRegistrar
}

// NewAgentConfigHandler 创建 AgentConfigHandler。
func NewAgentConfigHandler(store AgentConfigStore, registrar AgentRegistrar) *AgentConfigHandler {
	return &AgentConfigHandler{store: store, registrar: registrar}
}

type agentConfigRequest struct {
	Type        string `json:"type" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	Description string `json:"description"`
	Command     string `json:"command" binding:"required"`
	Args        []string `json:"args"`
	Env         map[string]string `json:"env"`
	APIKeyEnv   string `json:"api_key_env"`
	Timeout     string `json:"timeout"`
	Enabled     *bool  `json:"enabled"`
}

// agentConfigItem 是对外暴露的 agent 配置。
type agentConfigItem struct {
	ID          uint     `json:"id"`
	Type        string   `json:"type"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Env         map[string]string `json:"env"`
	APIKeyEnv   string   `json:"api_key_env"`
	Timeout     string   `json:"timeout"`
	Enabled     bool     `json:"enabled"`
}

func toAgentConfigItem(cfg *models.AgentConfig) agentConfigItem {
	var args []string
	if cfg.Args != "" {
		_ = json.Unmarshal([]byte(cfg.Args), &args)
	}
	envMap := parseEnvMap(cfg.Env)
	enabled := false
	if cfg.Enabled != nil {
		enabled = *cfg.Enabled
	}
	return agentConfigItem{
		ID:          cfg.ID,
		Type:        cfg.Type,
		DisplayName: cfg.DisplayName,
		Description: cfg.Description,
		Command:     cfg.Command,
		Args:        args,
		Env:         envMap,
		APIKeyEnv:   cfg.APIKeyEnv,
		Timeout:     cfg.Timeout,
		Enabled:     enabled,
	}
}

// parseArgs 将请求中的 args 编码为 JSON 字符串。
func parseArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	b, _ := json.Marshal(args)
	return string(b)
}

// parseEnv 将请求中的 env map 编码为 JSON 字符串，空 map 返回空串。
func parseEnv(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	b, _ := json.Marshal(env)
	return string(b)
}

// parseEnvMap 将存储的 JSON 字符串解码为 map，空串或非法 JSON 返回 nil。
func parseEnvMap(env string) map[string]string {
	if env == "" {
		return nil
	}
	var m map[string]string
	_ = json.Unmarshal([]byte(env), &m)
	return m
}

// validateTimeout 校验 timeout 字符串可被 time.ParseDuration 解析（空串合法）。
func validateTimeout(timeout string) error {
	if timeout == "" {
		return nil
	}
	if _, err := time.ParseDuration(timeout); err != nil {
		return errors.New("timeout 格式无效，应为如 300s / 5m 的时长字符串")
	}
	return nil
}

// applyToRegistrar 根据 cfg 在 registrar 中注册/替换或注销。
func (h *AgentConfigHandler) applyToRegistrar(cfg *models.AgentConfig) {
	if cfg.Enabled == nil || !*cfg.Enabled {
		h.registrar.UnregisterAgent(cfg.Type)
		h.registrar.UnregisterBackend(cfg.Type)
		return
	}
	h.registrar.ReplaceBackend(acp.NewConfigBackend(*cfg))
	h.registrar.ReplaceAgent(&agent.AgentDescriptor{
		Type:        cfg.Type,
		DisplayName: cfg.DisplayName,
		Description: cfg.Description,
		Backend:     acp.NewConfigBackend(*cfg),
	})
	h.registrar.PreconnectAgent(cfg.Type)
}

// List GET /api/v1/agent-configs
func (h *AgentConfigHandler) List(c *gin.Context) {
	list, err := h.store.FindAll()
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	items := make([]agentConfigItem, 0, len(list))
	for i := range list {
		items = append(items, toAgentConfigItem(&list[i]))
	}
	Success(c, http.StatusOK, gin.H{"agent_configs": items})
}

// Create POST /api/v1/agent-configs
func (h *AgentConfigHandler) Create(c *gin.Context) {
	var req agentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	if err := validateTimeout(req.Timeout); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_TIMEOUT", err.Error())
		return
	}
	// 检查 type 是否已存在
	if existing, err := h.store.FindByType(req.Type); err == nil && existing != nil {
		Fail(c, http.StatusConflict, "AGENT_TYPE_EXISTS", "agent 类型已存在")
		return
	}
	cfg := &models.AgentConfig{
		Type:        req.Type,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Command:     req.Command,
		Args:        parseArgs(req.Args),
		Env:         parseEnv(req.Env),
		APIKeyEnv:   req.APIKeyEnv,
		Timeout:     req.Timeout,
		Enabled:     req.Enabled,
	}
	if err := h.store.Create(cfg); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	h.applyToRegistrar(cfg)
	Success(c, http.StatusCreated, toAgentConfigItem(cfg))
}

// Update PUT /api/v1/agent-configs/:id
func (h *AgentConfigHandler) Update(c *gin.Context) {
	id, ok := parseAgentConfigID(c)
	if !ok {
		return
	}
	var req agentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	if err := validateTimeout(req.Timeout); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_TIMEOUT", err.Error())
		return
	}
	cfg, err := h.store.FindByID(id)
	if err != nil {
		Fail(c, http.StatusNotFound, "AGENT_CONFIG_NOT_FOUND", "agent 配置不存在")
		return
	}
	// 若修改了 type，需校验新 type 不与其它记录冲突
	if req.Type != cfg.Type {
		if existing, err := h.store.FindByType(req.Type); err == nil && existing != nil && existing.ID != id {
			Fail(c, http.StatusConflict, "AGENT_TYPE_EXISTS", "agent 类型已存在")
			return
		}
		// 旧 type 注销
		h.registrar.UnregisterAgent(cfg.Type)
		h.registrar.UnregisterBackend(cfg.Type)
	}
	cfg.Type = req.Type
	cfg.DisplayName = req.DisplayName
	cfg.Description = req.Description
	cfg.Command = req.Command
	cfg.Args = parseArgs(req.Args)
	cfg.Env = parseEnv(req.Env)
	cfg.APIKeyEnv = req.APIKeyEnv
	cfg.Timeout = req.Timeout
	cfg.Enabled = req.Enabled
	if err := h.store.Update(cfg); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	h.applyToRegistrar(cfg)
	Success(c, http.StatusOK, toAgentConfigItem(cfg))
}

// Delete DELETE /api/v1/agent-configs/:id
func (h *AgentConfigHandler) Delete(c *gin.Context) {
	id, ok := parseAgentConfigID(c)
	if !ok {
		return
	}
	cfg, err := h.store.FindByID(id)
	if err != nil {
		Fail(c, http.StatusNotFound, "AGENT_CONFIG_NOT_FOUND", "agent 配置不存在")
		return
	}
	if err := h.store.Delete(id); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	h.registrar.UnregisterAgent(cfg.Type)
	h.registrar.UnregisterBackend(cfg.Type)
	Success(c, http.StatusOK, struct{}{})
}

// parseAgentConfigID 解析 :id（uint，>0）。
func parseAgentConfigID(c *gin.Context) (uint, bool) {
	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "无效的 agent 配置 ID")
		return 0, false
	}
	return uint(id), true
}
