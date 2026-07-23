package acp

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreesDir 是存放各任务 worktree 的目录名，位于仓库根下。
const WorktreesDir = ".worktrees"

// ErrNotGitRepo 表示给定路径不是一个 git 仓库。
var ErrNotGitRepo = errors.New("路径不是 git 仓库")

// IsGitRepo 检测 path 是否为 git 仓库（含 .git 目录或文件）。
func IsGitRepo(path string) bool {
	git := filepath.Join(path, ".git")
	if _, err := os.Stat(git); err == nil {
		return true
	}
	// git submodule / worktree 中 .git 可能是文件
	if info, err := os.Stat(git); err == nil && !info.IsDir() {
		return true
	}
	return false
}

// GitRoot 返回 path 所在 git 仓库的根目录；非仓库返回 ErrNotGitRepo。
// 优先使用 git rev-parse，能正确处理 worktree（公共 .git）的情况。
func GitRoot(path string) (string, error) {
	if !IsGitRepo(path) {
		// 尝试向上查找（path 可能本身是 worktree）
		cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
		if out, err := cmd.Output(); err == nil {
			return strings.TrimSpace(string(out)), nil
		}
		return "", ErrNotGitRepo
	}
	// 仍以 rev-parse 为准，能正确处理 worktree 指向公共 .git 的情况
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	if out, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	return path, nil
}

// runGit 在 repoPath 下执行 git 命令，返回合并后的 stderr 错误。
func runGit(repoPath string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

// CreateWorktree 在 repoPath 仓库中创建一个新分支 branch 并检出到 destPath。
// 若 destPath 已存在则返回错误。base 为空时从当前 HEAD 创建。
func CreateWorktree(repoPath, branch, destPath, base string) error {
	root, err := GitRoot(repoPath)
	if err != nil {
		return err
	}
	if destPath == "" {
		return fmt.Errorf("destPath 不能为空")
	}
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("worktree 目录已存在: %s", destPath)
	}

	args := []string{"worktree", "add", "-b", branch, destPath}
	if base != "" {
		args = append(args, base)
	}
	if err := runGit(root, args...); err != nil {
		return err
	}
	return nil
}

// RemoveWorktree 移除 destPath 对应的 worktree，并删除其分支 branch（force）。
// 容错：worktree 已不存在时视为成功。
func RemoveWorktree(repoPath, destPath, branch string) error {
	root, err := GitRoot(repoPath)
	if err != nil {
		return err
	}
	// 移除 worktree（--force 以应对有未提交改动的情况）
	if err := runGit(root, "worktree", "remove", "--force", destPath); err != nil {
		// 若目录已被手动删除，prune 后忽略错误
		_ = runGit(root, "worktree", "prune")
		if _, statErr := os.Stat(destPath); statErr == nil {
			return err
		}
	}
	// 删除分支（可能不存在或为当前分支，忽略错误）
	if branch != "" {
		_ = runGit(root, "branch", "-D", branch)
	}
	return nil
}

// WorktreePath 返回仓库根下 .worktrees/<name> 的绝对路径。
func WorktreePath(repoRoot, name string) string {
	return filepath.Join(repoRoot, WorktreesDir, name)
}

// EnsureWorktreesDir 确保仓库根下的 .worktrees 目录存在。
func EnsureWorktreesDir(repoRoot string) error {
	return os.MkdirAll(filepath.Join(repoRoot, WorktreesDir), 0o755)
}

// hasCommit 报告 path 所在仓库是否已有可用的 HEAD 提交。
// git worktree add 需要基于某个提交创建，空仓库（无提交）会失败，故初始化时需先建初始提交。
func hasCommit(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--verify", "HEAD")
	return cmd.Run() == nil
}

// ensureInitialCommit 若仓库尚无提交，则将现有文件全部暂存并创建初始提交。
// 附带最小 author 信息，避免宿主机未配置 user.name/user.email 时提交失败。
func ensureInitialCommit(path string) error {
	if hasCommit(path) {
		return nil
	}
	// 暂存现有文件（若目录本就有内容），使各任务 worktree 能包含项目文件；无内容时允许空提交。
	_ = runGit(path, "add", "-A")
	return runGit(path,
		"-c", "user.email=nexus@local",
		"-c", "user.name=NexusAgent",
		"commit", "--allow-empty", "-m", "chore: initialize repository for orchestration",
	)
}

// GitInit 在 path 下初始化 git 仓库并确保存在初始提交（供 worktree 创建）。
// 若 path 已是 git 仓库则仅补齐初始提交（幂等）。
func GitInit(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path 不能为空")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("创建目录: %w", err)
	}
	if !IsGitRepo(path) {
		if err := runGit(path, "init"); err != nil {
			return err
		}
	}
	return ensureInitialCommit(path)
}
