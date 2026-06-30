package models

import "time"

const (
	SessionStatusActive = "active"
	SessionStatusClosed = "closed"
	SessionStatusError  = "error"

	// 会话来源：手动创建或定时任务创建
	SessionSourceManual    = "manual"
	SessionSourceScheduled = "scheduled"
)

type Session struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	SessionID     string `gorm:"uniqueIndex;size:128;not null" json:"session_id"`
	AgentType     string `gorm:"size:64;not null" json:"agent_type"`
	Cwd           string `gorm:"size:512;not null" json:"-"` // 废弃，cwd 从 Workspace 获取
	Status        string `gorm:"size:32;not null;default:active" json:"status"`
	UserID        uint   `gorm:"index" json:"user_id"`
	WorkspaceMode string `gorm:"size:32;not null" json:"-"` // 废弃
	TempDir       string `gorm:"size:512" json:"-"`          // 废弃
	// WorkspaceID 关联的工作区 ID（可选，向后兼容旧数据）
	WorkspaceID *uint     `gorm:"index" json:"workspace_id"`
	Workspace   Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	LastPrompt  string    `gorm:"type:text" json:"last_prompt"`
	// Title 是会话显示标题，首次对话后从 prompt 提取前若干字符生成。
	Title string `gorm:"size:128" json:"title"`
	// Source 标识会话来源：manual（手动创建）或 scheduled（定时任务创建）。
	Source    string     `gorm:"size:32;not null;default:manual" json:"source"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `gorm:"index" json:"closed_at"`
}
