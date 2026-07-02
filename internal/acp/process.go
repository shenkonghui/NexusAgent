package acp

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Process 管理 agent 子进程的生命周期。
type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	backend Backend
}

// NewProcess 启动指定后端的 agent 子进程。
// workDir 非空时设为子进程工作目录，使 agent 工具调用与 ACP session cwd 一致。
func NewProcess(backend Backend, workDir string) (*Process, error) {
	cmd := exec.Command(backend.Command(), backend.Args()...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append(cmd.Environ(), backend.Env()...)

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
func (p *Process) Stop() error {
	if p.cmd.Process == nil {
		return nil
	}
	_ = p.stdin.Close()
	if err := p.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("停止 agent 进程: %w", err)
	}
	_, _ = p.cmd.Process.Wait()
	return nil
}
