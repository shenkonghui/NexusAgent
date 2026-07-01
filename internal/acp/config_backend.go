package acp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
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
	return fixNpmExecArgs(b.cfg.Command, args)
}

// fixNpmExecArgs 为 npm exec 命令在 package 参数后插入 "--"，
// 确保 package 专属参数（如 --acp）传给子进程而非被 npm 忽略。
func fixNpmExecArgs(command string, args []string) []string {
	if command != "npm" || len(args) < 4 || args[0] != "exec" {
		return args
	}
	for _, a := range args {
		if a == "--" {
			return args
		}
	}
	// 格式: exec --include=optional --yes <package> [packageArgs...]
	if len(args) == 4 {
		return args
	}
	fixed := append(append([]string{}, args[:4]...), "--")
	return append(fixed, args[4:]...)
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

// BinaryBackend 是二进制分发 agent 的后端，Command() 时自动下载解压。
// 嵌入 ConfigBackend，仅覆盖 Command() 方法的返回值。
// 下载失败时允许后续调用重试（配合健康检查重连）。
type BinaryBackend struct {
	*ConfigBackend
	info    BinaryInstallInfo
	mu      sync.Mutex
	cmdPath string
}

// NewBinaryBackend 根据已有的 AgentConfig 和 BinaryInstallInfo 创建延迟下载的后端。
func NewBinaryBackend(cfg models.AgentConfig, info BinaryInstallInfo) *BinaryBackend {
	return &BinaryBackend{
		ConfigBackend: NewConfigBackend(cfg),
		info:          info,
	}
}

// Command 返回二进制文件的完整路径，必要时触发下载解压。
func (b *BinaryBackend) Command() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cmdPath != "" {
		if _, err := os.Stat(b.cmdPath); err == nil {
			return b.cmdPath
		}
		b.cmdPath = ""
	}

	slog.Info("开始安装 binary agent", "agent", b.cfg.Type, "url", b.info.ArchiveURL)
	path, err := ensureBinaryInstalled(b.cfg.Type, b.info.Version, b.info.ArchiveURL, b.info.BinaryCmd)
	if err != nil {
		slog.Error("安装 binary agent 失败", "agent", b.cfg.Type, "err", err)
		return ""
	}
	b.cmdPath = path
	return b.cmdPath
}

// NewBackendFromAgentConfig 根据 AgentConfig 创建合适的后端。
// 若该 agent 在 BinaryRegistry 中注册，则创建 BinaryBackend（延迟下载），否则创建普通 ConfigBackend。
func NewBackendFromAgentConfig(cfg models.AgentConfig) Backend {
	if info, ok := BinaryRegistry[cfg.Type]; ok {
		return NewBinaryBackend(cfg, info)
	}
	return NewConfigBackend(cfg)
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
	enabled := true
	return NewConfigBackend(models.AgentConfig{
		Type:      name,
		Command:   command,
		Args:      argsJSON,
		APIKeyEnv: apiKeyEnv,
		Timeout:   timeout,
		Enabled:   &enabled,
	}), nil
}
