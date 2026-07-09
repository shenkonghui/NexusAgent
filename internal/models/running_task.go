package models

import "time"

const (
	// RunningTaskStatusRunning 表示任务正在执行中（agent 正在产出消息）。
	RunningTaskStatusRunning = "running"
	// RunningTaskStatusInterrupted 表示任务因服务重启而中断，等待用户手动重发。
	RunningTaskStatusInterrupted = "interrupted"
	// RunningTaskStatusDone 表示任务已正常完成。
	RunningTaskStatusDone = "done"
)

// RunningTask 记录一次进行中的 prompt 任务，用于服务重启后的中断恢复。
// 每次 PromptWithExecution 启动时创建一行，prompt 流结束后置为 done。
// 若服务在 prompt 流期间重启，该行保持 running，启动恢复会将其标记为 interrupted。
type RunningTask struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	DBSessionID uint       `gorm:"index;not null" json:"db_session_id"`
	UserID      uint       `gorm:"index;not null" json:"user_id"`
	Prompt      string     `gorm:"type:text;not null" json:"prompt"`
	Status      string     `gorm:"size:32;not null;default:running" json:"status"`
	// LastSeq 记录最后一条已持久化消息的 sequence，便于恢复时定位进度。
	LastSeq     int        `gorm:"not null;default:0" json:"last_seq"`
	// ExecutionID 关联定时任务执行块（手动会话为 nil）。
	ExecutionID *uint      `gorm:"index" json:"execution_id"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `gorm:"index" json:"finished_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
