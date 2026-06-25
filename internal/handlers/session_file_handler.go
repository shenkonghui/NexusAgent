package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/models"
)

// 文件读写大小上限（1MB），避免传输超大文件。
const maxFileRWSize = 1 << 20

// sessionFileEntry 文件树节点。
type sessionFileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"` // 相对 session cwd 的路径
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

// SessionFileHandler 处理会话工作目录内的文件浏览与编辑。
// 所有路径均限制在 session.cwd 之内，防止路径穿越。
type SessionFileHandler struct {
	store SessionStore
}

// NewSessionFileHandler 创建 SessionFileHandler。
func NewSessionFileHandler(store SessionStore) *SessionFileHandler {
	return &SessionFileHandler{store: store}
}

// resolveSessionPath 加载会话并将请求的相对路径解析为 cwd 内的绝对路径。
// relPath 为空时返回 cwd 本身。失败时已写入错误响应。
func (h *SessionFileHandler) resolveSessionPath(c *gin.Context) (*models.Session, string, bool) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return nil, "", false
	}
	if sess.Cwd == "" {
		Fail(c, http.StatusBadRequest, "NO_CWD", "该会话没有工作目录")
		return nil, "", false
	}

	relPath := strings.TrimSpace(c.Query("path"))
	absPath, err := safeJoin(sess.Cwd, relPath)
	if err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return nil, "", false
	}
	return sess, absPath, true
}

// loadOwnedSession 加载 :id 对应会话并校验归属（复用 SessionHandler 的逻辑）。
func (h *SessionFileHandler) loadOwnedSession(c *gin.Context) (*models.Session, bool) {
	id, ok := parseSessionID(c)
	if !ok {
		return nil, false
	}
	sess, err := h.store.GetSessionByDBID(id)
	if err != nil || sess == nil {
		Fail(c, http.StatusNotFound, "SESSION_NOT_FOUND", "会话不存在")
		return nil, false
	}
	uid, ok := currentUserID(c)
	if !ok || sess.UserID != uid {
		Fail(c, http.StatusNotFound, "SESSION_NOT_FOUND", "会话不存在")
		return nil, false
	}
	return sess, true
}

// safeJoin 将 relPath 安全地拼接在 root 下，返回绝对路径。
// 解析后的路径必须在 root 之内，否则返回错误。
func safeJoin(root, relPath string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if relPath == "" || relPath == "." {
		return rootAbs, nil
	}
	// 清理相对路径，防止 ../ 穿越到 root 之外
	cleaned := filepath.Clean("/" + relPath) // 前置 / 使其无法逃逸 root
	joined := filepath.Join(rootAbs, cleaned)
	if !isWithin(rootAbs, joined) {
		return "", errPathOutsideCwd
	}
	return joined, nil
}

// isWithin 判断 target 是否在 root 目录内（或等于 root）。
func isWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var errPathOutsideCwd = &pathError{"路径超出会话工作目录范围"}

type pathError struct{ msg string }

func (e *pathError) Error() string { return e.msg }

// ListFiles GET /api/v1/sessions/:id/files?path=...
// 列出 session cwd 下指定子路径的文件和目录（单层，懒加载）。
// path 为空或 "." 时列出 cwd 根目录。目录排前、文件排后。
func (h *SessionFileHandler) ListFiles(c *gin.Context) {
	sess, absPath, ok := h.resolveSessionPath(c)
	if !ok {
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			Fail(c, http.StatusNotFound, "PATH_NOT_FOUND", "路径不存在")
			return
		}
		Fail(c, http.StatusForbidden, "ACCESS_DENIED", "无法访问该路径")
		return
	}
	if !info.IsDir() {
		Fail(c, http.StatusBadRequest, "NOT_A_DIRECTORY", "路径不是目录")
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		Fail(c, http.StatusForbidden, "READ_DENIED", "无法读取目录内容")
		return
	}

	result := make([]sessionFileEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		// 跳过隐藏文件
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(absPath, name)
		rel, _ := filepath.Rel(sess.Cwd, full)
		fe := sessionFileEntry{
			Name:  name,
			Path:  rel,
			IsDir: entry.IsDir(),
		}
		if !entry.IsDir() {
			if fi, err := entry.Info(); err == nil {
				fe.Size = fi.Size()
			}
		}
		result = append(result, fe)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir // 目录排前
		}
		return result[i].Name < result[j].Name
	})

	Success(c, http.StatusOK, gin.H{
		"cwd":     sess.Cwd,
		"path":    strings.TrimSpace(c.Query("path")),
		"entries": result,
	})
}

// ReadFile GET /api/v1/sessions/:id/files/content?path=...
// 读取 session cwd 下的文件内容（文本）。超过 maxFileRWSize 时拒绝。
func (h *SessionFileHandler) ReadFile(c *gin.Context) {
	_, absPath, ok := h.resolveSessionPath(c)
	if !ok {
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			Fail(c, http.StatusNotFound, "FILE_NOT_FOUND", "文件不存在")
			return
		}
		Fail(c, http.StatusForbidden, "ACCESS_DENIED", "无法访问该文件")
		return
	}
	if info.IsDir() {
		Fail(c, http.StatusBadRequest, "IS_A_DIRECTORY", "路径是目录，不是文件")
		return
	}
	if info.Size() > maxFileRWSize {
		Fail(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE",
			"文件过大，最大支持 1MB")
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		Fail(c, http.StatusForbidden, "READ_DENIED", "无法读取文件")
		return
	}

	Success(c, http.StatusOK, gin.H{
		"path":    strings.TrimSpace(c.Query("path")),
		"content": string(data),
		"size":    info.Size(),
	})
}

// WriteFile PUT /api/v1/sessions/:id/files/content
// Body: { "path": "relative/path", "content": "..." }
// 保存文件内容到 session cwd 下。文件不存在则创建，存在则覆盖。
func (h *SessionFileHandler) WriteFile(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	if sess.Cwd == "" {
		Fail(c, http.StatusBadRequest, "NO_CWD", "该会话没有工作目录")
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求体格式错误")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		Fail(c, http.StatusBadRequest, "PATH_REQUIRED", "path 不能为空")
		return
	}
	if len(req.Content) > maxFileRWSize {
		Fail(c, http.StatusRequestEntityTooLarge, "CONTENT_TOO_LARGE",
			"内容过大，最大支持 1MB")
		return
	}

	absPath, err := safeJoin(sess.Cwd, req.Path)
	if err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}

	// 确保父目录存在
	parent := filepath.Dir(absPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		Fail(c, http.StatusInternalServerError, "MKDIR_FAILED", "无法创建父目录")
		return
	}

	if err := os.WriteFile(absPath, []byte(req.Content), 0o644); err != nil {
		Fail(c, http.StatusInternalServerError, "WRITE_FAILED", "写入文件失败")
		return
	}

	Success(c, http.StatusOK, gin.H{
		"path": req.Path,
		"size": len(req.Content),
	})
}
