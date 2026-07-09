package models

import "time"

// TaskSettings 是用户任务自动打标签 / 自动生成标题的配置。
type TaskSettings struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	UserID      uint   `gorm:"uniqueIndex;not null" json:"user_id"`
	AutoTagEnabled   bool   `gorm:"not null;default:false" json:"auto_tag_enabled"`
	AutoTitleEnabled bool   `gorm:"not null;default:true" json:"auto_title_enabled"`
	// AgentType / ModelValue 用于分类和生成标题的临时会话（RunPromptOnce）。
	AgentType  string `gorm:"size:64" json:"agent_type"`
	ModelValue string `gorm:"size:128" json:"model_value"`
	// Tags 是预定义标签 JSON 数组，如 ["后端","前端","mysql"]。
	Tags string `gorm:"type:text" json:"tags"`
	// TagPrompt / TitlePrompt 是自定义提示词模板（空则用默认）。
	TagPrompt   string `gorm:"type:text" json:"tag_prompt"`
	TitlePrompt string `gorm:"type:text" json:"title_prompt"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
