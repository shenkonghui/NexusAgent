package acp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"nexusagent/internal/config"
	"nexusagent/internal/database"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

func setupACPTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM users")
	db.Exec("DELETE FROM refresh_tokens")
	db.Exec("DELETE FROM sessions")
	db.Exec("DELETE FROM messages")
	db.Exec("DELETE FROM workspaces")
	db.Exec("DELETE FROM running_tasks")
	return db
}

func testDiscoveryConfig(t *testing.T) (config.SkillsConfig, config.CommandsConfig, config.RulesConfig) {
	t.Helper()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "this-is-a-very-long-jwt-secret-key-32+bytes!"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate 默认配置失败: %v", err)
	}
	return cfg.Agents.Skills, cfg.Agents.Commands, cfg.Agents.Rules
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	db := setupACPTestDB(t)
	wsCfg := config.WorkspaceConfig{
		DefaultMode:   "external",
		TempDirPrefix: "test-",
	}
	skills, commands, rules := testDiscoveryConfig(t)
	return NewService(db, wsCfg, skills, commands, rules)
}

func TestService_RegisterBackend(t *testing.T) {
	svc := newTestService(t)
	svc.RegisterBackend(NewClaudeCodeBackend(config.ClaudeCodeConfig{}))

	b, err := svc.GetBackend("claude-code")
	if err != nil {
		t.Fatalf("GetBackend 错误: %v", err)
	}
	if b.Name() != "claude-code" {
		t.Errorf("Name = %q", b.Name())
	}
}

func TestService_GetBackend_NotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetBackend("unknown"); err == nil {
		t.Error("期望未注册后端返回错误")
	}
}

func TestService_ListSessions_Empty(t *testing.T) {
	svc := newTestService(t)
	sessions, err := svc.ListSessions(1)
	if err != nil {
		t.Fatalf("ListSessions 错误: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("期望 0 条会话，实际 %d", len(sessions))
	}
}

func TestService_GetSession_NotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetSession("missing"); err == nil {
		t.Error("期望未找到时返回错误")
	}
}



func TestService_RecoverActiveSessions(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	_ = repo.Create(&models.Session{
		SessionID:     "recovery-1",
		AgentType:     "claude-code",
		Cwd:           "/tmp",
		Status:        models.SessionStatusActive,
		WorkspaceMode: "",
	})

	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)
	svc.RecoverActiveSessions()

	s, _ := svc.GetSession("recovery-1")
	if s.Status != models.SessionStatusError {
		t.Errorf("期望恢复后 status=error，实际 %q", s.Status)
	}
}

func TestService_GetSessionByDBID(t *testing.T) {
	svc := newTestService(t)
	repo := repository.NewSessionRepository(setupACPTestDB(t))
	_ = repo.Create(&models.Session{
		SessionID: "db-id-test", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	})
	// 用 svc 自身的 sessions 仓库重新查（因为 newTestService 用的是同一个 db）
	sess, err := svc.GetSession("db-id-test")
	if err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}

	got, err := svc.GetSessionByDBID(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionByDBID 返回错误: %v", err)
	}
	if got.SessionID != "db-id-test" {
		t.Errorf("SessionID = %q", got.SessionID)
	}
}

func TestService_GetSessionByDBID_NotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetSessionByDBID(99999); err == nil {
		t.Error("期望不存在的 DB ID 返回错误")
	}
}

func TestService_ListMessages(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	sess := &models.Session{
		SessionID: "msg-list-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	}
	_ = repo.Create(sess)

	msgRepo := repository.NewMessageRepository(db)
	_ = msgRepo.Create(&models.Message{SessionID: "msg-list-1", DBSessionID: sess.ID, Role: models.MessageRoleUser, Kind: models.MessageKindUserMessageChunk, Content: "问题", RawJSON: "{}", Sequence: 1})
	_ = msgRepo.Create(&models.Message{SessionID: "msg-list-1", DBSessionID: sess.ID, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "回答", RawJSON: "{}", Sequence: 2})

	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)
	msgs, err := svc.ListMessages("msg-list-1")
	if err != nil {
		t.Fatalf("ListMessages 返回错误: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息，实际 %d", len(msgs))
	}
	if msgs[0].Content != "问题" {
		t.Errorf("第一条消息 Content = %q", msgs[0].Content)
	}
	if msgs[1].Content != "回答" {
		t.Errorf("第二条消息 Content = %q", msgs[1].Content)
	}
}

func TestService_ListMessages_SessionNotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.ListMessages("nonexistent"); err == nil {
		t.Error("期望不存在的会话返回错误")
	}
}

func TestService_ListMessages_Empty(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	_ = repo.Create(&models.Session{
		SessionID: "empty-msg-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	})

	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)
	msgs, err := svc.ListMessages("empty-msg-1")
	if err != nil {
		t.Fatalf("ListMessages 返回错误: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("期望 0 条消息，实际 %d", len(msgs))
	}
}

func TestService_ResumeSession_Closed_NoBackend(t *testing.T) {
	// 已关闭会话现在允许重开；此处后端未注册，应在获取后端阶段失败
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	_ = repo.Create(&models.Session{
		SessionID: "closed-resume-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusClosed, WorkspaceMode: "",
	})

	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)
	_, err := svc.ResumeSession(context.Background(), "closed-resume-1")
	if err == nil {
		t.Error("期望后端未注册时重开返回错误")
	}
}

func TestService_ResumeSession_PersistentCwdNotExists(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	wsRepo := repository.NewWorkspaceRepository(db)
	missingDir := filepath.Join(t.TempDir(), "missing-persistent")
	ws := &models.Workspace{
		UserID: 1, Name: "项目", Cwd: missingDir,
		Mode: models.WorkspaceModePersistent,
	}
	if err := wsRepo.Create(ws); err != nil {
		t.Fatalf("创建 workspace 失败: %v", err)
	}
	wid := ws.ID
	_ = repo.Create(&models.Session{
		SessionID: "closed-resume-3", AgentType: "claude-code", Cwd: missingDir,
		Status: models.SessionStatusError, WorkspaceID: &wid,
	})

	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)
	_, err := svc.ResumeSession(context.Background(), "closed-resume-3")
	if err == nil {
		t.Error("persistent 工作目录不存在时期望返回错误")
	}
}

func TestService_ResumeSession_CwdNotExists(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	wsRepo := repository.NewWorkspaceRepository(db)
	tempDir := filepath.Join(t.TempDir(), "missing-resume")
	ws := &models.Workspace{
		UserID: 1, Name: "默认", Cwd: tempDir,
		Mode: models.WorkspaceModeTemporary, TempDir: tempDir,
	}
	if err := wsRepo.Create(ws); err != nil {
		t.Fatalf("创建 workspace 失败: %v", err)
	}
	wid := ws.ID
	_ = repo.Create(&models.Session{
		SessionID: "closed-resume-2", AgentType: "claude-code", Cwd: tempDir,
		Status: models.SessionStatusError, WorkspaceID: &wid,
	})

	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)
	_, err := svc.ResumeSession(context.Background(), "closed-resume-2")
	if err == nil {
		t.Error("期望后端未注册时重开返回错误")
	}
	if _, statErr := os.Stat(tempDir); statErr != nil {
		t.Errorf("temporary 工作目录应已重建: %v", statErr)
	}
}

func TestService_ResumeSession_SessionNotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.ResumeSession(context.Background(), "nonexistent"); err == nil {
		t.Error("期望不存在的会话恢复返回错误")
	}
}

func TestService_DeleteSession_RemovesSessionAndMessages(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	wsRepo := repository.NewWorkspaceRepository(db)
	tempDir := filepath.Join(t.TempDir(), "keep-after-delete")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatalf("创建目录: %v", err)
	}
	ws := &models.Workspace{
		UserID: 1, Name: "默认", Cwd: tempDir,
		Mode: models.WorkspaceModeTemporary, TempDir: tempDir,
	}
	if err := wsRepo.Create(ws); err != nil {
		t.Fatalf("创建 workspace 失败: %v", err)
	}
	wid := ws.ID
	sess := &models.Session{
		SessionID: "delete-1", AgentType: "claude-code", Cwd: tempDir,
		Status: models.SessionStatusClosed, WorkspaceID: &wid,
	}
	if err := repo.Create(sess); err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	if err := msgRepo.Create(&models.Message{
		SessionID: "delete-1", DBSessionID: sess.ID, Role: "user",
		Kind: "user_message_chunk", Content: "hi", Sequence: 1, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("创建消息失败: %v", err)
	}

	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)
	if err := svc.DeleteSession(context.Background(), "delete-1"); err != nil {
		t.Fatalf("DeleteSession 错误: %v", err)
	}
	if _, err := repo.FindByID(sess.ID); err == nil {
		t.Error("期望会话记录已被删除")
	}
	msgs, _ := msgRepo.FindByDBSessionID(sess.ID)
	if len(msgs) != 0 {
		t.Errorf("期望消息已删除，实际 %d 条", len(msgs))
	}
	if _, err := os.Stat(tempDir); err != nil {
		t.Errorf("删除会话后工作区目录应保留: %v", err)
	}
}

func TestService_DeleteSession_NotFound(t *testing.T) {
	svc := newTestService(t)
	if err := svc.DeleteSession(context.Background(), "nonexistent"); err == nil {
		t.Error("期望不存在的会话删除返回错误")
	}
}

// TestMsgBroadcaster_FanOut 验证多订阅者都能收到广播的消息。
func TestMsgBroadcaster_FanOut(t *testing.T) {
	bc := newMsgBroadcaster(0)
	ch1, _ := bc.subscribe(16)
	ch2, _ := bc.subscribe(16)

	msg := models.Message{Sequence: 1, Content: "hello"}
	bc.broadcast(msg)

	m1 := <-ch1
	m2 := <-ch2
	if m1.Content != "hello" || m2.Content != "hello" {
		t.Errorf("两个订阅者都应收到消息，得到 %q 和 %q", m1.Content, m2.Content)
	}
	if bc.subscriberCount() != 2 {
		t.Errorf("期望 2 个订阅者，实际 %d", bc.subscriberCount())
	}

	bc.close()
	if bc.subscriberCount() != 0 {
		t.Errorf("关闭后期望 0 个订阅者，实际 %d", bc.subscriberCount())
	}
}

// TestService_RecoverActiveSessions_InterruptsRunningTasks 验证启动恢复会将 running 状态的 running_task 标记为 interrupted。
func TestService_RecoverActiveSessions_InterruptsRunningTasks(t *testing.T) {
	db := setupACPTestDB(t)
	skills, commands, rules := testDiscoveryConfig(t)
	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"}, skills, commands, rules)

	// 插入一个 running 状态的 running_task
	taskRepo := repository.NewRunningTaskRepository(db)
	_ = taskRepo.Create(&models.RunningTask{
		DBSessionID: 1,
		UserID:      1,
		Prompt:      "test prompt",
		Status:      models.RunningTaskStatusRunning,
		StartedAt:   time.Now(),
	})

	// 触发恢复
	svc.RecoverActiveSessions()

	tasks, _ := svc.ListInterruptedTasks(1)
	if len(tasks) != 1 {
		t.Fatalf("期望 1 个 interrupted 任务，实际 %d", len(tasks))
	}
	if tasks[0].Status != models.RunningTaskStatusInterrupted {
		t.Errorf("期望 status=interrupted，实际 %q", tasks[0].Status)
	}
}

// TestRunningTaskRepository_CRUD 验证 running_task 仓库的基本 CRUD。
func TestRunningTaskRepository_CRUD(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewRunningTaskRepository(db)

	task := &models.RunningTask{
		DBSessionID: 10,
		UserID:      5,
		Prompt:      "hello agent",
		Status:      models.RunningTaskStatusRunning,
		StartedAt:   time.Now(),
	}
	if err := repo.Create(task); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if task.ID == 0 {
		t.Fatal("Create 后 ID 应非零")
	}

	// UpdateLastSeq
	if err := repo.UpdateLastSeq(task.ID, 42); err != nil {
		t.Fatalf("UpdateLastSeq 失败: %v", err)
	}
	got, _ := repo.FindByID(task.ID)
	if got.LastSeq != 42 {
		t.Errorf("期望 LastSeq=42，实际 %d", got.LastSeq)
	}

	// MarkRunningAsInterrupted
	if err := repo.MarkRunningAsInterrupted(); err != nil {
		t.Fatalf("MarkRunningAsInterrupted 失败: %v", err)
	}
	interrupted, _ := repo.FindInterruptedByDBSessionID(10)
	if len(interrupted) != 1 {
		t.Errorf("期望 1 个 interrupted 任务，实际 %d", len(interrupted))
	}

	// UpdateStatus done
	if err := repo.UpdateStatus(task.ID, models.RunningTaskStatusDone, nil); err != nil {
		t.Fatalf("UpdateStatus 失败: %v", err)
	}
	interrupted2, _ := repo.FindInterruptedByDBSessionID(10)
	if len(interrupted2) != 0 {
		t.Errorf("标记 done 后期望 0 个 interrupted，实际 %d", len(interrupted2))
	}
}
