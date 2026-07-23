package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"opennexus/internal/middleware"
	"opennexus/internal/models"
	"opennexus/internal/services"
)

// stubWorkspaceStore 返回固定工作区，用于编排 handler 测试解析 cwd/归属。
type stubWorkspaceStore struct {
	ws *models.Workspace
}

func (s *stubWorkspaceStore) FindWorkspaceByID(_ uint) (*models.Workspace, error) {
	return s.ws, nil
}

func setupOrchRouter(t *testing.T, userID uint, cwd string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := services.NewOrchestratorService(nil)
	ws := &models.Workspace{UserID: userID, Cwd: cwd}
	ws.ID = 1
	h := NewOrchestrationHandler(svc, &stubWorkspaceStore{ws: ws})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey(), userID)
		c.Next()
	})
	g := r.Group("/api/v1")
	g.PUT("/orchestration/parent-session", h.SetParentSession)
	g.GET("/orchestration", h.Get)
	return r
}

// TestSetParentSessionHandler 验证 PUT /orchestration/parent-session 将 session_id 写入 tasks.json。
func TestSetParentSessionHandler(t *testing.T) {
	cwd := t.TempDir()
	r := setupOrchRouter(t, 9, cwd)

	w := doJSON(t, r, "PUT", "/api/v1/orchestration/parent-session?workspace_id=1", gin.H{"session_id": 55})
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", w.Code, w.Body.String())
	}

	// 通过 GET 读取 def，确认 parent_session_id 已持久化。
	w = doJSON(t, r, "GET", "/api/v1/orchestration?workspace_id=1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			ParentSessionID *uint `json:"parent_session_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ParentSessionID == nil || *resp.Data.ParentSessionID != 55 {
		t.Fatalf("parent_session_id 未持久化: %v", resp.Data.ParentSessionID)
	}
}
