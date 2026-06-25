package models

import "time"

const (
	SessionStatusActive = "active"
	SessionStatusClosed = "closed"
	SessionStatusError  = "error"

	WorkspaceModeExternal  = "external"
	WorkspaceModeTemporary = "temporary"

	// 会话来源：手动创建或定时任务创建
	SessionSourceManual    = "manual"
	SessionSourceScheduled = "scheduled"
)

type Session struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	SessionID     string `gorm:"uniqueIndex;size:128;not null" json:"session_id"`
	AgentType     string `gorm:"size:64;not null" json:"agent_type"`
	Cwd           string `gorm:"size:512;not null" json:"cwd"`
	Status        string `gorm:"size:32;not null;default:active" json:"status"`
	UserID        uint   `gorm:"index" json:"user_id"`
	WorkspaceMode string `gorm:"size:32;not null" json:"workspace_mode"`
	TempDir       string `gorm:"size:512" json:"temp_dir"`
	LastPrompt    string `gorm:"type:text" json:"last_prompt"`
	// Title 是会话显示标题，首次对话后从 prompt 提取前若干字符生成。
	Title string `gorm:"size:128" json:"title"`
	// Source 标识会话来源：manual（手动创建）或 scheduled（定时任务创建）。
	Source    string     `gorm:"size:32;not null;default:manual" json:"source"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `gorm:"index" json:"closed_at"`
}
