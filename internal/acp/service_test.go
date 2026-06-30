package acp

import (
	"context"
	"testing"

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
	return db
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	db := setupACPTestDB(t)
	wsCfg := config.WorkspaceConfig{
		DefaultMode:   "external",
		TempDirPrefix: "test-",
	}
	return NewService(db, wsCfg)
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
		WorkspaceMode: models.WorkspaceModeExternal,
	})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
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
		Status: models.SessionStatusActive, WorkspaceMode: models.WorkspaceModeExternal,
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
		Status: models.SessionStatusActive, WorkspaceMode: models.WorkspaceModeExternal,
	}
	_ = repo.Create(sess)

	msgRepo := repository.NewMessageRepository(db)
	_ = msgRepo.Create(&models.Message{SessionID: "msg-list-1", DBSessionID: sess.ID, Role: models.MessageRoleUser, Kind: models.MessageKindUserMessageChunk, Content: "问题", RawJSON: "{}", Sequence: 1})
	_ = msgRepo.Create(&models.Message{SessionID: "msg-list-1", DBSessionID: sess.ID, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "回答", RawJSON: "{}", Sequence: 2})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
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
		Status: models.SessionStatusActive, WorkspaceMode: models.WorkspaceModeExternal,
	})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
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
		Status: models.SessionStatusClosed, WorkspaceMode: models.WorkspaceModeExternal,
	})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
	_, err := svc.ResumeSession(context.Background(), "closed-resume-1", "")
	if err == nil {
		t.Error("期望后端未注册时重开返回错误")
	}
}

func TestService_ResumeSession_CwdNotExists(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	_ = repo.Create(&models.Session{
		SessionID: "closed-resume-2", AgentType: "claude-code", Cwd: "/this/path/does/not/exist",
		Status: models.SessionStatusError, WorkspaceMode: models.WorkspaceModeExternal,
	})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
	_, err := svc.ResumeSession(context.Background(), "closed-resume-2", "")
	if err == nil {
		t.Error("期望工作目录不存在时返回错误")
	}
}

func TestService_ResumeSession_SessionNotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.ResumeSession(context.Background(), "nonexistent", ""); err == nil {
		t.Error("期望不存在的会话恢复返回错误")
	}
}

func TestService_DeleteSession_RemovesSessionAndMessages(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	sess := &models.Session{
		SessionID: "delete-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusClosed, WorkspaceMode: models.WorkspaceModeExternal,
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

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
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
}

func TestService_DeleteSession_NotFound(t *testing.T) {
	svc := newTestService(t)
	if err := svc.DeleteSession(context.Background(), "nonexistent"); err == nil {
		t.Error("期望不存在的会话删除返回错误")
	}
}
