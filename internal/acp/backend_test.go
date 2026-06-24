package acp

import (
	"testing"
	"time"

	"nexusagent/internal/config"
)

func TestClaudeCodeBackend_Name(t *testing.T) {
	b := NewClaudeCodeBackend(config.ClaudeCodeConfig{
		Enabled:   true,
		Command:   "npx",
		Args:      []string{"-y", "@zed-industries/claude-code-acp@latest"},
		APIKeyEnv: "ANTHROPIC_API_KEY",
		Timeout:   60 * time.Second,
	})
	if b.Name() != "claude-code" {
		t.Errorf("Name() = %q, 期望 claude-code", b.Name())
	}
}

func TestClaudeCodeBackend_Command(t *testing.T) {
	b := NewClaudeCodeBackend(config.ClaudeCodeConfig{
		Command: "npx",
		Args:    []string{"-y", "@zed-industries/claude-code-acp@latest"},
	})
	if b.Command() != "npx" {
		t.Errorf("Command() = %q, 期望 npx", b.Command())
	}
	if len(b.Args()) != 2 || b.Args()[0] != "-y" {
		t.Errorf("Args() = %v", b.Args())
	}
}

func TestClaudeCodeBackend_Env_WithAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-123")
	b := NewClaudeCodeBackend(config.ClaudeCodeConfig{
		APIKeyEnv: "ANTHROPIC_API_KEY",
	})
	envs := b.Env()
	found := false
	for _, e := range envs {
		if e == "ANTHROPIC_API_KEY=test-key-123" {
			found = true
		}
	}
	if !found {
		t.Errorf("期望 Env() 包含 ANTHROPIC_API_KEY=test-key-123, 实际 %v", envs)
	}
}

func TestClaudeCodeBackend_Env_WithoutAPIKey(t *testing.T) {
	b := NewClaudeCodeBackend(config.ClaudeCodeConfig{
		APIKeyEnv: "ANTHROPIC_API_KEY",
	})
	envs := b.Env()
	for _, e := range envs {
		if e == "ANTHROPIC_API_KEY=" || e == "ANTHROPIC_API_KEY" {
			t.Errorf("不应在无 API key 时注入空值: %v", envs)
		}
	}
}

func TestClaudeCodeBackend_Timeout(t *testing.T) {
	b := NewClaudeCodeBackend(config.ClaudeCodeConfig{
		Timeout: 120 * time.Second,
	})
	if b.Timeout() != 120*time.Second {
		t.Errorf("Timeout() = %v, 期望 120s", b.Timeout())
	}
}
