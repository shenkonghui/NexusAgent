package acp

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"nexusagent/internal/models"
)

// ConfigBackend 是基于 models.AgentConfig 的通用 ACP 后端实现。
// 用于设置页面动态添加的 agent（claude code / codebuddy / devin 等）。
type ConfigBackend struct {
	cfg models.AgentConfig
}

// NewConfigBackend 根据数据库配置创建后端。
func NewConfigBackend(cfg models.AgentConfig) *ConfigBackend {
	return &ConfigBackend{cfg: cfg}
}

func (b *ConfigBackend) Name() string {
	return b.cfg.Type
}

func (b *ConfigBackend) Command() string {
	return b.cfg.Command
}

func (b *ConfigBackend) Args() []string {
	if b.cfg.Args == "" {
		return nil
	}
	var args []string
	if err := json.Unmarshal([]byte(b.cfg.Args), &args); err != nil {
		return nil
	}
	return args
}

func (b *ConfigBackend) Env() []string {
	var envs []string
	if b.cfg.APIKeyEnv != "" {
		if key := os.Getenv(b.cfg.APIKeyEnv); key != "" {
			envs = append(envs, b.cfg.APIKeyEnv+"="+key)
		}
	}
	return envs
}

func (b *ConfigBackend) Timeout() time.Duration {
	if b.cfg.Timeout == "" {
		return 300 * time.Second
	}
	d, err := time.ParseDuration(b.cfg.Timeout)
	if err != nil {
		return 300 * time.Second
	}
	if d <= 0 {
		return 300 * time.Second
	}
	return d
}

// ConfigBackendFromParams 根据手工参数构造后端，供测试与动态注册使用。
func ConfigBackendFromParams(name, command string, args []string, apiKeyEnv, timeout string) (*ConfigBackend, error) {
	argsJSON := ""
	if len(args) > 0 {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("编码 args: %w", err)
		}
		argsJSON = string(b)
	}
	return NewConfigBackend(models.AgentConfig{
		Type:      name,
		Command:   command,
		Args:      argsJSON,
		APIKeyEnv: apiKeyEnv,
		Timeout:   timeout,
		Enabled:   true,
	}), nil
}
