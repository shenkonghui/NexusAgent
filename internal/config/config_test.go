package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_FromYAML(t *testing.T) {
	cfg, err := Load("testdata/config_test.yaml")
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, 期望 9090", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data/test.db" {
		t.Errorf("Database.Path = %q, 期望 ./data/test.db", cfg.Database.Path)
	}
	if cfg.JWT.AccessTTL != 15*time.Minute {
		t.Errorf("JWT.AccessTTL = %v, 期望 15m", cfg.JWT.AccessTTL)
	}
	if cfg.Password.BcryptCost != 10 {
		t.Errorf("Password.BcryptCost = %d, 期望 10", cfg.Password.BcryptCost)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("JWT_SECRET", "env-secret-from-env-var-long-enough")
	t.Setenv("SERVER_PORT", "7070")
	cfg, err := Load("testdata/config_test.yaml")
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if cfg.JWT.Secret != "env-secret-from-env-var-long-enough" {
		t.Errorf("JWT.Secret 未被环境变量覆盖: %q", cfg.JWT.Secret)
	}
	if cfg.Server.Port != 7070 {
		t.Errorf("Server.Port 未被环境变量覆盖: %d", cfg.Server.Port)
	}
}

func TestValidate_SecretTooShort(t *testing.T) {
	cfg := &Config{JWT: JWTConfig{Secret: "short"}}
	if err := cfg.Validate(); err == nil {
		t.Error("期望 secret 过短时返回错误，实际无错误")
	}
}

func TestValidate_OK(t *testing.T) {
	cfg := &Config{JWT: JWTConfig{Secret: "this-is-a-very-long-jwt-secret-key-32+bytes!"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望校验通过，实际错误: %v", err)
	}
}

func TestLoad_AgentsConfig(t *testing.T) {
	cfg, err := Load("testdata/config_test.yaml")
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if cfg.Agents.Workspace.DefaultMode != "temporary" {
		t.Errorf("Workspace.DefaultMode = %q, 期望 temporary", cfg.Agents.Workspace.DefaultMode)
	}
	if cfg.Agents.Workspace.TempDirPrefix != "test-" {
		t.Errorf("Workspace.TempDirPrefix = %q, 期望 test-", cfg.Agents.Workspace.TempDirPrefix)
	}
	if cfg.Agents.Workspace.SessionDir != "" {
		t.Errorf("Workspace.SessionDir 未设置时应为空，实际 %q", cfg.Agents.Workspace.SessionDir)
	}
	if !cfg.Agents.ClaudeCode.Enabled {
		t.Error("ClaudeCode.Enabled 期望 true")
	}
	if cfg.Agents.ClaudeCode.Command != "npx" {
		t.Errorf("ClaudeCode.Command = %q, 期望 npx", cfg.Agents.ClaudeCode.Command)
	}
	if cfg.Agents.ClaudeCode.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("ClaudeCode.APIKeyEnv = %q, 期望 ANTHROPIC_API_KEY", cfg.Agents.ClaudeCode.APIKeyEnv)
	}
	if cfg.Agents.ClaudeCode.Timeout != 60*time.Second {
		t.Errorf("ClaudeCode.Timeout = %v, 期望 60s", cfg.Agents.ClaudeCode.Timeout)
	}
}

func TestLoad_AgentsConfig_EnvOverride(t *testing.T) {
	t.Setenv("AGENTS_WORKSPACE_DEFAULT_MODE", "external")
	t.Setenv("CLAUDE_CODE_COMMAND", "/usr/local/bin/npx")
	cfg, err := Load("testdata/config_test.yaml")
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if cfg.Agents.Workspace.DefaultMode != "external" {
		t.Errorf("DefaultMode 未被环境变量覆盖: %q", cfg.Agents.Workspace.DefaultMode)
	}
	if cfg.Agents.ClaudeCode.Command != "/usr/local/bin/npx" {
		t.Errorf("Command 未被环境变量覆盖: %q", cfg.Agents.ClaudeCode.Command)
	}
}

func TestValidate_WorkspaceDefaultMode_Invalid(t *testing.T) {
	cfg := &Config{
		JWT:    JWTConfig{Secret: "this-is-a-very-long-jwt-secret-key-32+bytes!"},
		Agents: AgentsConfig{Workspace: WorkspaceConfig{DefaultMode: "invalid"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("期望 default_mode 非法时返回错误")
	}
}

func TestValidate_WorkspaceDefaultMode_OK(t *testing.T) {
	cfg := &Config{
		JWT:    JWTConfig{Secret: "this-is-a-very-long-jwt-secret-key-32+bytes!"},
		Agents: AgentsConfig{Workspace: WorkspaceConfig{DefaultMode: "external"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望 external 校验通过，实际: %v", err)
	}
}

func TestValidate_WorkspaceSessionDir_Default(t *testing.T) {
	cfg := &Config{
		JWT:    JWTConfig{Secret: "this-is-a-very-long-jwt-secret-key-32+bytes!"},
		Agents: AgentsConfig{Workspace: WorkspaceConfig{DefaultMode: "external"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate 错误: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("获取主目录失败: %v", err)
	}
	expected := filepath.Join(home, ".nextAgent", "session")
	if cfg.Agents.Workspace.SessionDir != expected {
		t.Errorf("SessionDir = %q, 期望 %q", cfg.Agents.Workspace.SessionDir, expected)
	}
}

func TestValidate_WorkspaceSessionDir_EnvOverride(t *testing.T) {
	t.Setenv("JWT_SECRET", "env-secret-from-env-var-long-enough")
	t.Setenv("AGENTS_WORKSPACE_SESSION_DIR", "/custom/session-dir")
	cfg, err := Load("testdata/config_test.yaml")
	if err != nil {
		t.Fatalf("Load 错误: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate 错误: %v", err)
	}
	if cfg.Agents.Workspace.SessionDir != "/custom/session-dir" {
		t.Errorf("SessionDir 未被环境变量覆盖: %q", cfg.Agents.Workspace.SessionDir)
	}
}
