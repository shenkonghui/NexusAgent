package acp

import (
	"encoding/json"

	"github.com/coder/acp-go-sdk"

	"nexusagent/internal/models"
)

// MapUpdate 将 acp.SessionUpdate 映射为 models.Message 的字段值。
// 检测 SessionUpdate 的哪个变体指针非 nil，提取 kind / role / content / raw_json。
func MapUpdate(sessionID string, dbSessionID uint, seq int, update acp.SessionUpdate) models.Message {
	kind, role := extractKindRole(update)
	content := extractContent(update)
	rawJSON := ""

	data, err := json.Marshal(update)
	if err == nil {
		rawJSON = string(data)
	}

	return models.Message{
		SessionID:   sessionID,
		DBSessionID: dbSessionID,
		Role:        role,
		Kind:        kind,
		Content:     content,
		RawJSON:     rawJSON,
		Sequence:    seq,
	}
}

// extractKindRole 根据 SessionUpdate 的变体指针提取 kind 和 role。
func extractKindRole(update acp.SessionUpdate) (kind, role string) {
	switch {
	case update.UserMessageChunk != nil:
		return models.MessageKindUserMessageChunk, models.MessageRoleUser
	case update.AgentMessageChunk != nil:
		return models.MessageKindAgentMessageChunk, models.MessageRoleAssistant
	case update.AgentThoughtChunk != nil:
		return models.MessageKindAgentThoughtChunk, models.MessageRoleAssistant
	case update.ToolCall != nil:
		return models.MessageKindToolCall, models.MessageRoleTool
	case update.ToolCallUpdate != nil:
		return models.MessageKindToolCallUpdate, models.MessageRoleTool
	case update.Plan != nil:
		return models.MessageKindPlan, models.MessageRoleAssistant
	case update.PlanUpdate != nil:
		return models.MessageKindPlanUpdate, models.MessageRoleAssistant
	case update.PlanRemoved != nil:
		return models.MessageKindPlanRemoved, models.MessageRoleAssistant
	case update.SessionInfoUpdate != nil:
		return models.MessageKindSessionInfoUpdate, models.MessageRoleAssistant
	case update.UsageUpdate != nil:
		return models.MessageKindUsageUpdate, models.MessageRoleAssistant
	case update.CurrentModeUpdate != nil:
		return models.MessageKindCurrentModeUpdate, models.MessageRoleAssistant
	default:
		return models.MessageKindUnknown, models.MessageRoleAssistant
	}
}

// extractContent 从 SessionUpdate 中提取可读文本内容。
// user/agent/thought chunk 取 Content.Text.Text；tool_call 取 Title；tool_call_update 取 Title 指针。
func extractContent(update acp.SessionUpdate) string {
	switch {
	case update.UserMessageChunk != nil:
		if update.UserMessageChunk.Content.Text != nil {
			return update.UserMessageChunk.Content.Text.Text
		}
	case update.AgentMessageChunk != nil:
		if update.AgentMessageChunk.Content.Text != nil {
			return update.AgentMessageChunk.Content.Text.Text
		}
	case update.AgentThoughtChunk != nil:
		if update.AgentThoughtChunk.Content.Text != nil {
			return update.AgentThoughtChunk.Content.Text.Text
		}
	case update.ToolCall != nil:
		return update.ToolCall.Title
	case update.ToolCallUpdate != nil:
		if update.ToolCallUpdate.Title != nil {
			return *update.ToolCallUpdate.Title
		}
	}
	return ""
}
