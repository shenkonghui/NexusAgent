package subagentmcp

import (
	"context"
	"testing"

	"opennexus/internal/acp"
	"opennexus/internal/database"
	"opennexus/internal/models"
	"opennexus/internal/repository"
)

// ====== 测试桩 ======

type stubSessionCreator struct {
	gotAgentType string
	gotWorkspace uint
	gotUserID    uint
	gotSource    string
	gotModel     string
	gotParent    *uint
	returnSession *models.Session
	returnErr     error
}

func (s *stubSessionCreator) CreateSessionWithParent(ctx context.Context, agentType string, workspaceID uint, userID uint, source, modelValue string, parentSessionID *uint) (*models.Session, error) {
	s.gotAgentType = agentType
	s.gotWorkspace = workspaceID
	s.gotUserID = userID
	s.gotSource = source
	s.gotModel = modelValue
	s.gotParent = parentSessionID
	if s.returnErr != nil {
		return nil, s.returnErr
	}
	if s.returnSession != nil {
		return s.returnSession, nil
	}
	return &models.Session{SessionID: "stub-session-id", AgentType: agentType, UserID: userID, Source: source}, nil
}

type stubSessionTaskRunner struct {
	gotCfg     acp.SessionTaskConfig
	returnRes  acp.SessionTaskResult
	returnErr  error
}

func (s *stubSessionTaskRunner) RunSessionTask(ctx context.Context, cfg acp.SessionTaskConfig) (acp.SessionTaskResult, error) {
	s.gotCfg = cfg
	return s.returnRes, s.returnErr
}

// ====== 辅助 ======

func setupUserCtx(t *testing.T, uid uint) context.Context {
	t.Helper()
	return withUserID(context.Background(), uid)
}

func setupPrefsRepo(t *testing.T, uid uint, lastAgent string) *repository.UserAgentPrefsRepository {
	t.Helper()
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("DELETE FROM user_agent_prefs")
	repo := repository.NewUserAgentPrefsRepository(db)
	if lastAgent != "" {
		la := lastAgent
		if _, err := repo.Patch(uid, &la, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

// ====== create_session 测试 ======

func TestCreateSession_Independent(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "claude")
	creator := &stubSessionCreator{}

	_, out, err := handleCreateSession(setupUserCtx(t, 5), prefs, creator, nil, createSessionIn{
		AgentType: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.SessionID != "stub-session-id" {
		t.Fatalf("session_id=%q", out.SessionID)
	}
	if out.ParentSessionID != 0 {
		t.Fatalf("独立会话不应有 parent, got %d", out.ParentSessionID)
	}
	if creator.gotParent != nil {
		t.Fatalf("独立会话 parent 应为 nil")
	}
	if creator.gotSource != models.SessionSourceManual {
		t.Fatalf("source=%q, 期望 manual", creator.gotSource)
	}
}

func TestCreateSession_InheritAgent(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "codebuddy")
	creator := &stubSessionCreator{}

	_, _, err := handleCreateSession(setupUserCtx(t, 5), prefs, creator, nil, createSessionIn{
		// AgentType 留空，应继承
	})
	if err != nil {
		t.Fatal(err)
	}
	if creator.gotAgentType != "codebuddy" {
		t.Fatalf("继承 agent=%q, 期望 codebuddy", creator.gotAgentType)
	}
}

func TestCreateSession_Unauthenticated(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "claude")
	creator := &stubSessionCreator{}

	_, _, err := handleCreateSession(context.Background(), prefs, creator, nil, createSessionIn{
		AgentType: "claude",
	})
	if err == nil {
		t.Fatal("未认证应返回错误")
	}
}

func TestCreateSession_ParentOwnershipMismatch(t *testing.T) {
	db, _ := database.Connect("file::memory:?cache=shared")
	db.Exec("DELETE FROM sessions")
	db.Exec("DELETE FROM workspaces")
	sessionRepo := repository.NewSessionRepository(db)
	wsRepo := repository.NewWorkspaceRepository(db)

	// 用户 5 的父会话
	ws := &models.Workspace{UserID: 5, Name: "ws", Cwd: "/tmp", Mode: models.WorkspaceModeTemporary}
	_ = wsRepo.Create(ws)
	parent := &models.Session{SessionID: "parent-uuid", AgentType: "claude", UserID: 5, Status: models.SessionStatusActive, WorkspaceID: &ws.ID}
	_ = sessionRepo.Create(parent)

	// 使用真实 repo 作为 lookup（*agent.Router 在生产中实现）
	lookup := &realSessionLookup{repo: sessionRepo}

	prefs := setupPrefsRepo(t, 9, "claude") // 用户 9
	creator := &stubSessionCreator{}

	// 用户 9 试图把会话挂到用户 5 的父会话下
	_, _, err := handleCreateSession(setupUserCtx(t, 9), prefs, creator, lookup, createSessionIn{
		AgentType:       "claude",
		ParentSessionID: parent.ID,
	})
	if err == nil {
		t.Fatal("跨用户引用父会话应失败")
	}
}

// ====== run_session_task 测试 ======

func TestRunSessionTask_Success(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "claude")
	runner := &stubSessionTaskRunner{
		returnRes: acp.SessionTaskResult{SessionID: "task-session", Result: "任务完成", Success: true},
	}

	_, out, err := handleRunSessionTask(setupUserCtx(t, 5), prefs, runner, nil, runSessionTaskIn{
		AgentType: "claude",
		Task:      "写一个 hello world",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Fatalf("期望成功, error=%s", out.Error)
	}
	if out.Result != "任务完成" {
		t.Fatalf("result=%q", out.Result)
	}
	if out.SessionID != "task-session" {
		t.Fatalf("session_id=%q", out.SessionID)
	}
	if runner.gotCfg.Prompt != "写一个 hello world" {
		t.Fatalf("prompt=%q", runner.gotCfg.Prompt)
	}
	if runner.gotCfg.UserID != 5 {
		t.Fatalf("userID=%d", runner.gotCfg.UserID)
	}
}

func TestRunSessionTask_EmptyTask(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "claude")
	runner := &stubSessionTaskRunner{}

	_, _, err := handleRunSessionTask(setupUserCtx(t, 5), prefs, runner, nil, runSessionTaskIn{
		AgentType: "claude",
		Task:      "   ",
	})
	if err == nil {
		t.Fatal("空 task 应返回错误")
	}
}

func TestRunSessionTask_TimeoutConfig(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "claude")
	runner := &stubSessionTaskRunner{
		returnRes: acp.SessionTaskResult{Success: true, Result: "ok"},
	}

	_, _, err := handleRunSessionTask(setupUserCtx(t, 5), prefs, runner, nil, runSessionTaskIn{
		AgentType:      "claude",
		Task:           "test",
		TimeoutSeconds: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.gotCfg.Timeout.Seconds() != 120 {
		t.Fatalf("timeout=%v, 期望 120s", runner.gotCfg.Timeout)
	}
}

// ====== resolveAgentType 测试 ======

func TestResolveAgentType_ExplicitOverridesInherit(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "codebuddy")
	at, err := resolveAgentType(prefs, 5, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if at != "claude" {
		t.Fatalf("显式 agent 应优先, got %q", at)
	}
}

func TestResolveAgentType_NoHistoryErrors(t *testing.T) {
	prefs := setupPrefsRepo(t, 5, "") // 用户从未使用过 agent
	_, err := resolveAgentType(prefs, 5, "")
	if err == nil {
		t.Fatal("无历史且未指定 agent 应返回错误")
	}
}

// realSessionLookup 包装 SessionRepository 实现 SessionLookup 接口（测试用）。
// 生产中由 *agent.Router 实现。
type realSessionLookup struct {
	repo *repository.SessionRepository
}

func (r *realSessionLookup) GetSessionByDBID(id uint) (*models.Session, error) {
	return r.repo.FindByID(id)
}
