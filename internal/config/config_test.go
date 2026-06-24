package config

import (
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
