package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
	"nexusagent/internal/middleware"
	"nexusagent/internal/models"
)

// SessionStore 暴露会话相关业务能力（*agent.Router 实现该接口）。
type SessionStore interface {
	CreateSession(ctx context.Context, agentType, cwd string, userID uint) (*models.Session, error)
	ListSessions(userID uint) ([]models.Session, error)
	GetSessionByDBID(id uint) (*models.Session, error)
	CloseSession(ctx context.Context, sessionID string) error
	CancelSession(ctx context.Context, sessionID string) error
	ResumeSession(ctx context.Context, sessionID string) (*models.Session, error)
	ListMessages(sessionID string) ([]models.Message, error)
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
	default:
		Fail(c, http.StatusInternalServerError, "INTERNAL", "内部错误")
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

// List GET /api/v1/sessions
func (h *SessionHandler) List(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	sessions, err := h.store.ListSessions(uid)
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

// Resume POST /api/v1/sessions/:id/resume
func (h *SessionHandler) Resume(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	if sess.Status == models.SessionStatusClosed {
		Fail(c, http.StatusConflict, "SESSION_CLOSED", "会话已关闭，无法恢复")
		return
	}
	updated, err := h.store.ResumeSession(c.Request.Context(), sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, updated)
}

type promptRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

// Prompt POST /api/v1/sessions/:id/prompt （SSE 流）
func (h *SessionHandler) Prompt(c *gin.Context) {
	id, ok := parseSessionID(c)
	if !ok {
		return
	}
	var req promptRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Prompt) == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "prompt 不能为空")
		return
	}
	sess, err := h.store.GetSessionByDBID(id)
	if err != nil || sess == nil {
		Fail(c, http.StatusNotFound, "SESSION_NOT_FOUND", "会话不存在")
		return
	}
	uid, ok := currentUserID(c)
	if !ok || sess.UserID != uid {
		Fail(c, http.StatusNotFound, "SESSION_NOT_FOUND", "会话不存在")
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

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		Fail(c, http.StatusInternalServerError, "INTERNAL", "不支持流式响应")
		return
	}

	for msg := range ch {
		b, _ := json.Marshal(msg)
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		flusher.Flush()
	}
	_, _ = fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()
}
