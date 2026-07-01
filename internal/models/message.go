package models

import "time"

// 消息角色常量
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleTool      = "tool"
)

// 消息 kind 常量（对应 ACP SessionUpdate 的 sessionUpdate 判别字段）
const (
	MessageKindUserMessageChunk  = "user_message_chunk"
	MessageKindAgentMessageChunk = "agent_message_chunk"
	MessageKindAgentThoughtChunk = "agent_thought_chunk"
	MessageKindToolCall          = "tool_call"
	MessageKindToolCallUpdate    = "tool_call_update"
	MessageKindPlan              = "plan"
	MessageKindPlanUpdate        = "plan_update"
	MessageKindPlanRemoved       = "plan_removed"
	MessageKindSessionInfoUpdate = "session_info_update"
	MessageKindUsageUpdate       = "usage_update"
	MessageKindCurrentModeUpdate = "current_mode_update"
	MessageKindPermissionRequest = "permission_request"
	MessageKindUnknown           = "unknown"
)

// Message 是会话消息持久化模型，存储 Prompt 产生的每条 SessionUpdate。
type Message struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	SessionID   string `gorm:"index;size:128;not null" json:"session_id"`
	DBSessionID uint   `gorm:"index;not null" json:"db_session_id"`
	Role        string `gorm:"size:32;not null" json:"role"`
	Kind        string `gorm:"size:64;not null" json:"kind"`
	Content     string `gorm:"type:text" json:"content"`
	RawJSON     string `gorm:"type:text;not null" json:"raw_json"`
	Sequence    int    `gorm:"index;not null" json:"sequence"`
	// ExecutionID 标记定时任务的一次执行块；同一次执行的所有消息共享该 ID。
	// 手动会话的消息该字段为 null。
	ExecutionID *uint     `gorm:"index" json:"execution_id"`
	CreatedAt   time.Time `json:"created_at"`
}
