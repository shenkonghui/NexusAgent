package acp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"

	"nexusagent/internal/models"
)

func TestMapUpdate_UserMessageChunk(t *testing.T) {
	update := acp.SessionUpdate{
		UserMessageChunk: &acp.SessionUpdateUserMessageChunk{
			Content: acp.ContentBlock{
				Text: &acp.ContentBlockText{Text: "你好世界", Type: "text"},
			},
			SessionUpdate: "user_message_chunk",
		},
	}

	msg := MapUpdate("acp-session-x", 42, 7, update)

	if msg.SessionID != "acp-session-x" {
		t.Errorf("SessionID = %q", msg.SessionID)
	}
	if msg.DBSessionID != 42 {
		t.Errorf("DBSessionID = %d", msg.DBSessionID)
	}
	if msg.Sequence != 7 {
		t.Errorf("Sequence = %d", msg.Sequence)
	}
	if msg.Role != models.MessageRoleUser {
		t.Errorf("Role = %q, 期望 user", msg.Role)
	}
	if msg.Kind != models.MessageKindUserMessageChunk {
		t.Errorf("Kind = %q, 期望 user_message_chunk", msg.Kind)
	}
	if msg.Content != "你好世界" {
		t.Errorf("Content = %q", msg.Content)
	}
	if msg.RawJSON == "" {
		t.Error("RawJSON 不应为空")
	}
	if !strings.Contains(msg.RawJSON, "user_message_chunk") {
		t.Errorf("RawJSON 应包含 sessionUpdate 标识，实际 %s", msg.RawJSON)
	}
}

func TestMapUpdate_AgentMessageChunk(t *testing.T) {
	update := acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.ContentBlock{
				Text: &acp.ContentBlockText{Text: "我是回复", Type: "text"},
			},
			SessionUpdate: "agent_message_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
	if msg.Kind != models.MessageKindAgentMessageChunk {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "我是回复" {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestMapUpdate_AgentMessageChunk_TextNil(t *testing.T) {
	update := acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content:       acp.ContentBlock{},
			SessionUpdate: "agent_message_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Content != "" {
		t.Errorf("Text 为 nil 时 Content 应为空，实际 %q", msg.Content)
	}
}

func TestMapUpdate_AgentThoughtChunk(t *testing.T) {
	update := acp.SessionUpdate{
		AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{
			Content: acp.ContentBlock{
				Text: &acp.ContentBlockText{Text: "思考中...", Type: "text"},
			},
			SessionUpdate: "agent_thought_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
	if msg.Kind != models.MessageKindAgentThoughtChunk {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "思考中..." {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestMapUpdate_ToolCall(t *testing.T) {
	update := acp.SessionUpdate{
		ToolCall: &acp.SessionUpdateToolCall{
			Title:         "执行 grep 搜索",
			ToolCallId:    "tc-1",
			SessionUpdate: "tool_call",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleTool {
		t.Errorf("Role = %q, 期望 tool", msg.Role)
	}
	if msg.Kind != models.MessageKindToolCall {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "执行 grep 搜索" {
		t.Errorf("Content = %q, 期望取 Title", msg.Content)
	}
}

func TestMapUpdate_ToolCallUpdate(t *testing.T) {
	title := "更新后的标题"
	update := acp.SessionUpdate{
		ToolCallUpdate: &acp.SessionToolCallUpdate{
			Title:         &title,
			ToolCallId:    "tc-1",
			SessionUpdate: "tool_call_update",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Kind != models.MessageKindToolCallUpdate {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "更新后的标题" {
		t.Errorf("Content = %q, 期望取 Title", msg.Content)
	}
}

func TestMapUpdate_ToolCallUpdate_TitleNil(t *testing.T) {
	update := acp.SessionUpdate{
		ToolCallUpdate: &acp.SessionToolCallUpdate{
			ToolCallId:    "tc-1",
			SessionUpdate: "tool_call_update",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Content != "" {
		t.Errorf("Title 为 nil 时 Content 应为空，实际 %q", msg.Content)
	}
}

func TestMapUpdate_Plan(t *testing.T) {
	update := acp.SessionUpdate{
		Plan: &acp.SessionUpdatePlan{
			Entries:       []acp.PlanEntry{},
			SessionUpdate: "plan",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
	if msg.Kind != models.MessageKindPlan {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "" {
		t.Errorf("Plan 的 Content 应为空，实际 %q", msg.Content)
	}
}

func TestMapUpdate_UsageUpdate(t *testing.T) {
	update := acp.SessionUpdate{
		UsageUpdate: &acp.SessionUsageUpdate{
			Size:          1000,
			Used:          500,
			SessionUpdate: "usage_update",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Kind != models.MessageKindUsageUpdate {
		t.Errorf("Kind = %q", msg.Kind)
	}
}

func TestMapUpdate_Unknown(t *testing.T) {
	update := acp.SessionUpdate{}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Kind != models.MessageKindUnknown {
		t.Errorf("Kind = %q, 期望 unknown", msg.Kind)
	}
	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
}

func TestMapUpdate_RawJSONIsValidJSON(t *testing.T) {
	update := acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content:       acp.ContentBlock{Text: &acp.ContentBlockText{Text: "hi", Type: "text"}},
			SessionUpdate: "agent_message_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	var m map[string]any
	if err := json.Unmarshal([]byte(msg.RawJSON), &m); err != nil {
		t.Fatalf("RawJSON 不是有效 JSON: %v", err)
	}
	if m["sessionUpdate"] != "agent_message_chunk" {
		t.Errorf("RawJSON 中 sessionUpdate 字段 = %v", m["sessionUpdate"])
	}
}
