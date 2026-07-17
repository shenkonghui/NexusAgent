package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/acp"
	"nexusagent/internal/models"
)

// DebugHandler 提供会话 ACP 调试数据读取 API。
type DebugHandler struct {
	store SessionStore
	dbg   *acp.ACPDebugger
}

// NewDebugHandler 创建 DebugHandler；dbg 可为 nil（全部返回 enabled=false）。
func NewDebugHandler(store SessionStore, dbg *acp.ACPDebugger) *DebugHandler {
	return &DebugHandler{store: store, dbg: dbg}
}

// Meta GET /sessions/:id/debug — 返回调试元信息。
func (h *DebugHandler) Meta(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	dbID := strconv.FormatUint(uint64(sess.ID), 10)
	if h.dbg == nil {
		Success(c, http.StatusOK, acp.DebugMeta{Enabled: false})
		return
	}
	Success(c, http.StatusOK, h.dbg.Meta(dbID))
}

// Events GET /sessions/:id/debug/events?since=&limit=
func (h *DebugHandler) Events(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	if h.dbg == nil || !h.dbg.Enabled() {
		Success(c, http.StatusOK, gin.H{"events": []acp.EventRecord{}})
		return
	}
	since, limit := parseSinceLimit(c)
	dbID := strconv.FormatUint(uint64(sess.ID), 10)
	events, err := h.dbg.ReadEvents(dbID, since, limit)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "DEBUG_READ_FAILED", err.Error())
		return
	}
	if events == nil {
		events = []acp.EventRecord{}
	}
	Success(c, http.StatusOK, gin.H{"events": events})
}

// Raw GET /sessions/:id/debug/raw?since=&limit=
func (h *DebugHandler) Raw(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	if h.dbg == nil || !h.dbg.Enabled() {
		Success(c, http.StatusOK, gin.H{"raw": []acp.RawRecord{}})
		return
	}
	since, limit := parseSinceLimit(c)
	dbID := strconv.FormatUint(uint64(sess.ID), 10)
	raw, err := h.dbg.ReadRaw(dbID, since, limit)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "DEBUG_READ_FAILED", err.Error())
		return
	}
	if raw == nil {
		raw = []acp.RawRecord{}
	}
	Success(c, http.StatusOK, gin.H{"raw": raw})
}

func (h *DebugHandler) loadOwnedSession(c *gin.Context) (*models.Session, bool) {
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

func parseSinceLimit(c *gin.Context) (since, limit int) {
	since, _ = strconv.Atoi(c.DefaultQuery("since", "0"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "200"))
	if since < 0 {
		since = 0
	}
	// limit=0 表示不限制；负数回退默认；过大时做安全上限
	if limit < 0 {
		limit = 200
	}
	if limit > 100000 {
		limit = 100000
	}
	return since, limit
}
