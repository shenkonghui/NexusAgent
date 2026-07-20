package acp

import (
	"encoding/json"
	"testing"
	"time"

	"opennexus/internal/models"
)

func TestConfigBackend_BasicFields(t *testing.T) {
	enabled := true
	cfg := models.AgentConfig{
		Type:      "codebuddy",
		Command:   "codebuddy",
		Args:       `["--acp","--port","8080"]`,
		APIKeyEnv: "CODEBUDDY_API_KEY",
		Timeout:   "120s",
		Enabled:   &enabled,
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

func TestFixNpmExecArgs(t *testing.T) {
	in := []string{"exec", "--include=optional", "--yes", "@tencent-ai/codebuddy-code@2.106.7", "--acp"}
	want := []string{"exec", "--include=optional", "--yes", "@tencent-ai/codebuddy-code@2.106.7", "--", "--acp"}
	got := fixNpmExecArgs("npm", in)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
	// 已有 "--" 时不重复插入
	already := []string{"exec", "--include=optional", "--yes", "pkg", "--", "--acp"}
	if fixNpmExecArgs("npm", already) != nil {
		// slice compare - same content
		got2 := fixNpmExecArgs("npm", already)
		if len(got2) != len(already) {
			t.Errorf("已有 -- 时不应修改: %+v", got2)
		}
	}
	// 非 npm exec 不修改
	plain := []string{"--acp"}
	if fixNpmExecArgs("codebuddy", plain)[0] != "--acp" {
		t.Errorf("非 npm 命令不应修改 args")
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

// envInSlice 判断 envs 是否包含指定项。
func envInSlice(envs []string, want string) bool {
	for _, e := range envs {
		if e == want {
			return true
		}
	}
	return false
}

func TestConfigBackend_EnvInjection(t *testing.T) {
	t.Setenv("CODEBUDDY_API_KEY", "secret-key")
	enabled := true
	cfg := models.AgentConfig{
		Type:      "codebuddy",
		Command:   "codebuddy",
		APIKeyEnv: "CODEBUDDY_API_KEY",
		Enabled:   &enabled,
		Env:       `{"HTTPS_PROXY":"http://127.0.0.1:7890","HTTP_PROXY":"http://127.0.0.1:7890"}`,
	}
	b := NewConfigBackend(cfg)
	envs := b.Env()

	// API Key 与自定义环境变量都应被注入
	if !envInSlice(envs, "CODEBUDDY_API_KEY=secret-key") {
		t.Errorf("缺少 API Key 注入: %+v", envs)
	}
	if !envInSlice(envs, "HTTPS_PROXY=http://127.0.0.1:7890") {
		t.Errorf("缺少 HTTPS_PROXY 注入: %+v", envs)
	}
	if !envInSlice(envs, "HTTP_PROXY=http://127.0.0.1:7890") {
		t.Errorf("缺少 HTTP_PROXY 注入: %+v", envs)
	}
}

func TestConfigBackend_EnvInvalidJSON(t *testing.T) {
	// 非法 JSON 的 Env 不应 panic，且不影响其它注入。
	b := NewConfigBackend(models.AgentConfig{
		Type:    "x",
		Command: "x",
		Env:     "not-json",
	})
	envs := b.Env()
	if len(envs) != 0 {
		t.Errorf("非法 Env 应产生 0 个变量, 实际 %+v", envs)
	}
}

func TestConfigBackend_EnvEmpty(t *testing.T) {
	b := NewConfigBackend(models.AgentConfig{Type: "x", Command: "x", Env: ""})
	if got := b.Env(); len(got) != 0 {
		t.Errorf("空 Env 应返回空切片, 实际 %+v", got)
	}
}
