package acp

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// stopGracePeriod 是 Stop() 发送 SIGTERM 后等待进程自行退出的宽限时间，
// 超过后升级为 SIGKILL（强制终止进程组）。
const stopGracePeriod = 2 * time.Second

// Process 管理 agent 子进程的生命周期。
type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	backend Backend
}

// resolveAgentCommand 定位可执行文件。
// 对带路径的占位命令（如 ./dist-package/cursor-agent），取最后一段在 PATH 中查找。
func resolveAgentCommand(command string) (string, error) {
	path, err := exec.LookPath(command)
	if err == nil {
		return path, nil
	}
	base := filepath.Base(command)
	if base == "" || base == "." || base == command {
		return "", err
	}
	return exec.LookPath(base)
}

// NewProcess 启动指定后端的 agent 子进程。
// workDir 非空时设为子进程工作目录，使 agent 工具调用与 ACP session cwd 一致。
func NewProcess(backend Backend, workDir string) (*Process, error) {
	command, args := backend.Command(), backend.Args()
	slog.Debug("启动 agent 子进程",
		"agent", backend.Name(),
		"command", command,
		"args", args,
		"cwd", workDir,
	)

	// 预检命令是否在 PATH 中，给出比 exec 的 "executable file not found" 更明确的提示
	resolved, lookErr := resolveAgentCommand(command)
	if lookErr != nil {
		lookName := filepath.Base(command)
		if lookName == "" || lookName == "." {
			lookName = command
		}
		slog.Error("agent 命令不在 PATH 中",
			"agent", backend.Name(),
			"command", command,
			"look", lookName,
			"args", args,
			"err", lookErr)
		return nil, fmt.Errorf("启动 agent 进程 %s：命令 %q 不在 PATH 中（%w）；请检查命令是否安装或配置是否正确",
			backend.Name(), lookName, lookErr)
	}
	command = resolved

	cmd := exec.Command(command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append(cmd.Environ(), backend.Env()...)
	// 设置独立进程组，便于 Stop() 时按 PGID 一并终止孙进程
	// （npm exec / cursor-agent / codebuddy 等会派生子进程，否则会变成孤儿）。
	setProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stdin 管道: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stdout 管道: %w", err)
	}
	// 将 stderr 重定向到当前进程的 stderr，便于排查 agent 启动错误
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.Error("启动 agent 进程失败",
			"agent", backend.Name(),
			"command", command,
			"args", args,
			"cwd", workDir,
			"err", err)
		return nil, fmt.Errorf("启动 agent 进程 %s: %w", backend.Name(), err)
	}

	return &Process{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		backend: backend,
	}, nil
}

// Stdin 返回子进程的 stdin 管道。
func (p *Process) Stdin() io.WriteCloser {
	return p.stdin
}

// Stdout 返回子进程的 stdout 管道。
func (p *Process) Stdout() io.ReadCloser {
	return p.stdout
}

// Stop 停止子进程。
// 先关闭 stdin，再向进程组发送 SIGTERM 并等待 stopGracePeriod；
// 若进程仍存活则升级为 SIGKILL 强制终止整个进程组（含孙进程）。
// terminateProcessGroup 已回收直系子进程，此处不再重复 Wait。
func (p *Process) Stop() error {
	if p.cmd.Process == nil {
		return nil
	}
	_ = p.stdin.Close()
	if err := terminateProcessGroup(p.cmd.Process, stopGracePeriod); err != nil {
		return fmt.Errorf("停止 agent 进程: %w", err)
	}
	return nil
}
