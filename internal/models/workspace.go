package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const (
	WorkspaceModePersistent = "persistent"
	WorkspaceModeTemporary  = "temporary"
)

// StringArray 自定义类型：在数据库中以 JSON 字符串存储，在 Go / JSON API 中表现为 []string。
type StringArray []string

// Value 实现 driver.Valuer，将 []string 序列化为 JSON 存入数据库。
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "[]", nil
	}
	b, err := json.Marshal(a)
	return string(b), err
}

// Scan 实现 sql.Scanner，从数据库读取 JSON 字符串并反序列化为 []string。
func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("无法扫描 %T 到 StringArray", value)
	}
	if len(bytes) == 0 {
		*a = nil
		return nil
	}
	return json.Unmarshal(bytes, a)
}

// Workspace 用户级工作区，绑定固定文件系统目录。
// 每个 Session 归属于某个 Workspace，Session 的 cwd 从 Workspace 继承。
// Cwd 为主目录（primary），Directories 为附加目录（secondary），均可被 Agent 访问。
type Workspace struct {
	ID          uint        `gorm:"primaryKey" json:"id"`
	UserID      uint        `gorm:"index;not null" json:"user_id"`
	Name        string      `gorm:"size:128;not null" json:"name"`
	Cwd         string      `gorm:"size:512;not null" json:"cwd"`              // 主目录
	Directories StringArray `gorm:"size:4096" json:"directories"`              // 附加目录（次级）
	Mode        string      `gorm:"size:32;not null;default:persistent" json:"mode"`
	TempDir     string      `gorm:"size:512" json:"temp_dir"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
