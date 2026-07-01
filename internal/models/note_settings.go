package models

import "time"

// NoteSettings 是用户笔记自动分类配置。
type NoteSettings struct {
	ID                      uint      `gorm:"primaryKey" json:"id"`
	UserID                  uint      `gorm:"uniqueIndex;not null" json:"user_id"`
	AgentType               string    `gorm:"size:64" json:"agent_type"`
	ModelValue              string    `gorm:"size:128" json:"model_value"`
	ClassifyPrompt          string    `gorm:"type:text" json:"classify_prompt"`
	ClassifyIntervalMinutes int       `gorm:"not null;default:5" json:"classify_interval_minutes"`
	// ClassifySessionID / ClassifyDBSessionID 关联笔记分类任务会话（默认工作区）。
	ClassifySessionID   string `gorm:"size:128" json:"classify_session_id"`
	ClassifyDBSessionID uint   `gorm:"index" json:"classify_db_session_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}
