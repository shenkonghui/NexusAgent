package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
	"nexusagent/internal/middleware"
	"nexusagent/internal/models"
)

// fakeSessionStore 是内存版 SessionStore，用于隔离真实 ACP 子进程。
type fakeSessionStore struct {
	sessions     map[uint]*models.Session
	messages     map[string][]models.Message
	createErr    error
	listErr      error
	closeErr     error
	cancelErr    error
	resumeErr    error
	resumeResult *models.Session
	listMsgErr   error
	promptCh     chan models.Message
	promptErr    error
	nextID       uint
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{
		sessions: make(map[uint]*models.Session),
		messages: make(map[string][]models.Message),
		promptCh: make(chan models.Message),
	}
}

func (f *fakeSessionStore) CreateSession(_ context.Context, agentType, cwd string, userID uint) (*models.Session, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.nextID++
	s := &models.Session{
		ID:        f.nextID,
		SessionID: "acp-" + strconv.Itoa(int(f.nextID)),
		AgentType: agentType,
		Cwd:       cwd,
		Status:    models.SessionStatusActive,
		UserID:    userID,
	}
	f.sessions[s.ID] = s
	return s, nil
}

func (f *fakeSessionStore) ListSessions(userID uint) ([]models.Session, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []models.Session
	for _, s := range f.sessions {
		if s.UserID == userID {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (f *fakeSessionStore) GetSessionByDBID(id uint) (*models.Session, error) {
	s, ok := f.sessions[id]
	if !ok {
		return nil, errors.New("会话不存在")
	}
	return s, nil
}

func (f *fakeSessionStore) CloseSession(_ context.Context, _ string) error  { return f.closeErr }
func (f *fakeSessionStore) CancelSession(_ context.Context, _ string) error { return f.cancelErr }

func (f *fakeSessionStore) ResumeSession(_ context.Context, sessionID string) (*models.Session, error) {
	if f.resumeErr != nil {
		return nil, f.resumeErr
	}
	if f.resumeResult != nil {
		return f.resumeResult, nil
	}
	return &models.Session{SessionID: sessionID, Status: models.SessionStatusActive}, nil
}

func (f *fakeSessionStore) ListMessages(sessionID string) ([]models.Message, error) {
	if f.listMsgErr != nil {
		return nil, f.listMsgErr
	}
	return f.messages[sessionID], nil
}

func (f *fakeSessionStore) Prompt(_ context.Context, _, _ string) (<-chan models.Message, error) {
	if f.promptErr != nil {
		return nil, f.promptErr
	}
	return f.promptCh, nil
}

// newSessionTestRouter 构造带「注入 userID」中间件的测试路由。
func newSessionTestRouter(store SessionStore, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey(), userID)
		c.Next()
	})
	h := NewSessionHandler(store)
	v1 := r.Group("/api/v1")
	v1.POST("/sessions", h.Create)
	v1.GET("/sessions", h.List)
	v1.GET("/sessions/:id", h.Get)
	v1.DELETE("/sessions/:id", h.Close)
	v1.POST("/sessions/:id/prompt", h.Prompt)
	v1.POST("/sessions/:id/cancel", h.Cancel)
	v1.POST("/sessions/:id/resume", h.Resume)
	v1.GET("/sessions/:id/messages", h.Messages)
	return r
}

func TestSessionHandler_Create_Success(t *testing.T) {
	r := newSessionTestRouter(newFakeSessionStore(), 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions", gin.H{"agent_type": "claude-code"})
	if w.Code != http.StatusCreated {
		t.Fatalf("状态码 = %d, 期望 201, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data models.Session `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Data.ID == 0 || resp.Data.AgentType != "claude-code" || resp.Data.UserID != 100 {
		t.Errorf("会话字段不正确: %+v", resp.Data)
	}
}

func TestSessionHandler_Create_UnknownAgent(t *testing.T) {
	store := newFakeSessionStore()
	store.createErr = agent.ErrAgentNotFound
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions", gin.H{"agent_type": "ghost"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "AGENT_NOT_FOUND" {
		t.Errorf("error code = %q, 期望 AGENT_NOT_FOUND", resp.Error.Code)
	}
}

func TestSessionHandler_Create_NoBody(t *testing.T) {
	r := newSessionTestRouter(newFakeSessionStore(), 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

func TestSessionHandler_List_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	store.sessions[2] = &models.Session{ID: 2, SessionID: "acp-2", UserID: 200, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "GET", "/api/v1/sessions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Sessions []models.Session `json:"sessions"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data.Sessions) != 1 {
		t.Fatalf("会话数量 = %d, 期望 1（仅当前用户）", len(resp.Data.Sessions))
	}
	if resp.Data.Sessions[0].ID != 1 {
		t.Errorf("返回会话 ID = %d, 期望 1", resp.Data.Sessions[0].ID)
	}
}

func TestSessionHandler_Get_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "GET", "/api/v1/sessions/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
}

func TestSessionHandler_Get_NotFound(t *testing.T) {
	r := newSessionTestRouter(newFakeSessionStore(), 100)
	w := doJSON(t, r, "GET", "/api/v1/sessions/999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d, 期望 404", w.Code)
	}
}

func TestSessionHandler_Get_NotOwner(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 200, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "GET", "/api/v1/sessions/1", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d, 期望 404（不属于当前用户）", w.Code)
	}
}

func TestSessionHandler_Get_InvalidID(t *testing.T) {
	r := newSessionTestRouter(newFakeSessionStore(), 100)
	w := doJSON(t, r, "GET", "/api/v1/sessions/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

func TestSessionHandler_Close_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "DELETE", "/api/v1/sessions/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
}

func TestSessionHandler_Messages_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	store.messages["acp-1"] = []models.Message{
		{ID: 1, SessionID: "acp-1", Role: "user", Kind: "user_message", Content: "hi", Sequence: 1},
		{ID: 2, SessionID: "acp-1", Role: "assistant", Kind: "agent_message_chunk", Content: "hello", Sequence: 2},
	}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "GET", "/api/v1/sessions/1/messages", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Messages []models.Message `json:"messages"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data.Messages) != 2 {
		t.Fatalf("消息数量 = %d, 期望 2", len(resp.Data.Messages))
	}
}

func TestSessionHandler_Cancel_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions/1/cancel", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
}

func TestSessionHandler_Cancel_NotActive(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusClosed}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions/1/cancel", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("状态码 = %d, 期望 409", w.Code)
	}
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "SESSION_NOT_ACTIVE" {
		t.Errorf("error code = %q, 期望 SESSION_NOT_ACTIVE", resp.Error.Code)
	}
}

func TestSessionHandler_Resume_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusError}
	store.resumeResult = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions/1/resume", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data models.Session `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Status != models.SessionStatusActive {
		t.Errorf("恢复后状态 = %q, 期望 active", resp.Data.Status)
	}
}

func TestSessionHandler_Resume_Closed(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusClosed}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions/1/resume", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("状态码 = %d, 期望 409", w.Code)
	}
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error.Code != "SESSION_CLOSED" {
		t.Errorf("error code = %q, 期望 SESSION_CLOSED", resp.Error.Code)
	}
}

func TestSessionHandler_Prompt_Empty(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(gin.H{"prompt": "  "})
	req := httptest.NewRequest("POST", "/api/v1/sessions/1/prompt", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

func TestSessionHandler_Prompt_NotFound(t *testing.T) {
	r := newSessionTestRouter(newFakeSessionStore(), 100)
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(gin.H{"prompt": "hi"})
	req := httptest.NewRequest("POST", "/api/v1/sessions/9/prompt", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d, 期望 404", w.Code)
	}
}

func TestSessionHandler_Prompt_NotActive(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusClosed}
	r := newSessionTestRouter(store, 100)
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(gin.H{"prompt": "hi"})
	req := httptest.NewRequest("POST", "/api/v1/sessions/1/prompt", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("状态码 = %d, 期望 409", w.Code)
	}
}

func TestSessionHandler_Prompt_SSE_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}

	ch := make(chan models.Message, 2)
	ch <- models.Message{ID: 1, SessionID: "acp-1", Role: "assistant", Kind: "agent_message_chunk", Content: "Hello", Sequence: 1}
	ch <- models.Message{ID: 2, SessionID: "acp-1", Role: "tool", Kind: "tool_call", Content: "Read file", Sequence: 2}
	close(ch)
	store.promptCh = ch

	r := newSessionTestRouter(store, 100)
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(gin.H{"prompt": "写一个 hello world"})
	req := httptest.NewRequest("POST", "/api/v1/sessions/1/prompt", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, 期望 text/event-stream", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("响应体缺少结束标记 data: [DONE]\n%s", body)
	}
	if !strings.Contains(body, `"content":"Hello"`) {
		t.Errorf("响应体缺少第一条消息内容\n%s", body)
	}
	if !strings.Contains(body, `"kind":"tool_call"`) {
		t.Errorf("响应体缺少第二条消息\n%s", body)
	}
}
