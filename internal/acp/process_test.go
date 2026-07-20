package acp

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAgentCommand_PlainName(t *testing.T) {
	resolved, err := resolveAgentCommand("echo")
	if err != nil {
		t.Fatalf("resolveAgentCommand(echo): %v", err)
	}
	if filepath.Base(resolved) != "echo" {
		t.Errorf("resolved = %q, 期望 basename 为 echo", resolved)
	}
}

func TestResolveAgentCommand_RelativePathUsesBasename(t *testing.T) {
	// registry binary 占位命令形如 ./dist-package/cursor-agent；
	// 文件不存在时应取最后一段在 PATH 中查找。
	want, err := exec.LookPath("echo")
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveAgentCommand("./dist-package/echo")
	if err != nil {
		t.Fatalf("resolveAgentCommand(./dist-package/echo): %v", err)
	}
	if resolved != want {
		t.Errorf("resolved = %q, 期望 %q", resolved, want)
	}
}

func TestResolveAgentCommand_MissingReportsBasename(t *testing.T) {
	_, err := resolveAgentCommand("./dist-package/opennexus-not-exist-xyz")
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "opennexus-not-exist-xyz") {
		t.Errorf("错误应包含 basename，实际: %v", err)
	}
}
