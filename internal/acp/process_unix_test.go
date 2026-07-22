//go:build !windows

package acp

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestStop_TerminatesGrandchildren 验证 Stop() 通过进程组终止 agent 派生的孙进程。
// 子进程（sh）派生一个长 sleep 孙进程，模拟 npm exec / cursor-agent 派生 helper 的场景。
// Stop 后用 syscall.Kill(-pgid, 0) 探测进程组是否已被清空。
func TestStop_TerminatesGrandchildren(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	backend := &MockBackend{
		name:    "stub-agent",
		command: "sh",
		args:    []string{"-c", "sleep 30"},
	}
	proc, err := NewProcess(backend, "")
	if err != nil {
		t.Fatalf("NewProcess: %v", err)
	}
	pgid := proc.cmd.Process.Pid

	// 给 shell 一点时间派生孙进程。
	time.Sleep(200 * time.Millisecond)

	if err := proc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// SIGKILL 后进程组应已不存在。Kill(-pgid, 0) 探测：ESRCH 表示已清空。
	// 给 OS 一点时间回收，避免误报。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, 0); err != nil {
			return // ESRCH 或其他错误都意味着进程组已不存在
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Stop 后进程组 %d 仍存在，孙进程未被终止", pgid)
}
