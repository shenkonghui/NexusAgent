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
		Mode: models.WorkspaceModeExternal,
		Cwd:  cwd,
	}
}

// NewTemporaryWorkspace 创建一个临时目录作为工作区。
func NewTemporaryWorkspace(prefix string) (*Workspace, error) {
	dir, err := os.MkdirTemp("", prefix+"*")
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

// Cleanup 清理工作区。temporary 模式删除临时目录，external 模式不做任何操作。
func (w *Workspace) Cleanup() error {
	if w.Mode == models.WorkspaceModeTemporary && w.TempDir != "" {
		return os.RemoveAll(w.TempDir)
	}
	return nil
}
