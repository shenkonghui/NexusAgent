package acp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"opennexus/internal/models"
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
	// 注入自定义环境变量（如 HTTPS_PROXY 等代理设置），追加在末尾以覆盖父进程同名变量。
	// 附加在 API Key 之后，避免 API Key 逻辑被自定义变量覆盖。
	if b.cfg.Env != "" {
		var extra map[string]string
		if err := json.Unmarshal([]byte(b.cfg.Env), &extra); err == nil {
			for k, v := range extra {
				envs = append(envs, k+"="+v)
			}
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

// Preparable 是可选的后端预处理接口。
// 实现该接口的后端在启动进程前需要执行准备工作（如下载二进制），
// 失败时返回的 error 会经由 buildConnection 透传给用户。
type Preparable interface {
	Prepare() error
}

// BinaryBackend 是二进制分发 agent 的后端，通过 Prepare() 触发下载解压。
// 嵌入 ConfigBackend，仅覆盖 Command() 方法的返回值。
// 下载失败时允许后续调用重试（配合健康检查重连）。
type BinaryBackend struct {
	*ConfigBackend
	info    BinaryInstallInfo
	mu      sync.Mutex
	cmdPath string
	prepareErr error // 上次 Prepare() 的错误，供未调用 Prepare 时 Command() 返回有意义信息
}

// NewBinaryBackend 根据已有的 AgentConfig 和 BinaryInstallInfo 创建延迟下载的后端。
func NewBinaryBackend(cfg models.AgentConfig, info BinaryInstallInfo) *BinaryBackend {
	return &BinaryBackend{
		ConfigBackend: NewConfigBackend(cfg),
		info:          info,
	}
}

// Prepare 下载并解压二进制文件到缓存目录，成功后缓存路径供 Command() 使用。
// 并发安全：多次调用时，已成功则跳过，已失败则重新下载。
func (b *BinaryBackend) Prepare() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 已有缓存路径且文件仍存在 → 跳过
	if b.cmdPath != "" {
		if _, err := os.Stat(b.cmdPath); err == nil {
			return nil
		}
		b.cmdPath = ""
	}

	slog.Info("开始安装 binary agent", "agent", b.cfg.Type, "url", b.info.ArchiveURL)
	path, err := ensureBinaryInstalled(b.cfg.Type, b.info.Version, b.info.ArchiveURL, b.info.BinaryCmd)
	if err != nil {
		b.prepareErr = err
		slog.Error("安装 binary agent 失败", "agent", b.cfg.Type, "err", err)
		return fmt.Errorf("二进制下载失败: %w", err)
	}
	b.cmdPath = path
	b.prepareErr = nil
	return nil
}

// Command 返回二进制文件的完整路径。
// 不再触发下载——下载由 Prepare() 负责，buildConnection 会在启动进程前调用。
// 若 Prepare() 未调用或失败，返回空串，由 process.go 给出 PATH 相关提示。
func (b *BinaryBackend) Command() string {
	b.mu.Lock()
	defer b.mu.Unlock()
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
