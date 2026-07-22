//go:build windows

package acp

import (
	"os"
	"os/exec"
	"time"
)

// setProcessGroup 在 Windows 上无对应概念，保持空操作。
// Windows 上 agent 通常通过 Job Object 管理子进程，此处暂不处理。
func setProcessGroup(_ *exec.Cmd) {}

// terminateProcessGroup Windows 回退：直接 Kill 直系子进程并回收。
// 孙进程清理留待后续在 Windows 平台上以 Job Object 方案补齐。
// 调用方不应再调用 proc.Wait()——本方法已回收直系子进程。
func terminateProcessGroup(proc *os.Process, _ time.Duration) error {
	if err := proc.Kill(); err != nil {
		return err
	}
	_, _ = proc.Wait()
	return nil
}

// probeProcessState 在 Windows 上仅区分存活/退出，无「被信号停止」概念。
func probeProcessState(pid int) procState {
	// Windows 上通过 OpenProcess + GetExitCodeProcess 才能精确判断，
	// 此处简化处理：假定进程仍在运行，诊断依赖 stderr 尾部。
	return procStateRunning
}
