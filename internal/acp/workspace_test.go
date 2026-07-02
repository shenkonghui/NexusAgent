package acp

import (
	"os"
	"path/filepath"
	"testing"

	"nexusagent/internal/models"
)

func TestNewExternalWorkspace(t *testing.T) {
	w := NewExternalWorkspace("/some/path")
	if w.Mode != "persistent" {
		t.Errorf("Mode = %q, 期望 persistent", w.Mode)
	}
	if w.Cwd != "/some/path" {
		t.Errorf("Cwd = %q, 期望 /some/path", w.Cwd)
	}
	if w.TempDir != "" {
		t.Errorf("TempDir 应为空，实际 %q", w.TempDir)
	}
}

func TestNewTemporaryWorkspace(t *testing.T) {
	baseDir := t.TempDir()
	w, err := NewTemporaryWorkspace(baseDir, "test-")
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
	if filepath.Dir(w.TempDir) != baseDir {
		t.Errorf("临时目录应在 %s 下，实际 %s", baseDir, filepath.Dir(w.TempDir))
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

func TestNewTemporaryWorkspace_EmptyBaseDir(t *testing.T) {
	// baseDir 为空时回退到 ~/.nextAgent/session，仅验证目录被创建在 home 下并清理。
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("获取主目录失败: %v", err)
	}
	expectedBase := filepath.Join(home, ".nextAgent", "session")
	w, err := NewTemporaryWorkspace("", "test-home-")
	if err != nil {
		t.Fatalf("NewTemporaryWorkspace 错误: %v", err)
	}
	defer w.Cleanup()
	if filepath.Dir(w.TempDir) != expectedBase {
		t.Errorf("临时目录应在 %s 下，实际 %s", expectedBase, filepath.Dir(w.TempDir))
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
		t.Error("persistent 模式 Cleanup 不应删除目录")
	}
}

func TestEnsureWorkspaceDir_RecreatesTemporary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing-temp")
	if err := EnsureWorkspaceDir(models.WorkspaceModeTemporary, dir); err != nil {
		t.Fatalf("EnsureWorkspaceDir 错误: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temporary 目录应被重建: %v", err)
	}
}

func TestEnsureWorkspaceDir_PersistentMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing-persistent")
	err := EnsureWorkspaceDir(models.WorkspaceModePersistent, dir)
	if err == nil {
		t.Fatal("persistent 目录不存在时期望返回错误")
	}
}

func TestWorkspace_CleanupUsesCwdWhenTempDirEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "temp-only-cwd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建目录: %v", err)
	}
	w := &Workspace{Mode: models.WorkspaceModeTemporary, Cwd: dir}
	if err := w.Cleanup(); err != nil {
		t.Fatalf("Cleanup 错误: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("Cleanup 应删除 Cwd 指向的 temporary 目录")
	}
}
