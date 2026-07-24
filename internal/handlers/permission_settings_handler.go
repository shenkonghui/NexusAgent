package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"opennexus/internal/models"
	"opennexus/internal/repository"
)

// PermissionRuleReloader 在保存权限规则后触发热更新（由 *agent.Router 实现）。
type PermissionRuleReloader interface {
	ReloadPermissionRules(userID uint)
}

// PermissionSettingsHandler 处理全局权限规则配置（yolo / 白名单 / 黑名单）。
type PermissionSettingsHandler struct {
	settingsRepo *repository.PermissionSettingsRepository
	rel          PermissionRuleReloader
}

func NewPermissionSettingsHandler(settingsRepo *repository.PermissionSettingsRepository, rel PermissionRuleReloader) *PermissionSettingsHandler {
	return &PermissionSettingsHandler{settingsRepo: settingsRepo, rel: rel}
}

type permissionSettingsItem struct {
	Mode  string   `json:"mode"`  // normal | yolo
	Allow []string `json:"allow"` // 白名单
	Ask   []string `json:"ask"`   // 询问名单
	Deny  []string `json:"deny"`  // 黑名单
}

type permissionSettingsRequest struct {
	Mode  string   `json:"mode"`
	Allow []string `json:"allow"`
	Ask   []string `json:"ask"`
	Deny  []string `json:"deny"`
}

// toItem 把存储模型转换为返回给前端的结构（列表 JSON 解码为数组，mode 兜底）。
func (h *PermissionSettingsHandler) toItem(s *models.PermissionSettings) permissionSettingsItem {
	mode := strings.TrimSpace(s.Mode)
	if mode == "" {
		mode = models.PermissionModeNormal
	}
	return permissionSettingsItem{
		Mode:  mode,
		Allow: decodeStringList(s.Allow),
		Ask:   decodeStringList(s.Ask),
		Deny:  decodeStringList(s.Deny),
	}
}

// GetSettings GET /api/v1/permissions/settings
func (h *PermissionSettingsHandler) GetSettings(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	s, err := h.settingsRepo.FindByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询权限设置失败")
		return
	}
	Success(c, http.StatusOK, h.toItem(s))
}

// UpdateSettings PUT /api/v1/permissions/settings
func (h *PermissionSettingsHandler) UpdateSettings(c *gin.Context) {
	var req permissionSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	mode := strings.TrimSpace(req.Mode)
	if mode != models.PermissionModeNormal && mode != models.PermissionModeYolo {
		mode = models.PermissionModeNormal
	}
	s := &models.PermissionSettings{
		UserID: uid,
		Mode:   mode,
		Allow:  encodeStringList(cleanRuleList(req.Allow)),
		Ask:    encodeStringList(cleanRuleList(req.Ask)),
		Deny:   encodeStringList(cleanRuleList(req.Deny)),
	}
	if err := h.settingsRepo.Upsert(s); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "保存权限设置失败")
		return
	}
	// 热更新：下发新规则到所有连接的 broker
	if h.rel != nil {
		h.rel.ReloadPermissionRules(uid)
	}
	saved, err := h.settingsRepo.FindByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "查询权限设置失败")
		return
	}
	Success(c, http.StatusOK, h.toItem(saved))
}

// decodeStringList 解码 JSON 编码的字符串列表；空/非法返回空数组。
func decodeStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return []string{}
	}
	return arr
}

// encodeStringList 编码字符串列表为 JSON（空列表编为 "[]"）。
func encodeStringList(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// cleanRuleList 去空白、去重、去空行。
func cleanRuleList(list []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(list))
	for _, r := range list {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}
