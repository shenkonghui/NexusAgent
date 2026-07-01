package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Setup 根据配置等级初始化全局 slog，供 ACP SDK 与应用代码共用。
func Setup(level string) {
	lvl := ParseLevel(level)
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
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
