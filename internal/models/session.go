package models

import "time"

const (
	SessionStatusActive = "active"
	SessionStatusClosed = "closed"
	SessionStatusError  = "error"

	WorkspaceModeExternal  = "external"
	WorkspaceModeTemporary = "temporary"
)

type Session struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	SessionID     string     `gorm:"uniqueIndex;size:128;not null" json:"session_id"`
	AgentType     string     `gorm:"size:64;not null" json:"agent_type"`
	Cwd           string     `gorm:"size:512;not null" json:"cwd"`
	Status        string     `gorm:"size:32;not null;default:active" json:"status"`
	UserID        uint       `gorm:"index" json:"user_id"`
	WorkspaceMode string     `gorm:"size:32;not null" json:"workspace_mode"`
	TempDir       string     `gorm:"size:512" json:"temp_dir"`
	LastPrompt    string     `gorm:"type:text" json:"last_prompt"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ClosedAt      *time.Time `gorm:"index" json:"closed_at"`
}
