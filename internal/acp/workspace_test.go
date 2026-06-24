package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewExternalWorkspace(t *testing.T) {
	w := NewExternalWorkspace("/some/path")
	if w.Mode != "external" {
		t.Errorf("Mode = %q, 期望 external", w.Mode)
	}
	if w.Cwd != "/some/path" {
		t.Errorf("Cwd = %q, 期望 /some/path", w.Cwd)
	}
	if w.TempDir != "" {
		t.Errorf("TempDir 应为空，实际 %q", w.TempDir)
	}
}

func TestNewTemporaryWorkspace(t *testing.T) {
	w, err := NewTemporaryWorkspace("test-")
	if err != nil {
		t.Fatalf("NewTemporaryWorkspace 错误: %v", err)
	}
	if w.Mode != "temporary" {
		t.Errorf("Mode = %q, 期望 temporary", w.Mode)
	}
	if w.Cwd == "" {
		t.Error("Cwd 不应为空")
	}
	if w.TempDir == "" {
		t.Error("TempDir 不应为空")
	}
	if _, err := os.Stat(w.TempDir); os.IsNotExist(err) {
		t.Errorf("临时目录不存在: %s", w.TempDir)
	}
	if filepath.Base(w.TempDir)[:5] != "test-" {
		t.Errorf("目录名前缀不匹配: %s", filepath.Base(w.TempDir))
	}
	if err := w.Cleanup(); err != nil {
		t.Fatalf("Cleanup 错误: %v", err)
	}
	if _, err := os.Stat(w.TempDir); !os.IsNotExist(err) {
		t.Errorf("Cleanup 后目录应被删除: %s", w.TempDir)
	}
}

func TestExternalWorkspace_Cleanup_NoDelete(t *testing.T) {
	dir, _ := os.MkdirTemp("", "ext-test-")
	defer os.RemoveAll(dir)

	w := NewExternalWorkspace(dir)
	if err := w.Cleanup(); err != nil {
		t.Fatalf("Cleanup 错误: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("external 模式 Cleanup 不应删除目录")
	}
}
