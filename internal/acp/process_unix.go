//go:build !windows

package acp

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup 让子进程成为新会话的组长（PGID == SID == 子进程 PID），
// 以便 Stop() 时通过负 PID 向整个进程组发送信号，杀掉 agent 派生的孙进程。
// Setsid 创建新会话，使子进程脱离父进程的控制终端——否则像 codebuddy-code 这类
// 启动时会操作 TTY（交互式登录提示）的 agent，在 server 于前台终端运行时会被
// 作业控制信号（SIGTSTP）挂起，导致 ACP 握手永远得不到响应。
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// terminateProcessGroup 先向进程组发 SIGTERM，等待直系子进程退出；
// 若 grace 超时仍未退出则升级为 SIGKILL 强制终止整个进程组（含孙进程）。
// 调用方不应再调用 proc.Wait()——本方法已回收直系子进程。
func terminateProcessGroup(proc *os.Process, grace time.Duration) error {
	pgid := proc.Pid

	// SIGTERM 先发，给 agent 自行清理子进程的机会。
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	// 监听直系子进程退出；超时则 SIGKILL 整组强制终止。
	waitCh := make(chan error, 1)
	go func() {
		_, err := proc.Wait()
		waitCh <- err
	}()
	select {
	case <-waitCh:
		return nil
	case <-time.After(grace):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-waitCh
		return nil
	}
}

// probeProcessState 探测直系子进程（cmd.Process，通常是 npm exec 等 wrapper）的状态。
// 返回值：
//   - procStateExited：直系子进程已终止（agent 大概率也已退出）
//   - procStateRunning：直系子进程仍在运行（agent 可能正常、无响应或被信号停止）
//
// 注意：被信号停止（SIGTSTP）的通常是 wrapper 派生的孙进程（真正的 agent），
// 父进程无法用 Wait4 探测孙进程状态，故此处不区分 stopped。
// 「进程存活但握手无响应」的诊断由 unresponsiveDiagnosis 给出，其中会提示
// 可能被作业控制挂起等待用户检查。
func probeProcessState(pid int) procState {
	// signal 0 不实际发送信号，仅做存在性检查；返回 ESRCH 表示进程已退出。
	if err := syscall.Kill(pid, 0); err != nil {
		return procStateExited
	}
	return procStateRunning
}
