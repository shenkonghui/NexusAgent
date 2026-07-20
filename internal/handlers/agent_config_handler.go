package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
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
// 用 NewBackendFromAgentConfig 而非 NewConfigBackend:对 binary 类 agent(如 amp-acp、crow-cli)
// 走 BinaryBackend 的延迟下载 + 缓存绝对路径，与启动流程一致。
func (h *AgentConfigHandler) applyToRegistrar(cfg *models.AgentConfig) {
	if cfg.Enabled == nil || !*cfg.Enabled {
		h.registrar.UnregisterAgent(cfg.Type)
		h.registrar.UnregisterBackend(cfg.Type)
		return
	}
	h.registrar.ReplaceBackend(acp.NewBackendFromAgentConfig(*cfg))
	h.registrar.ReplaceAgent(&agent.AgentDescriptor{
		Type:        cfg.Type,
		DisplayName: cfg.DisplayName,
		Description: cfg.Description,
		Backend:     acp.NewBackendFromAgentConfig(*cfg),
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

// registryDefaultResponse 是 GetRegistryDefault 的返回结构，仅含可重置字段。
type registryDefaultResponse struct {
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	ArchiveURL  string   `json:"archive_url"` // binary 类 agent 才有；npx/uvx 为空
}

// resolveRegistryAgent 按 agentType 查 registry：优先 CDN 最新版，失败回退内嵌。
// 返回的 ra 非空时，caller 可安全调用 ToAgentConfig（会触发 binary 类写入 BinaryRegistry 的副作用）。
// CDN 与内嵌都找不到返回 (nil, "", nil)；CDN 网络错但内嵌能找到则用内嵌。
func resolveRegistryAgent(agentType string) (ra *acp.RegistryAgent, source string, err error) {
	online, onlineErr := acp.FindOnlineRegistryAgent(agentType)
	if online != nil {
		return online, "cdn", nil
	}
	// CDN 没找到（可能网络错误，也可能确实没有）→ 回退内嵌
	embedded, embErr := acp.FindEmbeddedRegistryAgent(agentType)
	if embedded != nil {
		if onlineErr != nil {
			slog.Warn("CDN 查询 agent 失败，回退到内嵌 registry", "type", agentType, "err", onlineErr)
		}
		return embedded, "embedded", nil
	}
	// 两个都没有
	if embErr != nil {
		return nil, "", embErr
	}
	return nil, "", nil
}

// GetRegistryDefault GET /api/v1/agent-configs/:id/registry-default
// 返回该 agent 在 registry 中的默认 command/args（供前端"重置为默认"按钮预填表单）。
// 数据源：CDN 优先，失败回退内嵌。env 不返回（敏感，不重置）。
// 注意：调用 ToAgentConfig 会触发 buildCommand 的副作用（binary 类写入 BinaryRegistry），
// 这使后续 PUT 保存时 applyToRegistrar→NewBackendFromAgentConfig 能正确选择 BinaryBackend。
func (h *AgentConfigHandler) GetRegistryDefault(c *gin.Context) {
	id, ok := parseAgentConfigID(c)
	if !ok {
		return
	}
	cfg, err := h.store.FindByID(id)
	if err != nil {
		Fail(c, http.StatusNotFound, "AGENT_CONFIG_NOT_FOUND", "agent 配置不存在")
		return
	}
	ra, _, err := resolveRegistryAgent(cfg.Type)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "REGISTRY_LOAD_ERROR", "加载 registry 失败: "+err.Error())
		return
	}
	if ra == nil {
		Fail(c, http.StatusNotFound, "REGISTRY_AGENT_NOT_FOUND", "该 agent 不在内置 registry 中，无法重置")
		return
	}
	def, err := ra.ToAgentConfig()
	if err != nil {
		Fail(c, http.StatusInternalServerError, "REGISTRY_BUILD_ERROR", "构建 registry 默认值失败: "+err.Error())
		return
	}
	Success(c, http.StatusOK, registryDefaultResponse{
		Command:     def.Command,
		Args:        parseArgsFromJSON(def.Args),
		DisplayName: def.DisplayName,
		Description: def.Description,
		Version:     ra.Version,
		ArchiveURL:  ra.Distribution.BinaryArchive,
	})
}

// updateFromRegistryResponse 是 UpdateFromRegistry 的返回结构。
type updateFromRegistryResponse struct {
	Version       string   `json:"version"`
	Command       string   `json:"command"`
	Args          []string `json:"args"`
	Redownloaded  bool     `json:"redownloaded"`  // binary agent 是否清除了旧缓存（下次启动重下）
	Source        string   `json:"source"`        // "cdn" | "embedded"
}

// UpdateFromRegistry POST /api/v1/agent-configs/:id/update-from-registry
// 从 CDN 最新 registry 同步单个 agent：原子完成"拉取→比对→(可能重下)→更新配置→重新注册"。
//  - command/args/display_name/description 覆盖为 registry 最新值
//  - env/enabled 保留（env 常含代理/密钥；enabled 是用户意愿）
//  - binary 类 agent：若 version 或 archiveURL 变化，清除旧缓存（下次 Prepare 重新下载）
//  - npx/uvx 类 agent：无下载步骤，npm/uvx 每次启动自动拉最新包，仅更新配置
func (h *AgentConfigHandler) UpdateFromRegistry(c *gin.Context) {
	id, ok := parseAgentConfigID(c)
	if !ok {
		return
	}
	cfg, err := h.store.FindByID(id)
	if err != nil {
		Fail(c, http.StatusNotFound, "AGENT_CONFIG_NOT_FOUND", "agent 配置不存在")
		return
	}
	ra, source, err := resolveRegistryAgent(cfg.Type)
	if err != nil {
		Fail(c, http.StatusBadGateway, "REGISTRY_FETCH_FAILED", "拉取 registry 失败: "+err.Error())
		return
	}
	if ra == nil {
		Fail(c, http.StatusNotFound, "REGISTRY_AGENT_NOT_FOUND", "该 agent 不在内置 registry 中，无法更新")
		return
	}

	def, err := ra.ToAgentConfig()
	if err != nil {
		Fail(c, http.StatusInternalServerError, "REGISTRY_BUILD_ERROR", "构建 registry 默认值失败: "+err.Error())
		return
	}

	// binary 类 agent：先预下载验证新版本可用，成功才切换 symlink 激活；失败则保留旧版不动。
	// EnsureBinaryDownloaded 成功后会自动切换 symlink + 记录 versions.json，使重启后仍用新版。
	// 这样即使新 version 的 URL 下不下来（如 403），agent 仍能用 symlink 指向的旧版工作，不会变砖。
	// npx/uvx 类无此步骤（npm/uvx 每次启动自动拉最新包）。
	redownloaded := false
	if ra.Distribution.Type == "binary" {
		if dlErr := acp.EnsureBinaryDownloaded(cfg.Type, ra.Version, ra.Distribution.BinaryArchive, ra.Distribution.BinaryCmd); dlErr != nil {
			// 新版下载失败：保留 symlink 指向的旧版，回退 version 供响应展示
			slog.Warn("新版本预下载失败，保留旧版", "agent", cfg.Type, "newVersion", ra.Version, "err", dlErr)
			if old, ok := acp.BinaryRegistry[cfg.Type]; ok && old.Version != "" {
				ra.Version = old.Version
			}
		} else {
			redownloaded = true
			// 旧版本目录保留作回滚储备，不主动删除
		}
	}

	// 覆盖配置（保留 env/enabled）
	cfg.Command = def.Command
	cfg.Args = def.Args
	cfg.DisplayName = def.DisplayName
	cfg.Description = def.Description
	if err := h.store.Update(cfg); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	h.applyToRegistrar(cfg)

	slog.Info("agent 已从 registry 更新", "type", cfg.Type, "source", source, "redownloaded", redownloaded)
	Success(c, http.StatusOK, updateFromRegistryResponse{
		Version:      ra.Version,
		Command:      def.Command,
		Args:         parseArgsFromJSON(def.Args),
		Redownloaded: redownloaded,
		Source:       source,
	})
}

// parseArgsFromJSON 将存储的 JSON args 字符串解码为切片；空串或非法返回 nil。
func parseArgsFromJSON(argsJSON string) []string {
	var args []string
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	return args
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
