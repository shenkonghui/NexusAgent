package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/logging"
	"nexusagent/internal/models"
)

// minLevelOrder 用于按等级过滤日志。
// 数值越大表示等级越高；过滤时只保留 >= 请求等级的日志。
var levelOrder = map[string]int{
	models.LogLevelDebug: 10,
	models.LogLevelInfo:  20,
	models.LogLevelWarn:  30,
	models.LogLevelError: 40,
}

// LogHandler 提供 GET /api/v1/logs/stream，通过 SSE 实时推送后端日志。
// 它订阅 logging.LogHub，先发送历史快照（环形缓冲中的最近 N 条），
// 再持续转发实时日志，直到客户端断开。
type LogHandler struct {
	hub *logging.LogHub
}

// NewLogHandler 创建日志流 handler。hub 为 nil 时，Stream 端点将返回 503。
func NewLogHandler(hub *logging.LogHub) *LogHandler {
	return &LogHandler{hub: hub}
}

// Stream GET /api/v1/logs/stream
// 通过 SSE 推送后端日志。响应格式（参考 session stream）：
//
//	id: <seq>\ndata: <LogEntry JSON>\n\n
//
// 可选查询参数：
//   - level: 最低日志等级（debug/info/warn/error），默认 debug（即全部）
//   - since: 起始 seq（> since 的历史日志才会被推送，默认 0 表示推送全部历史）
func (h *LogHandler) Stream(c *gin.Context) {
	if h.hub == nil {
		Fail(c, http.StatusServiceUnavailable, "LOG_UNAVAILABLE", "日志中心未初始化")
		return
	}

	// 解析过滤参数
	minLevel := c.Query("level")
	minOrder, ok := levelOrder[minLevel]
	if !ok || minOrder == 0 {
		minLevel = models.LogLevelDebug
		minOrder = levelOrder[models.LogLevelDebug]
	}

	var since int64
	if s := c.Query("since"); s != "" {
		var n int64
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil && n >= 0 {
			since = n
		}
	}

	// 订阅日志中心，获取实时 channel 与历史快照
	ch, history := h.hub.Subscribe(0)
	defer h.hub.Unsubscribe(ch)

	// 注意：Subscribe 返回的 history 是订阅时刻的快照，之后产生的日志
	// 会通过 channel 推送，二者在 seq 上连续（hub 内部用同一把锁保证快照与
	// 订阅注册的原子性）。客户端可借助 id: <seq> 做幂等去重。

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEntry := func(e models.LogEntry, w io.Writer) {
		if order, ok := levelOrder[e.Level]; ok && order < minOrder {
			return
		}
		if e.Seq <= since {
			return
		}
		b, _ := json.Marshal(e)
		_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.Seq, b)
	}

	// 1. 先发送历史快照（按 seq 升序）
	for _, e := range history {
		writeEntry(e, c.Writer)
	}
	c.Writer.Flush()

	// 2. 持续转发实时日志，直到客户端断开（c.Request.Context().Done）
	ctx := c.Request.Context()
	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctx.Done():
			return false
		case entry, ok := <-ch:
			if !ok {
				return false
			}
			writeEntry(entry, w)
			return true
		}
	})
}
