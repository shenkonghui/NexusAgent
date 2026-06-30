package models

import "time"

const (
	WorkspaceModePersistent = "persistent"
	WorkspaceModeTemporary  = "temporary"
)

// Workspace 用户级工作区，绑定固定文件系统目录。
// 每个 Session 归属于某个 Workspace，Session 的 cwd 从 Workspace 继承。
type Workspace struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Cwd       string    `gorm:"size:512;not null" json:"cwd"`
	Mode      string    `gorm:"size:32;not null;default:persistent" json:"mode"`
	TempDir   string    `gorm:"size:512" json:"temp_dir"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
