package models

import "time"

// AgentConfig 是通过设置页面添加的本地 ACP agent 配置（全局共享）。
// 与 config.yaml 内置 agent 互补，运行时动态注册到 registry 与 acp service。
type AgentConfig struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Type        string    `gorm:"uniqueIndex;size:64;not null" json:"type"`
	DisplayName string    `gorm:"size:128;not null" json:"display_name"`
	Description string    `gorm:"size:256" json:"description"`
	Command     string    `gorm:"size:256;not null" json:"command"`
	Args        string    `gorm:"type:text" json:"args"` // JSON 编码的 []string
	APIKeyEnv   string    `gorm:"size:64" json:"api_key_env"`
	Timeout     string    `gorm:"size:32" json:"timeout"` // time.Duration 字符串，如 "300s"
	Enabled     *bool     `gorm:"not null;default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
