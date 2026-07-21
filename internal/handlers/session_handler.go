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

	acplocal "opennexus/internal/acp"
	"opennexus/internal/agent"
	"opennexus/internal/middleware"
	"opennexus/internal/models"
	"opennexus/internal/repository"
)

// SessionStore 暴露会话相关业务能力（*agent.Router 实现该接口）。
type SessionStore interface {
	CreateSession(ctx context.Context, agentType string, workspaceID uint, userID uint, modelValue string) (*models.Session, error)
	ListSessions(userID uint) ([]models.Session, error)
	ListSessionsBySource(userID uint, source string) ([]models.Session, error)
	GetSessionByDBID(id uint) (*models.Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	CancelSession(ctx context.Context, sessionID string) error
	ResumeSession(ctx context.Context, sessionID string) (*models.Session, error)
	ClearContext(ctx context.Context, sessionID string) (*models.Session, error)
	ListMessages(sessionID string) ([]models.Message, error)
	// FindMessageByID 按消息主键查询单条消息（用于撤销等按消息定位的场景）。
	FindMessageByID(messageID uint) (*models.Message, error)
	// DeleteMessagesFromSequence 删除指定会话中 sequence 大于等于 fromSeq 的消息（会话回滚，含目标）。
	DeleteMessagesFromSequence(dbSessionID uint, fromSeq int) (int64, error)
	ListCommands(sessionID string) ([]acp.AvailableCommand, error)
	ListConfiguredCommandsForSession(sessionID string) ([]acplocal.SlashCommand, error)
	ListConfigOptions(sessionID string) ([]acp.SessionConfigOption, error)
	ListModes(sessionID string) ([]acp.SessionMode, error)
	ListSkills(sessionID string) ([]acplocal.Skill, error)
	SetConfigOption(ctx context.Context, sessionID, configID, value string) error
	SetSessionMode(ctx context.Context, sessionID, modeID string) error
	RespondPermission(sessionID, requestID, optionID string, cancelled bool) error
	UpdateTitle(dbSessionID uint, title string) error
	Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error)
	GetWorkspaceCwd(workspaceID uint) (string, error)
	ListExecutions(sessionID string) ([]repository.ExecutionAggregate, error)
	// SubscribeSession 订阅会话当前进行中的 prompt 流（断点续传），返回遗漏消息与实时 channel。
	SubscribeSession(sessionID string, lastSeq int) ([]models.Message, <-chan models.Message, error)
	// HasActivePrompt 判断会话是否有进行中的 prompt。
	HasActivePrompt(sessionID string) bool
	// ListInterruptedTasks 返回指定会话下因服务重启而中断的任务。
	ListInterruptedTasks(dbSessionID uint) ([]models.RunningTask, error)
	// ResumeInterruptedTask 恢复中断的任务：ResumeSession + 重新发送原 prompt。
	ResumeInterruptedTask(ctx context.Context, taskID uint) (<-chan models.Message, error)
	// ListRunningDBSessionIDs 返回指定用户下所有正在运行的 db_session_id。
	ListRunningDBSessionIDs(userID uint) ([]uint, error)
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
	default:
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}

type createSessionRequest struct {
	AgentType   string `json:"agent_type" binding:"required"`
	WorkspaceID uint   `json:"workspace_id"`
	ModelValue  string `json:"model_value"`
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
	sess, err := h.store.CreateSession(c.Request.Context(), req.AgentType, req.WorkspaceID, uid, req.ModelValue)
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

// RunningSessions GET /api/v1/sessions/running — 返回当前用户正在运行的会话 db_session_id 列表。
// 侧边栏据此展示哪些会话正在执行（旋转图标）。
func (h *SessionHandler) RunningSessions(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	ids, err := h.store.ListRunningDBSessionIDs(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusOK, gin.H{"db_session_ids": ids})
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

// Delete DELETE /api/v1/sessions/:id — 彻底删除会话及其消息。
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

// Executions GET /api/v1/sessions/:id/executions — 会话内按 execution_id 聚合的执行块（定时任务/笔记分类）。
func (h *SessionHandler) Executions(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	execs, err := h.store.ListExecutions(sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	if execs == nil {
		execs = []repository.ExecutionAggregate{}
	}
	Success(c, http.StatusOK, gin.H{"executions": execs})
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

// Resume POST /api/v1/sessions/:id/resume
// 恢复 error 状态会话或重开 closed 状态会话。
func (h *SessionHandler) Resume(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	updated, err := h.store.ResumeSession(c.Request.Context(), sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, updated)
}

// ClearContext POST /api/v1/sessions/:id/clear-context
// 清理会话上下文：重置底层 ACP 会话（token 占用归零），保留数据库会话与历史消息。
func (h *SessionHandler) ClearContext(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	updated, err := h.store.ClearContext(c.Request.Context(), sess.SessionID)
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
	configured, _ := h.store.ListConfiguredCommandsForSession(sess.SessionID)
	items := buildCommandItems(cmds, configured)
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
	Path        string `json:"path"`
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
			Path:        s.Path,
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
		// 会话记录中保存了用户发送时选择的模型（ModelValue），
		// 用它覆盖 model 选项的 CurrentValue，避免回显 agent 默认模型。
		if item.Category == "model" && sess.ModelValue != "" && optionValuePresent(item.Options, sess.ModelValue) {
			item.CurrentValue = sess.ModelValue
		}
		items = append(items, item)
	}
	Success(c, http.StatusOK, gin.H{"config_options": items})
}

// optionValuePresent 判断给定值是否存在于可选项列表中。
func optionValuePresent(options []configOptionValue, value string) bool {
	for _, o := range options {
		if o.Value == value {
			return true
		}
	}
	return false
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

type setModeRequest struct {
	ModeID string `json:"mode_id" binding:"required"`
}

// SetMode POST /api/v1/sessions/:id/mode — 切换会话模式（ask / agent / edit 等）。
func (h *SessionHandler) SetMode(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	var req setModeRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ModeID) == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "mode_id 不能为空")
		return
	}
	if sess.Status != models.SessionStatusActive {
		Fail(c, http.StatusConflict, "SESSION_NOT_ACTIVE", "会话不在活跃状态")
		return
	}
	if err := h.store.SetSessionMode(c.Request.Context(), sess.SessionID, req.ModeID); err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

type respondPermissionRequest struct {
	OptionID  string `json:"option_id"`
	Cancelled bool   `json:"cancelled"`
}

// RespondPermission POST /api/v1/sessions/:id/permissions/:requestId/respond
func (h *SessionHandler) RespondPermission(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	requestID := strings.TrimSpace(c.Param("requestId"))
	if requestID == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "缺少 requestId")
		return
	}
	var req respondPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	if !req.Cancelled && strings.TrimSpace(req.OptionID) == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "option_id 不能为空")
		return
	}
	if err := h.store.RespondPermission(sess.SessionID, requestID, req.OptionID, req.Cancelled); err != nil {
		if errors.Is(err, acplocal.ErrPermissionNotFound) {
			Fail(c, http.StatusNotFound, "PERMISSION_NOT_FOUND", "权限请求不存在或已过期")
			return
		}
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
	if sess.Status != models.SessionStatusActive && sess.Status != models.SessionStatusPending {
		// 自动恢复 error 或 closed 状态的会话
		if _, err := h.store.ResumeSession(c.Request.Context(), sess.SessionID); err != nil {
			writeSessionError(c, err)
			return
		}
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
		// 每条消息附带 id: <sequence>，供客户端断点续传（Last-Event-ID）
		_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", msg.Sequence, b)
		return true
	})
}

// Stream GET /api/v1/sessions/:id/stream
// 订阅会话当前进行中的 prompt 流（断点续传），不发起新 prompt。
// 客户端通过 Last-Event-ID 头携带最后收到的 sequence，服务端先从 DB 补齐遗漏消息，再推送实时流。
func (h *SessionHandler) Stream(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}

	// 解析 Last-Event-ID 头（客户端最后收到的 sequence）
	lastSeq := 0
	if lei := c.GetHeader("Last-Event-ID"); lei != "" {
		if n, err := strconv.Atoi(lei); err == nil && n >= 0 {
			lastSeq = n
		}
	}

	missed, ch, err := h.store.SubscribeSession(sess.SessionID, lastSeq)
	if err != nil {
		writeSessionError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// 先发送 DB 补齐的遗漏消息（每条带 id: sequence）
	for _, msg := range missed {
		b, _ := json.Marshal(msg)
		_, _ = fmt.Fprintf(c.Writer, "id: %d\ndata: %s\n\n", msg.Sequence, b)
	}
	c.Writer.Flush()

	// 若无实时 channel（无活跃 prompt），直接结束
	if ch == nil {
		_, _ = fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		return
	}

	c.Stream(func(w io.Writer) bool {
		msg, ok := <-ch
		if !ok {
			_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
			return false
		}
		b, _ := json.Marshal(msg)
		_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", msg.Sequence, b)
		return true
	})
}

// InterruptedTasks GET /api/v1/sessions/:id/interrupted-tasks
// 返回指定会话下因服务重启而中断的任务列表。
func (h *SessionHandler) InterruptedTasks(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	tasks, err := h.store.ListInterruptedTasks(sess.ID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "LIST_INTERRUPTED_FAILED", "查询中断任务失败")
		return
	}
	Success(c, http.StatusOK, gin.H{"tasks": tasks})
}

// ResumeInterruptedTask POST /api/v1/running-tasks/:taskId/resume
// 恢复中断的任务：自动 ResumeSession 并重新发送原 prompt。
func (h *SessionHandler) ResumeInterruptedTask(c *gin.Context) {
	taskIDStr := c.Param("taskId")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 64)
	if err != nil || taskID == 0 {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "无效的任务 ID")
		return
	}

	ch, err := h.store.ResumeInterruptedTask(c.Request.Context(), uint(taskID))
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
		_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", msg.Sequence, b)
		return true
	})
}
