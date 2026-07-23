package models

import "time"

// ACPConnection 记录每个 acp 子进程的心跳状态，供 watchdog 独立进程判定空闲与存活。
//
// 通信模型（主 server 与 watchdog 共享同一 SQLite）：
//   - 主 server 在建连 / 每次发 prompt / 每 30s 心跳时写入该表。
//   - watchdog 每 60s 扫描：
//       1. last_active_at 超过空闲阈值 → kill 该进程组 + 删行。
//       2. server_heartbeat 全部超时未续约 → 主程序已死 → kill 表中全部 + 自退。
//
// Pid 存的是 acp 子进程的直系 PID。由于 setProcessGroup 使用 Setsid 使其成为
// 新会话组长（PGID == PID），watchdog 可用 kill(-pid) 杀掉整个进程组（含 npm/cursor 派生的孙进程）。
type ACPConnection struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	PoolKey string `gorm:"uniqueIndex;size:600;not null" json:"-"` // agentType + cwd，与 Service.pool 的键一致
	// AgentType acp 后端类型（如 claude-code、cursor-agent）。
	AgentType string `gorm:"size:64;index" json:"agent_type"`
	// Cwd 子进程工作目录（连接池键的第二维）。
	Cwd string `gorm:"size:512" json:"cwd"`
	// Pid acp 直系子进程 PID（同时也是进程组 PGID）。
	Pid int `json:"pid"`
	// LastActiveAt 最后一次 prompt 活动时间，watchdog 据此判定空闲。
	LastActiveAt time.Time `json:"last_active_at"`
	// ServerHeartbeat 主 server 每次心跳续约时间；watchdog 据此判断主程序是否存活。
	ServerHeartbeat time.Time `json:"server_heartbeat"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 显式指定表名。
func (ACPConnection) TableName() string { return "acp_connections" }
