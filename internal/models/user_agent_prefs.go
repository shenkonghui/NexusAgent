package models

import "time"

// UserAgentPrefs 是用户最近使用的 agent 与各 agent 配置偏好。
type UserAgentPrefs struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"uniqueIndex;not null" json:"user_id"`
	LastAgentType  string    `gorm:"size:64" json:"last_agent_type"`
	PrefsJSON      string    `gorm:"type:text" json:"prefs_json"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
