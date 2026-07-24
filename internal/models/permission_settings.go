package models

import "time"

// 全局权限模式常量。
const (
	PermissionModeNormal = "normal" // 默认：按 allow/ask/deny 列表匹配，未命中则询问
	PermissionModeYolo   = "yolo"   // 全部自动放行（deny 命中除外）
)

// PermissionSettings 是用户级全局权限规则配置（yolo / 白名单 / 黑名单）。
// 运行时在 permissionBroker.request() 最前面匹配 ToolCall.Title，决定自动放行 / 拒绝 / 询问。
// Allow/Ask/Deny 为 JSON 编码的 []string，每条规则支持 `*` 通配符（如 "Bash(git status:*)"）。
type PermissionSettings struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"uniqueIndex;not null" json:"user_id"`
	Mode      string    `gorm:"size:16;not null;default:'normal'" json:"mode"` // normal | yolo
	Allow     string    `gorm:"type:text" json:"allow"`                        // JSON []string，白名单（命中→放行）
	Ask       string    `gorm:"type:text" json:"ask"`                          // JSON []string，询问名单（命中→强制询问）
	Deny      string    `gorm:"type:text" json:"deny"`                         // JSON []string，黑名单（命中→拒绝，最高优先级）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
