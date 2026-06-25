package acp

import (
	"encoding/json"
	"testing"
	"time"

	"nexusagent/internal/models"
)

func TestConfigBackend_BasicFields(t *testing.T) {
	cfg := models.AgentConfig{
		Type:      "codebuddy",
		Command:   "codebuddy",
		Args:       `["--acp","--port","8080"]`,
		APIKeyEnv: "CODEBUDDY_API_KEY",
		Timeout:   "120s",
		Enabled:   true,
	}
	b := NewConfigBackend(cfg)
	if b.Name() != "codebuddy" {
		t.Errorf("Name = %q", b.Name())
	}
	if b.Command() != "codebuddy" {
		t.Errorf("Command = %q", b.Command())
	}
	args := b.Args()
	if len(args) != 3 || args[0] != "--acp" {
		t.Errorf("Args = %+v", args)
	}
	if d := b.Timeout(); d != 120*time.Second {
		t.Errorf("Timeout = %v", d)
	}
}

func TestConfigBackend_Defaults(t *testing.T) {
	b := NewConfigBackend(models.AgentConfig{Type: "x", Command: "x"})
	if b.Args() != nil {
		t.Errorf("空 Args 期望 nil, 实际 %+v", b.Args())
	}
	if d := b.Timeout(); d != 300*time.Second {
		t.Errorf("空 Timeout 期望 300s, 实际 %v", d)
	}
}

func TestConfigBackend_InvalidArgsAndTimeout(t *testing.T) {
	b := NewConfigBackend(models.AgentConfig{
		Type:    "x",
		Command: "x",
		Args:    "not-json",
		Timeout: "bad",
	})
	if b.Args() != nil {
		t.Errorf("非法 Args 期望 nil, 实际 %+v", b.Args())
	}
	if d := b.Timeout(); d != 300*time.Second {
		t.Errorf("非法 Timeout 期望回退 300s, 实际 %v", d)
	}
}

func TestConfigBackendFromParams(t *testing.T) {
	b, err := ConfigBackendFromParams("devin", "devin", []string{"--acp"}, "DEVIN_KEY", "90s")
	if err != nil {
		t.Fatalf("ConfigBackendFromParams 错误: %v", err)
	}
	if b.Name() != "devin" || b.Timeout() != 90*time.Second {
		t.Errorf("后端字段不正确: %+v", b)
	}
	// 验证 args 已正确编码
	var args []string
	_ = json.Unmarshal([]byte(b.cfg.Args), &args)
	if len(args) != 1 || args[0] != "--acp" {
		t.Errorf("args 编码不正确: %s", b.cfg.Args)
	}
}
