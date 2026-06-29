package agent

import (
	"testing"

	"nexusagent/internal/acp"
	"nexusagent/internal/config"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	b := acp.NewClaudeCodeBackend(config.ClaudeCodeConfig{})
	desc := &AgentDescriptor{
		Type:        "claude-code",
		DisplayName: "Claude Code",
		Description: "Anthropic Claude Code",
		Backend:     b,
	}
	if err := r.Register(desc); err != nil {
		t.Fatalf("Register 错误: %v", err)
	}

	got, err := r.Get("claude-code")
	if err != nil {
		t.Fatalf("Get 错误: %v", err)
	}
	if got.DisplayName != "Claude Code" {
		t.Errorf("DisplayName = %q", got.DisplayName)
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	desc := &AgentDescriptor{
		Type:    "claude-code",
		Backend: acp.NewClaudeCodeBackend(config.ClaudeCodeConfig{}),
	}
	_ = r.Register(desc)
	if err := r.Register(desc); err == nil {
		t.Error("期望重复注册返回错误")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("unknown"); err == nil {
		t.Error("期望未注册类型返回错误")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&AgentDescriptor{Type: "a", Backend: acp.NewClaudeCodeBackend(config.ClaudeCodeConfig{})})
	_ = r.Register(&AgentDescriptor{Type: "b", Backend: acp.NewClaudeCodeBackend(config.ClaudeCodeConfig{})})

	list := r.List()
	if len(list) != 2 {
		t.Errorf("期望 2 个 agent，实际 %d", len(list))
	}
}

func TestRouter_ListAgents(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&AgentDescriptor{Type: "claude-code", DisplayName: "Claude Code", Backend: acp.NewClaudeCodeBackend(config.ClaudeCodeConfig{})})
	router := NewRouter(r, nil)

	agents := router.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("期望 1 个 agent，实际 %d", len(agents))
	}
	if agents[0].Type != "claude-code" {
		t.Errorf("Type = %q", agents[0].Type)
	}
}

func TestRouter_CreateSession_UnknownAgent(t *testing.T) {
	r := NewRegistry()
	router := NewRouter(r, nil)

	if _, err := router.CreateSession(nil, "unknown", "/tmp", 1, ""); err == nil {
		t.Error("期望未知 agent 类型返回错误")
	}
}

func TestRouter_NewMethods_NilService(t *testing.T) {
	r := NewRegistry()
	router := NewRouter(r, nil)

	if _, err := router.GetSessionByDBID(1); err == nil {
		t.Error("期望 GetSessionByDBID 在 service 为 nil 时返回错误")
	}
	if _, err := router.ListMessages("x"); err == nil {
		t.Error("期望 ListMessages 在 service 为 nil 时返回错误")
	}
	if _, err := router.ResumeSession(nil, "x", ""); err == nil {
		t.Error("期望 ResumeSession 在 service 为 nil 时返回错误")
	}
}
