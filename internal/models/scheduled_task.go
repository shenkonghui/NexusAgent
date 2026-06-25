package models

import "time"

// 定时任务最近执行状态
const (
	TaskStatusSuccess = "success"
	TaskStatusRunning = "running"
	TaskStatusFailed  = "failed"
	TaskStatusSkipped = "skipped"
)

// ScheduledTask 是定时任务配置。每个任务关联一个 Session（首次执行时创建），
// 每次 cron 触发在该 session 内追加一轮对话（用 execution_id 标记执行块）。
type ScheduledTask struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	AgentType string `gorm:"size:64;not null" json:"agent_type"`
	Cwd       string `gorm:"size:512;not null" json:"cwd"`
	Prompt    string `gorm:"type:text;not null" json:"prompt"`
	CronExpr  string `gorm:"size:128;not null" json:"cron_expr"`
	Enabled   bool   `gorm:"not null;default:true" json:"enabled"`
	UserID    uint   `gorm:"index;not null" json:"user_id"`
	// ModelValue 是可选的模型配置值，执行时设置到会话的 model config option。
	// 为空则使用 agent 默认模型。
	ModelValue  string     `gorm:"size:128" json:"model_value"`
	SessionID   string     `gorm:"size:128" json:"session_id"`
	DBSessionID uint       `gorm:"index" json:"db_session_id"`
	LastRunAt   *time.Time `json:"last_run_at"`
	LastStatus  string     `gorm:"size:32" json:"last_status"`
	LastError   string     `gorm:"type:text" json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
