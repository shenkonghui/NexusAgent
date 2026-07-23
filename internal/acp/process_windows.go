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

// KillProcessGroup 在 Windows 上暂不支持，返回 nil。
// TODO: 如需 Windows 支持，改用 taskkill /T /F 或 Job Object 实现。
func KillProcessGroup(pid int) error { return nil }

// KillOrphanACPProcesses 在 Windows 上暂不支持，返回 0, nil。
func KillOrphanACPProcesses() (int, error) { return 0, nil }

// ProcessAlive 在 Windows 上简化为始终返回 true（精确判断需 OpenProcess，暂未实现）。
func ProcessAlive(pid int) bool { return pid > 0 }
