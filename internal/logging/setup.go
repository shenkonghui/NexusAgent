package logging

import (
	"log/slog"
	"os"
	"strings"
)

// defaultHub 是包级日志中心单例，由 Setup 初始化。
// 供 LogHandler 订阅、转发给前端日志查看器。
var defaultHub *LogHub

// Setup 根据配置等级初始化全局 slog，供 ACP SDK 与应用代码共用。
// 它在原有 stderr text handler 之上包一层 fanoutHandler，使每条日志
// 同时输出到控制台并被 LogHub 收集，供前端通过 SSE 实时查看。
func Setup(level string) {
	lvl := ParseLevel(level)
	opts := &slog.HandlerOptions{Level: lvl, AddSource: true}
	textH := slog.NewTextHandler(os.Stderr, opts)

	// 创建日志中心（保持 stderr 输出不变 + 转发到 hub）
	defaultHub = NewLogHub(defaultRingCapacity)
	fanout := newFanoutHandler(textH, defaultHub)
	slog.SetDefault(slog.New(fanout))
}

// DefaultHub 返回由 Setup 初始化的日志中心单例。
// 在 Setup 之前调用返回 nil，调用方需自行判空。
func DefaultHub() *LogHub {
	return defaultHub
}

// ParseLevel 解析日志等级字符串，未知值默认为 info。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Preview 截断长文本用于 debug 日志。
func Preview(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
