package acp

import (
	"os"
	"time"

	"opennexus/internal/config"
)

// Backend 定义 ACP agent 后端的接口。
type Backend interface {
	Name() string
	Command() string
	Args() []string
	Env() []string
	Timeout() time.Duration
}

// ClaudeCodeBackend 是 Claude Code agent 的后端实现。
type ClaudeCodeBackend struct {
	cfg config.ClaudeCodeConfig
}

// NewClaudeCodeBackend 根据配置创建 Claude Code 后端。
func NewClaudeCodeBackend(cfg config.ClaudeCodeConfig) *ClaudeCodeBackend {
	return &ClaudeCodeBackend{cfg: cfg}
}

func (b *ClaudeCodeBackend) Name() string {
	return "claude-code"
}

func (b *ClaudeCodeBackend) Command() string {
	return b.cfg.Command
}

func (b *ClaudeCodeBackend) Args() []string {
	return b.cfg.Args
}

func (b *ClaudeCodeBackend) Env() []string {
	var envs []string
	if b.cfg.APIKeyEnv != "" {
		if key := os.Getenv(b.cfg.APIKeyEnv); key != "" {
			envs = append(envs, b.cfg.APIKeyEnv+"="+key)
		}
	}
	return envs
}

func (b *ClaudeCodeBackend) Timeout() time.Duration {
	return b.cfg.Timeout
}
