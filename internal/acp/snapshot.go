package acp

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 快照相关常量。
const (
	maxSnapshotFileSize = 512 * 1024 // 512KB：超过此大小的文件不纳入快照
	snapshotBinaryCheck = 8 * 1024   // 检测二进制时读取的前 N 字节
)

// snapshotIgnoreDirs 是快照遍历时跳过的目录名（与 filesystem_handler 一致）。
var snapshotIgnoreDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	".next": true, "__pycache__": true, ".venv": true, "vendor": true,
	".nextAgent": true, ".claude": true,
}

// takeSnapshot 递归遍历 cwd，返回 relPath→content 映射。
// 跳过隐藏文件/目录、忽略目录、二进制文件和超大文件。
// cwd 为空或不存在时返回空 map。
func takeSnapshot(cwd string) map[string]string {
	if cwd == "" {
		return map[string]string{}
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return map[string]string{}
	}

	snapshot := make(map[string]string)
	_ = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过无法访问的路径
		}
		name := d.Name()

		// 跳过隐藏文件/目录
		if strings.HasPrefix(name, ".") && path != cwd {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if snapshotIgnoreDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		// 跳过超大文件
		if fi, e := d.Info(); e == nil && fi.Size() > maxSnapshotFileSize {
			return nil
		}

		// 仅快照文本文件
		if !isTextFile(path) {
			return nil
		}

		data, e := os.ReadFile(path)
		if e != nil {
			return nil
		}
		rel, e := filepath.Rel(cwd, path)
		if e != nil {
			return nil
		}
		snapshot[rel] = string(data)
		return nil
	})
	return snapshot
}

// isTextFile 通过检测前 snapshotBinaryCheck 字节中是否含 NULL 字节来判断是否为文本文件。
func isTextFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, snapshotBinaryCheck)
	n, _ := f.Read(buf)
	if n == 0 {
		return true // 空文件视为文本
	}
	return !bytes.Contains(buf[:n], []byte{0})
}

// compareSnapshots 对比前后快照，返回变更文件的 FileWriteNotify 列表。
// 仅检测新增和修改的文件（删除暂不纳入展示）。
// 返回的 Path 为相对 cwd 的路径。
func compareSnapshots(before, after map[string]string) []FileWriteNotify {
	var diffs []FileWriteNotify
	for rel, newText := range after {
		oldText, existed := before[rel]
		if !existed {
			// 新文件
			diffs = append(diffs, FileWriteNotify{
				Path:    rel,
				NewText: newText,
				IsNew:   true,
			})
		} else if oldText != newText {
			// 修改的文件
			diffs = append(diffs, FileWriteNotify{
				Path:    rel,
				OldText: oldText,
				NewText: newText,
				IsNew:   false,
			})
		}
	}
	return diffs
}
