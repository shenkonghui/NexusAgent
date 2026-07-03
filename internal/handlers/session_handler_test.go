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

	"github.com/coder/acp-go-sdk"
	"github.com/gin-gonic/gin"

	acplocal "nexusagent/internal/acp"
	"nexusagent/internal/agent"
	"nexusagent/internal/middleware"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

// fakeSessionStore 是内存版 SessionStore，用于隔离真实 ACP 子进程。
type fakeSessionStore struct {
	sessions     map[uint]*models.Session
	messages     map[string][]models.Message
	createErr    error
	listErr      error
	deleteErr    error
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

func (f *fakeSessionStore) CreateSession(_ context.Context, agentType string, workspaceID uint, userID uint, _ string) (*models.Session, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.nextID++
	wid := workspaceID
	s := &models.Session{
		ID:          f.nextID,
		SessionID:   "acp-" + strconv.Itoa(int(f.nextID)),
		AgentType:   agentType,
		Status:      models.SessionStatusActive,
		UserID:      userID,
		WorkspaceID: &wid,
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

func (f *fakeSessionStore) ListSessionsBySource(userID uint, source string) ([]models.Session, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []models.Session
	for _, s := range f.sessions {
		if s.UserID == userID && (source == "" || s.Source == source) {
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

func (f *fakeSessionStore) DeleteSession(_ context.Context, _ string) error { return f.deleteErr }
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

func (f *fakeSessionStore) ListExecutions(_ string) ([]repository.ExecutionAggregate, error) {
	return nil, nil
}

func (f *fakeSessionStore) ListCommands(_ string) ([]acp.AvailableCommand, error) {
	return nil, nil
}

func (f *fakeSessionStore) ListConfiguredCommandsForSession(_ string) ([]acplocal.SlashCommand, error) {
	return nil, nil
}

func (f *fakeSessionStore) ListConfigOptions(_ string) ([]acp.SessionConfigOption, error) {
	return nil, nil
}

func (f *fakeSessionStore) ListModes(_ string) ([]acp.SessionMode, error) {
	return nil, nil
}

func (f *fakeSessionStore) ListSkills(_ string) ([]acplocal.Skill, error) {
	return nil, nil
}

func (f *fakeSessionStore) SetConfigOption(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *fakeSessionStore) SetSessionMode(_ context.Context, _, _ string) error {
	return nil
}

func (f *fakeSessionStore) RespondPermission(_, _, _ string, _ bool) error {
	return nil
}

func (f *fakeSessionStore) UpdateTitle(dbSessionID uint, title string) error {
	for i := range f.sessions {
		if f.sessions[i].ID == dbSessionID {
			f.sessions[i].Title = title
			return nil
		}
	}
	return errors.New("session not found")
}

func (f *fakeSessionStore) Prompt(_ context.Context, _, _ string) (<-chan models.Message, error) {
	if f.promptErr != nil {
		return nil, f.promptErr
	}
	return f.promptCh, nil
}

func (f *fakeSessionStore) GetWorkspaceCwd(workspaceID uint) (string, error) {
	for _, s := range f.sessions {
		if s.WorkspaceID != nil && *s.WorkspaceID == workspaceID && s.Cwd != "" {
			return s.Cwd, nil
		}
	}
	return "/tmp", nil
}

// closeNotifyRecorder 包装 httptest.ResponseRecorder，补充 CloseNotifier 接口以兼容 Gin 的 c.Stream。
type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
	notify chan bool
}

func newCloseNotifyRecorder() *closeNotifyRecorder {
	return &closeNotifyRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		notify:           make(chan bool),
	}
}

func (c *closeNotifyRecorder) CloseNotify() <-chan bool {
	return c.notify
}

// doSSERequest 发送 SSE 流式请求，使用支持 CloseNotifier 的 recorder。
func doSSERequest(t *testing.T, r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		j, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		buf = *bytes.NewBuffer(j)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := newCloseNotifyRecorder()
	r.ServeHTTP(rec, req)
	return rec.ResponseRecorder
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
	v1.DELETE("/sessions/:id", h.Delete)
	v1.POST("/sessions/:id/prompt", h.Prompt)
	v1.POST("/sessions/:id/cancel", h.Cancel)
	v1.POST("/sessions/:id/resume", h.Resume)
	v1.GET("/sessions/:id/messages", h.Messages)
	v1.GET("/sessions/:id/commands", h.Commands)
	v1.GET("/sessions/:id/config-options", h.ConfigOptions)
	v1.POST("/sessions/:id/config-options", h.SetConfigOption)
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

func TestSessionHandler_Delete_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "DELETE", "/api/v1/sessions/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
}

func TestSessionHandler_Delete_NotOwner(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 200, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "DELETE", "/api/v1/sessions/1", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d, 期望 404（不属于当前用户）", w.Code)
	}
}

func TestSessionHandler_Delete_Error(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	store.deleteErr = errors.New("boom")
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "DELETE", "/api/v1/sessions/1", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d, 期望 500", w.Code)
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
	// 已关闭会话现在可以被重新打开（重开）
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusClosed}
	store.resumeResult = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}
	r := newSessionTestRouter(store, 100)
	w := doJSON(t, r, "POST", "/api/v1/sessions/1/resume", gin.H{"cwd": "/tmp"})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200（已关闭会话可重开）, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data models.Session `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Status != models.SessionStatusActive {
		t.Errorf("重开后状态 = %q, 期望 active", resp.Data.Status)
	}
}

func TestSessionHandler_Commands_Success(t *testing.T) {
	store := &commandsFakeStore{
		sessions: map[uint]*models.Session{1: {ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusActive}},
		cmds: []acp.AvailableCommand{
			{Name: "help", Description: "显示帮助"},
			{Name: "clear", Description: "清空上下文", Input: &acp.AvailableCommandInput{}},
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey(), uint(100))
		c.Next()
	})
	h := NewSessionHandler(store)
	r.GET("/api/v1/sessions/:id/commands", h.Commands)
	w := doJSON(t, r, "GET", "/api/v1/sessions/1/commands", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Commands []commandItem `json:"commands"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data.Commands) != 2 {
		t.Fatalf("命令数量 = %d, 期望 2", len(resp.Data.Commands))
	}
	if resp.Data.Commands[0].Name != "help" || resp.Data.Commands[1].HasInput != true {
		t.Errorf("命令字段不正确: %+v", resp.Data.Commands)
	}
}

// commandsFakeStore 是仅用于 Commands 接口的最小 SessionStore 实现。
type commandsFakeStore struct {
	sessions map[uint]*models.Session
	cmds     []acp.AvailableCommand
}

func (s *commandsFakeStore) CreateSession(context.Context, string, uint, uint, string) (*models.Session, error) {
	return nil, nil
}
func (s *commandsFakeStore) ListSessions(uint) ([]models.Session, error) { return nil, nil }
func (s *commandsFakeStore) ListSessionsBySource(uint, string) ([]models.Session, error) {
	return nil, nil
}
func (s *commandsFakeStore) GetSessionByDBID(id uint) (*models.Session, error) {
	if sess, ok := s.sessions[id]; ok {
		return sess, nil
	}
	return nil, errors.New("会话不存在")
}
func (s *commandsFakeStore) DeleteSession(context.Context, string) error { return nil }
func (s *commandsFakeStore) CancelSession(context.Context, string) error { return nil }
func (s *commandsFakeStore) ResumeSession(context.Context, string) (*models.Session, error) {
	return nil, nil
}
func (s *commandsFakeStore) ListMessages(string) ([]models.Message, error) { return nil, nil }
func (s *commandsFakeStore) ListExecutions(string) ([]repository.ExecutionAggregate, error) {
	return nil, nil
}
func (s *commandsFakeStore) ListCommands(_ string) ([]acp.AvailableCommand, error) {
	return s.cmds, nil
}

func (s *commandsFakeStore) ListConfiguredCommandsForSession(_ string) ([]acplocal.SlashCommand, error) {
	return nil, nil
}

func (s *commandsFakeStore) ListConfigOptions(_ string) ([]acp.SessionConfigOption, error) {
	return nil, nil
}

func (s *commandsFakeStore) ListModes(_ string) ([]acp.SessionMode, error) {
	return nil, nil
}

func (s *commandsFakeStore) ListSkills(_ string) ([]acplocal.Skill, error) {
	return nil, nil
}

func (s *commandsFakeStore) SetConfigOption(_ context.Context, _, _, _ string) error {
	return nil
}
func (s *commandsFakeStore) SetSessionMode(_ context.Context, _, _ string) error { return nil }
func (s *commandsFakeStore) RespondPermission(_, _, _ string, _ bool) error     { return nil }
func (s *commandsFakeStore) UpdateTitle(_ uint, _ string) error { return nil }
func (s *commandsFakeStore) Prompt(context.Context, string, string) (<-chan models.Message, error) {
	return nil, nil
}
func (s *commandsFakeStore) GetWorkspaceCwd(uint) (string, error) { return "/tmp", nil }

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

func TestSessionHandler_Prompt_Resume_Failed(t *testing.T) {
	store := newFakeSessionStore()
	store.sessions[1] = &models.Session{ID: 1, SessionID: "acp-1", UserID: 100, Status: models.SessionStatusClosed}
	store.resumeErr = errors.New("resume failed")
	r := newSessionTestRouter(store, 100)
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(gin.H{"prompt": "hi"})
	req := httptest.NewRequest("POST", "/api/v1/sessions/1/prompt", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d, 期望 500", w.Code)
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
	w := doSSERequest(t, r, "POST", "/api/v1/sessions/1/prompt", gin.H{"prompt": "写一个 hello world"})

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
