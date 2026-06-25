package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// dirEntry 是目录浏览 API 返回的单个目录项。
type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// fileEntry 是文件列表 API 返回的单个文件/目录项。
type fileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// FileSystemHandler 提供本地文件系统目录浏览能力（用于前端目录选择器）。
type FileSystemHandler struct{}

// NewFileSystemHandler 创建 FileSystemHandler。
func NewFileSystemHandler() *FileSystemHandler {
	return &FileSystemHandler{}
}

// resolveDirPath 解析并校验请求路径，返回绝对路径。失败时已写入错误响应。
func resolveDirPath(c *gin.Context) (string, bool) {
	reqPath := strings.TrimSpace(c.Query("path"))

	if reqPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			Fail(c, http.StatusInternalServerError, "HOME_UNAVAILABLE", "无法获取用户主目录")
			return "", false
		}
		reqPath = home
	}

	absPath, err := filepath.Abs(reqPath)
	if err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_PATH", "路径无效")
		return "", false
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			Fail(c, http.StatusNotFound, "PATH_NOT_FOUND", "目录不存在")
			return "", false
		}
		Fail(c, http.StatusForbidden, "PATH_ACCESS_DENIED", "无法访问该目录")
		return "", false
	}
	if !info.IsDir() {
		Fail(c, http.StatusBadRequest, "NOT_A_DIRECTORY", "路径不是目录")
		return "", false
	}
	return absPath, true
}

// ListDirs GET /api/v1/filesystem/dirs?path=...
// 返回指定目录下的子目录列表（仅目录，不含文件）。
// path 为空时默认返回用户主目录及其子目录。
func (h *FileSystemHandler) ListDirs(c *gin.Context) {
	absPath, ok := resolveDirPath(c)
	if !ok {
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		Fail(c, http.StatusForbidden, "READ_DENIED", "无法读取目录内容")
		return
	}

	dirs := make([]dirEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// 跳过隐藏目录（以 . 开头）
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		dirs = append(dirs, dirEntry{
			Name: name,
			Path: filepath.Join(absPath, name),
		})
	}

	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Name < dirs[j].Name
	})

	Success(c, http.StatusOK, gin.H{
		"current_path": absPath,
		"parent_path":  parentPath(absPath),
		"dirs":         dirs,
	})
}

// ListFiles GET /api/v1/filesystem/list?path=...&query=...
// 返回指定目录下的文件和目录列表，支持 query 过滤文件名。
// 用于 @ 文件引用的自动补全。目录排前、文件排后，跳过隐藏文件和常见忽略目录。
func (h *FileSystemHandler) ListFiles(c *gin.Context) {
	absPath, ok := resolveDirPath(c)
	if !ok {
		return
	}

	query := strings.ToLower(strings.TrimSpace(c.Query("query")))

	entries, err := os.ReadDir(absPath)
	if err != nil {
		Fail(c, http.StatusForbidden, "READ_DENIED", "无法读取目录内容")
		return
	}

	// 常见忽略目录名
	ignoreDirs := map[string]bool{
		"node_modules": true, ".git": true, "dist": true, "build": true,
		".next": true, "__pycache__": true, ".venv": true, "vendor": true,
	}

	var dirs, files []fileEntry
	for _, entry := range entries {
		name := entry.Name()
		// 跳过隐藏文件
		if strings.HasPrefix(name, ".") {
			continue
		}
		// 跳过忽略目录
		if entry.IsDir() && ignoreDirs[name] {
			continue
		}
		// query 过滤
		if query != "" && !strings.Contains(strings.ToLower(name), query) {
			continue
		}
		fe := fileEntry{
			Name:  name,
			Path:  filepath.Join(absPath, name),
			IsDir: entry.IsDir(),
		}
		if entry.IsDir() {
			dirs = append(dirs, fe)
		} else {
			files = append(files, fe)
		}
	}

	// 限制返回数量，避免超大目录
	const maxItems = 100
	result := make([]fileEntry, 0, len(dirs)+len(files))
	result = append(result, dirs...)
	result = append(result, files...)
	if len(result) > maxItems {
		result = result[:maxItems]
	}

	Success(c, http.StatusOK, gin.H{
		"current_path": absPath,
		"parent_path":  parentPath(absPath),
		"entries":      result,
	})
}

// parentPath 返回父目录路径，根目录时返回自身。
func parentPath(p string) string {
	parent := filepath.Dir(p)
	if parent == p {
		return ""
	}
	return parent
}
