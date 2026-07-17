package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/database"
	"nexusagent/internal/middleware"
	"nexusagent/internal/repository"
)

func setupNoteMCPRouter(t *testing.T, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM note_settings")
	db.Exec("DELETE FROM notes")
	h := NewNoteHandler(repository.NewNoteRepository(db), repository.NewNoteSettingsRepository(db), nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey(), userID)
		c.Next()
	})
	notes := r.Group("/api/v1/notes")
	notes.GET("/settings", h.GetSettings)
	notes.POST("/settings/mcp-token", h.GenerateMCPToken)
	return r
}

func TestGenerateMCPToken_Once(t *testing.T) {
	r := setupNoteMCPRouter(t, 42)

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/api/v1/notes/settings/mcp-token", nil))
	if w1.Code != http.StatusOK {
		t.Fatalf("首次生成 status=%d body=%s", w1.Code, w1.Body.String())
	}
	var resp1 struct {
		Data struct {
			McpToken string `json:"mcp_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil || resp1.Data.McpToken == "" {
		t.Fatalf("解析 token 失败: %v body=%s", err, w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/api/v1/notes/settings/mcp-token", nil))
	if w2.Code != http.StatusConflict {
		t.Fatalf("再次生成 status=%d, 期望 409", w2.Code)
	}

	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/api/v1/notes/settings", nil))
	if w3.Code != http.StatusOK {
		t.Fatalf("GetSettings status=%d", w3.Code)
	}
	var resp3 struct {
		Data struct {
			McpToken string `json:"mcp_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w3.Body.Bytes(), &resp3)
	if resp3.Data.McpToken != resp1.Data.McpToken {
		t.Fatalf("settings.mcp_token=%q, 期望 %q", resp3.Data.McpToken, resp1.Data.McpToken)
	}
}
