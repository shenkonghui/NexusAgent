//go:build !windows

package acp

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup 让子进程成为独立进程组的组长（PGID == 子进程 PID），
// 以便 Stop() 时通过负 PID 向整个进程组发送信号，杀掉 agent 派生的孙进程。
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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
