package logging

import (
	"context"
	"log/slog"
	"runtime"

	"nexusagent/internal/models"
)

// fanoutHandler 是一个把日志同时输出到多个目的地的 slog.Handler。
// 它把 slog.Handler 接口的调用委托给一个底层 handler（通常为 stderr text handler），
// 同时把每条日志结构化后转发给 LogHub 供前端实时查看。
//
// 这样既能保持原有「日志输出到控制台」的行为不变，
// 又能让前端通过 SSE 端点订阅到与控制台一致的日志流。
type fanoutHandler struct {
	// downstream 是原有的 stderr 输出 handler，保持控制台行为不变。
	downstream slog.Handler
	hub        *LogHub
	// attrs 与 group 由 WithAttrs/WithGroup 累积，用于传递给 downstream。
	// 转发给 hub 的 entry 只保留 message + level + source，不携带 attrs（简化前端展示）。
	attrs []slog.Attr
}

// newFanoutHandler 包装一个下游 handler，并把每条日志转发到 hub。
func newFanoutHandler(downstream slog.Handler, hub *LogHub) *fanoutHandler {
	return &fanoutHandler{
		downstream: downstream,
		hub:        hub,
	}
}

// Enabled 直接采用下游 handler 的等级配置。
func (h *fanoutHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.downstream.Enabled(ctx, lvl)
}

// Handle 先把记录交给下游 handler 输出到 stderr，再把结构化条目转发到 hub。
func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	// 1. 保持原有控制台输出
	if err := h.downstream.Handle(ctx, r.Clone()); err != nil {
		return err
	}

	// 2. 转发到 hub 供前端查看（hub 可能为 nil，例如 setup 前的早期日志）
	if h.hub == nil {
		return nil
	}

	entry := models.LogEntry{
		Timestamp: r.Time,
		Level:     levelToString(r.Level),
		Message:   r.Message,
		Source:    sourceFromRecord(&r),
	}
	h.hub.Append(entry)
	return nil
}

// WithAttrs 返回一个附带额外属性的新 handler。
// 这里把属性透传给下游 handler；上游 hub 始终是共享单例，不复制。
func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newDownstream := h.downstream.WithAttrs(attrs)
	return &fanoutHandler{
		downstream: newDownstream,
		hub:        h.hub,
		attrs:      append(h.attrs, attrs...),
	}
}

// WithGroup 返回一个附带嵌套 group 的新 handler，透传给下游。
func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	newDownstream := h.downstream.WithGroup(name)
	return &fanoutHandler{
		downstream: newDownstream,
		hub:        h.hub,
		attrs:      h.attrs,
	}
}

// levelToString 把 slog.Level 映射为前端可读的等级字符串。
func levelToString(lvl slog.Level) string {
	switch {
	case lvl >= slog.LevelError:
		return models.LogLevelError
	case lvl >= slog.LevelWarn:
		return models.LogLevelWarn
	case lvl >= slog.LevelInfo:
		return models.LogLevelInfo
	default:
		return models.LogLevelDebug
	}
}

// sourceFromRecord 从 slog.Record 的 PC 字段解析出调用方的包名.函数名。
// record.PC 为 0（例如手动构造的 record）时返回空字符串。
func sourceFromRecord(r *slog.Record) string {
	if r.PC == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, more := frames.Next()
	_ = more
	if frame.Function == "" {
		return ""
	}
	return frame.Function
}
