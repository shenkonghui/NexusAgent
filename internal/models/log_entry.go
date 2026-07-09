package models

import "time"

// 日志等级常量（字符串形式，便于序列化与过滤）
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// LogEntry 是一条后端日志条目，用于通过 SSE 推送给前端日志查看器。
// 与持久化的领域模型不同，日志仅保存在内存环形缓冲中，进程重启即丢失。
type LogEntry struct {
	// Seq 是单调递增的序号，用于前端去重与断点续传（Last-Event-ID 对应此字段）。
	Seq int64 `json:"seq"`
	// Timestamp 日志产生时间（RFC3339）。
	Timestamp time.Time `json:"timestamp"`
	// Level 日志等级：debug / info / warn / error。
	Level string `json:"level"`
	// Source 日志来源，通常为产生日志的包名/函数名（由 slog record 的 PC 解析得到）。
	Source string `json:"source"`
	// Message 日志正文。
	Message string `json:"message"`
}
