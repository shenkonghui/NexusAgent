package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/coder/acp-go-sdk"
	"github.com/gin-gonic/gin"

	acplocal "nexusagent/internal/acp"
	"nexusagent/internal/agent"
	"nexusagent/internal/middleware"
	"nexusagent/internal/models"
)

// commandItem 是对外暴露的 slash command 描述。
type commandItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HasInput    bool   `json:"has_input"`
}

// SessionStore 暴露会话相关业务能力（*agent.Router 实现该接口）。
type SessionStore interface {
	CreateSession(ctx context.Context, agentType, cwd string, userID uint) (*models.Session, error)
	ListSessions(userID uint) ([]models.Session, error)
	ListSessionsBySource(userID uint, source string) ([]models.Session, error)
	GetSessionByDBID(id uint) (*models.Session, error)
	CloseSession(ctx context.Context, sessionID string) error
	DeleteSession(ctx context.Context, sessionID string) error
	CancelSession(ctx context.Context, sessionID string) error
	ResumeSession(ctx context.Context, sessionID, cwdOverride string) (*models.Session, error)
	ListMessages(sessionID string) ([]models.Message, error)
	ListCommands(sessionID string) ([]acp.AvailableCommand, error)
	ListConfigOptions(sessionID string) ([]acp.SessionConfigOption, error)
	ListModes(sessionID string) ([]acp.SessionMode, error)
	ListSkills(sessionID string) ([]acplocal.Skill, error)
	SetConfigOption(ctx context.Context, sessionID, configID, value string) error
	UpdateTitle(dbSessionID uint, title string) error
	Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error)
}

// SessionHandler 处理会话相关请求。
type SessionHandler struct {
	store SessionStore
}

// NewSessionHandler 创建 SessionHandler。
func NewSessionHandler(store SessionStore) *SessionHandler {
	return &SessionHandler{store: store}
}

// currentUserID 从 context 读取中间件注入的 userID。
func currentUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get(middleware.UserIDKey())
	if !exists {
		return 0, false
	}
	uid, ok := v.(uint)
	return uid, ok
}

// parseSessionID 解析 :id（uint，>0）。
func parseSessionID(c *gin.Context) (uint, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "无效的会话 ID")
		return 0, false
	}
	return uint(id), true
}

// loadOwnedSession 加载 :id 对应会话并校验归属；失败时已写入错误响应。
func (h *SessionHandler) loadOwnedSession(c *gin.Context) (*models.Session, bool) {
	id, ok := parseSessionID(c)
	if !ok {
		return nil, false
	}
	sess, err := h.store.GetSessionByDBID(id)
	if err != nil || sess == nil {
		Fail(c, http.StatusNotFound, "SESSION_NOT_FOUND", "会话不存在")
		return nil, false
	}
	uid, ok := currentUserID(c)
	if !ok || sess.UserID != uid {
		Fail(c, http.StatusNotFound, "SESSION_NOT_FOUND", "会话不存在")
		return nil, false
	}
	return sess, true
}

// writeSessionError 将 service 层错误映射为统一 HTTP 响应。
func writeSessionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent.ErrAgentNotFound):
		Fail(c, http.StatusBadRequest, "AGENT_NOT_FOUND", "未知的 agent 类型")
	case strings.Contains(err.Error(), "必须提供 cwd"):
		Fail(c, http.StatusBadRequest, "CWD_REQUIRED", "external 模式必须提供工作目录")
	case strings.Contains(err.Error(), "工作目录不存在"):
		Fail(c, http.StatusBadRequest, "CWD_NOT_FOUND", err.Error())
	case strings.Contains(err.Error(), "恢复会话需要工作目录"):
		Fail(c, http.StatusBadRequest, "CWD_REQUIRED", err.Error())
	default:
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}

type createSessionRequest struct {
	AgentType string `json:"agent_type" binding:"required"`
	Cwd       string `json:"cwd"`
}

// Create POST /api/v1/sessions
func (h *SessionHandler) Create(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	sess, err := h.store.CreateSession(c.Request.Context(), req.AgentType, req.Cwd, uid)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusCreated, sess)
}

// List GET /api/v1/sessions?source=manual|scheduled
func (h *SessionHandler) List(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	source := strings.TrimSpace(c.Query("source"))
	if source == "" {
		sessions, err := h.store.ListSessions(uid)
		if err != nil {
			writeSessionError(c, err)
			return
		}
		Success(c, http.StatusOK, gin.H{"sessions": sessions})
		return
	}
	sessions, err := h.store.ListSessionsBySource(uid, source)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, gin.H{"sessions": sessions})
}

// Get GET /api/v1/sessions/:id
func (h *SessionHandler) Get(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	Success(c, http.StatusOK, sess)
}

// UpdateTitle PUT /api/v1/sessions/:id/title
func (h *SessionHandler) UpdateTitle(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		Fail(c, http.StatusBadRequest, "INVALID_TITLE", "标题不能为空")
		return
	}
	if err := h.store.UpdateTitle(sess.ID, title); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	sess.Title = title
	Success(c, http.StatusOK, sess)
}

// Close DELETE /api/v1/sessions/:id
func (h *SessionHandler) Close(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	if err := h.store.CloseSession(c.Request.Context(), sess.SessionID); err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

// Delete POST /api/v1/sessions/:id/delete — 彻底删除会话及其消息。
func (h *SessionHandler) Delete(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	if err := h.store.DeleteSession(c.Request.Context(), sess.SessionID); err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

// Messages GET /api/v1/sessions/:id/messages
func (h *SessionHandler) Messages(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	msgs, err := h.store.ListMessages(sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, gin.H{"messages": msgs})
}

// Cancel POST /api/v1/sessions/:id/cancel
func (h *SessionHandler) Cancel(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	if sess.Status != models.SessionStatusActive {
		Fail(c, http.StatusConflict, "SESSION_NOT_ACTIVE", "会话不在活跃状态")
		return
	}
	if err := h.store.CancelSession(c.Request.Context(), sess.SessionID); err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

type resumeRequest struct {
	Cwd string `json:"cwd"`
}

// Resume POST /api/v1/sessions/:id/resume
// 恢复 error 状态会话或重开 closed 状态会话。可选 cwd 用于指定新工作目录。
func (h *SessionHandler) Resume(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	var req resumeRequest
	// body 可选，解析失败时使用空 cwd
	_ = c.ShouldBindJSON(&req)
	updated, err := h.store.ResumeSession(c.Request.Context(), sess.SessionID, strings.TrimSpace(req.Cwd))
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, updated)
}

// Commands GET /api/v1/sessions/:id/commands — 返回会话缓存的可用 slash command。
func (h *SessionHandler) Commands(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	cmds, err := h.store.ListCommands(sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	items := make([]commandItem, 0, len(cmds))
	for _, cmd := range cmds {
		items = append(items, commandItem{
			Name:        cmd.Name,
			Description: cmd.Description,
			HasInput:    cmd.Input != nil,
		})
	}
	Success(c, http.StatusOK, gin.H{"commands": items})
}

// modeItem 是对外暴露的 session mode 描述。
type modeItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Modes GET /api/v1/sessions/:id/modes — 返回会话可用的 mode 列表（agent skill/模式）。
func (h *SessionHandler) Modes(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	modes, err := h.store.ListModes(sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
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

// skillItem 是对外暴露的 Agent Skill 描述。
type skillItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Scope       string `json:"scope"`
}

// Skills GET /api/v1/sessions/:id/skills — 返回会话工作目录下发现的 Agent Skills。
func (h *SessionHandler) Skills(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	skills, err := h.store.ListSkills(sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	items := make([]skillItem, 0, len(skills))
	for _, s := range skills {
		items = append(items, skillItem{
			Name:        s.Name,
			Description: s.Description,
			Location:    s.Location,
			Scope:       s.Scope,
		})
	}
	Success(c, http.StatusOK, gin.H{"skills": items})
}

// configOptionItem 是对外暴露的 config option 描述。
type configOptionItem struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Category     string              `json:"category"`
	Type         string              `json:"type"`
	CurrentValue string              `json:"current_value"`
	Options      []configOptionValue `json:"options"`
}

// configOptionValue 是可选项的值描述。
type configOptionValue struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ConfigOptions GET /api/v1/sessions/:id/config-options — 返回会话的 config option（含模型选择）。
func (h *SessionHandler) ConfigOptions(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	opts, err := h.store.ListConfigOptions(sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	items := make([]configOptionItem, 0, len(opts))
	for _, opt := range opts {
		item := configOptionItem{
			Type: "boolean",
		}
		if opt.Select != nil {
			item.ID = string(opt.Select.Id)
			item.Name = opt.Select.Name
			item.Type = "select"
			item.CurrentValue = string(opt.Select.CurrentValue)
			if opt.Select.Category != nil {
				item.Category = string(*opt.Select.Category)
			}
			if opt.Select.Description != nil {
				item.Name = opt.Select.Name
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

type setConfigOptionRequest struct {
	ConfigID string `json:"config_id" binding:"required"`
	Value    string `json:"value" binding:"required"`
}

// SetConfigOption POST /api/v1/sessions/:id/config-options — 设置会话的 config option 值。
func (h *SessionHandler) SetConfigOption(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	var req setConfigOptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	if err := h.store.SetConfigOption(c.Request.Context(), sess.SessionID, req.ConfigID, req.Value); err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

type promptRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

// Prompt POST /api/v1/sessions/:id/prompt （SSE 流）
func (h *SessionHandler) Prompt(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	var req promptRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "prompt 不能为空")
		return
	}
	if sess.Status != models.SessionStatusActive {
		Fail(c, http.StatusConflict, "SESSION_NOT_ACTIVE", "会话不在活跃状态")
		return
	}
	ch, err := h.store.Prompt(c.Request.Context(), sess.SessionID, req.Prompt)
	if err != nil {
		writeSessionError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	c.Stream(func(w io.Writer) bool {
		msg, ok := <-ch
		if !ok {
			_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
			return false
		}
		b, _ := json.Marshal(msg)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		return true
	})
}
