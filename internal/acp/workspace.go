package acp

import (
	"fmt"
	"os"
	"path/filepath"

	"nexusagent/internal/models"
)

// Workspace 管理会话的工作目录。
type Workspace struct {
	Mode    string
	Cwd     string
	TempDir string
}

// NewExternalWorkspace 创建一个使用外部指定 cwd 的工作区。
func NewExternalWorkspace(cwd string) *Workspace {
	return &Workspace{
		Mode: models.WorkspaceModePersistent,
		Cwd:  cwd,
	}
}

// NewTemporaryWorkspace 创建一个临时目录作为工作区。
// baseDir 指定会话工作区的存放根目录（如 ~/.nextAgent/session），
// 会话目录在该根目录下以 prefix 为前缀创建，由程序在删除会话时清理，
// 而不依赖操作系统对系统临时目录的清理。
// baseDir 为空时回退到 ~/.nextAgent/session。
func NewTemporaryWorkspace(baseDir, prefix string) (*Workspace, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("获取用户主目录: %w", err)
		}
		baseDir = filepath.Join(home, ".nextAgent", "session")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建会话根目录 %s: %w", baseDir, err)
	}
	dir, err := os.MkdirTemp(baseDir, prefix+"*")
	if err != nil {
		return nil, fmt.Errorf("创建临时工作区: %w", err)
	}
	absDir, _ := filepath.Abs(dir)
	return &Workspace{
		Mode:    models.WorkspaceModeTemporary,
		Cwd:     absDir,
		TempDir: absDir,
	}, nil
}

// Cleanup 清理工作区。temporary 模式删除临时目录，persistent 模式不做任何操作。
// 仅在删除工作区时调用，删除单个会话时不应清理共享工作区目录。
func (w *Workspace) Cleanup() error {
	if w.Mode != models.WorkspaceModeTemporary {
		return nil
	}
	dir := w.TempDir
	if dir == "" {
		dir = w.Cwd
	}
	if dir == "" {
		return nil
	}
	return os.RemoveAll(dir)
}

// EnsureWorkspaceDir 确保工作区目录存在。
// temporary 模式下若目录被误删则自动重建；persistent 模式目录不存在则返回错误。
func EnsureWorkspaceDir(mode, cwd string) error {
	if cwd == "" {
		return fmt.Errorf("工作目录路径为空")
	}
	if dirExists(cwd) {
		return nil
	}
	if mode == models.WorkspaceModeTemporary {
		return os.MkdirAll(cwd, 0o755)
	}
	return fmt.Errorf("工作目录不存在: %s", cwd)
}

// dirExists 判断目录是否存在。
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
