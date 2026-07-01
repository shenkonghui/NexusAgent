package models

import "time"

// Note 是用户全局笔记（不关联工作区）。
type Note struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Title     string    `gorm:"size:256;not null" json:"title"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Tags            string `gorm:"type:text" json:"tags"` // JSON 数组，如 ["work","idea"]
	ClassifyPending bool   `gorm:"index;not null;default:false" json:"classify_pending"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
