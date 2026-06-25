package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/middleware"
	"nexusagent/internal/models"
)

// newSessionFileTestRouter 构造带「注入 userID」中间件的测试路由，注册文件相关路由。
func newSessionFileTestRouter(store SessionStore, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey(), userID)
		c.Next()
	})
	h := NewSessionFileHandler(store)
	v1 := r.Group("/api/v1")
	v1.GET("/sessions/:id/files", h.ListFiles)
	v1.GET("/sessions/:id/files/content", h.ReadFile)
	v1.PUT("/sessions/:id/files/content", h.WriteFile)
	return r
}

// setupFileTestStore 创建一个 fakeSessionStore，其中 session 1 的 cwd 指向临时目录。
func setupFileTestStore(t *testing.T) (*fakeSessionStore, string) {
	t.Helper()
	cwd := t.TempDir()
	// 创建测试文件结构
	os.WriteFile(filepath.Join(cwd, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(cwd, "readme.md"), []byte("# Test\n"), 0o644)
	os.MkdirAll(filepath.Join(cwd, "sub"), 0o755)
	os.WriteFile(filepath.Join(cwd, "sub", "util.go"), []byte("package sub\n"), 0o644)

	store := newFakeSessionStore()
	store.nextID = 1
	store.sessions[1] = &models.Session{
		ID:        1,
		SessionID: "acp-1",
		Cwd:       cwd,
		Status:    models.SessionStatusActive,
		UserID:    100,
	}
	return store, cwd
}

func TestSessionFileHandler_ListFiles(t *testing.T) {
	store, _ := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	w := doJSON(t, r, "GET", "/api/v1/sessions/1/files", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Entries []sessionFileEntry `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	names := make(map[string]bool) // name -> isDir
	exists := make(map[string]bool)
	for _, e := range resp.Data.Entries {
		names[e.Name] = e.IsDir
		exists[e.Name] = true
	}
	// 应包含 main.go, readme.md, sub（目录排前）
	if !exists["main.go"] {
		t.Error("缺少 main.go")
	}
	if !exists["readme.md"] {
		t.Error("缺少 readme.md")
	}
	if isDir, ok := names["sub"]; !ok || !isDir {
		t.Error("缺少 sub 目录或不是目录")
	}
	// 隐藏文件应被跳过
	if exists[".hidden"] {
		t.Error("不应包含隐藏文件")
	}
}

func TestSessionFileHandler_ListFiles_SubDir(t *testing.T) {
	store, _ := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	w := doJSON(t, r, "GET", "/api/v1/sessions/1/files?path=sub", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Entries []sessionFileEntry `json:"entries"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data.Entries) != 1 || resp.Data.Entries[0].Name != "util.go" {
		t.Errorf("子目录文件列表不匹配: %+v", resp.Data.Entries)
	}
}

func TestSessionFileHandler_ListFiles_NotOwner(t *testing.T) {
	store, _ := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 999) // 不同用户

	w := doJSON(t, r, "GET", "/api/v1/sessions/1/files", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("期望 404（非所有者），实际 %d", w.Code)
	}
}

func TestSessionFileHandler_ReadFile(t *testing.T) {
	store, _ := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	w := doJSON(t, r, "GET", "/api/v1/sessions/1/files/content?path=main.go", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Content != "package main\n" {
		t.Errorf("内容 = %q, 期望 package main\\n", resp.Data.Content)
	}
}

func TestSessionFileHandler_ReadFile_NotFound(t *testing.T) {
	store, _ := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	w := doJSON(t, r, "GET", "/api/v1/sessions/1/files/content?path=nonexistent.go", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("期望 404，实际 %d", w.Code)
	}
}

func TestSessionFileHandler_ReadFile_PathTraversal(t *testing.T) {
	store, _ := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	// ../ 被安全地限制在 cwd 内（清理后变为 cwd/etc/passwd），不会逃逸
	w := doJSON(t, r, "GET", "/api/v1/sessions/1/files/content?path=../../etc/passwd", nil)
	// 路径被限制在 cwd 内，文件不存在所以返回 404，而非 400
	if w.Code != http.StatusNotFound {
		t.Errorf("期望 404（路径被限制在 cwd 内，文件不存在），实际 %d, body=%s", w.Code, w.Body.String())
	}
}

func TestSessionFileHandler_WriteFile_Create(t *testing.T) {
	store, cwd := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	body := gin.H{"path": "new.go", "content": "package main\n\nfunc main() {}\n"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/v1/sessions/1/files/content", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(cwd, "new.go"))
	if err != nil {
		t.Fatalf("文件未创建: %v", err)
	}
	if string(data) != "package main\n\nfunc main() {}\n" {
		t.Errorf("文件内容不匹配: %q", string(data))
	}
}

func TestSessionFileHandler_WriteFile_Overwrite(t *testing.T) {
	store, cwd := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	body := gin.H{"path": "main.go", "content": "// overwritten\n"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/v1/sessions/1/files/content", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", rec.Code, rec.Body.String())
	}
	data, _ := os.ReadFile(filepath.Join(cwd, "main.go"))
	if string(data) != "// overwritten\n" {
		t.Errorf("文件内容不匹配: %q", string(data))
	}
}

func TestSessionFileHandler_WriteFile_PathTraversal(t *testing.T) {
	store, cwd := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	// ../ 被安全地限制在 cwd 内（清理后变为 cwd/etc/evil），不会逃逸到外部
	body := gin.H{"path": "../../../etc/evil", "content": "bad"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/v1/sessions/1/files/content", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("期望 200（路径被安全限制在 cwd 内），实际 %d, body=%s", rec.Code, rec.Body.String())
	}
	// 验证文件写在了 cwd/etc/evil 而非真正的 /etc/evil
	if _, err := os.Stat(filepath.Join(cwd, "etc", "evil")); err != nil {
		t.Errorf("文件应写入 cwd/etc/evil，但不存在: %v", err)
	}
	// 验证没有写到真正的 /etc/evil
	if _, err := os.Stat("/etc/evil"); err == nil {
		t.Error("文件不应写到 /etc/evil")
	}
}

func TestSessionFileHandler_WriteFile_NestedDir(t *testing.T) {
	store, cwd := setupFileTestStore(t)
	r := newSessionFileTestRouter(store, 100)

	body := gin.H{"path": "deep/nested/dir/file.go", "content": "test"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/v1/sessions/1/files/content", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, "deep", "nested", "dir", "file.go")); err != nil {
		t.Errorf("嵌套目录文件未创建: %v", err)
	}
}

func TestSafeJoin(t *testing.T) {
	root := "/tmp/testroot"
	tests := []struct {
		rel  string
		want string
	}{
		{"", "/tmp/testroot"},
		{".", "/tmp/testroot"},
		{"foo.go", "/tmp/testroot/foo.go"},
		{"sub/bar.go", "/tmp/testroot/sub/bar.go"},
		{"../escape", "/tmp/testroot/escape"}, // 清理后仍在 root 内
		{"../../etc/passwd", "/tmp/testroot/etc/passwd"},
	}
	for _, tt := range tests {
		got, err := safeJoin(root, tt.rel)
		if err != nil {
			t.Errorf("safeJoin(%q) 错误: %v", tt.rel, err)
			continue
		}
		if got != tt.want {
			t.Errorf("safeJoin(%q) = %q, 期望 %q", tt.rel, got, tt.want)
		}
	}
}

func TestSafeJoin_AbsolutePathContained(t *testing.T) {
	// 绝对路径作为 relPath 时，filepath.Join 会将其当作相对路径处理，
	// 结果仍在 root 内，不会逃逸。
	got, err := safeJoin("/tmp/testroot", "/etc/passwd")
	if err != nil {
		t.Fatalf("safeJoin 错误: %v", err)
	}
	if got != "/tmp/testroot/etc/passwd" {
		t.Errorf("safeJoin(/etc/passwd) = %q, 期望 /tmp/testroot/etc/passwd", got)
	}
}
