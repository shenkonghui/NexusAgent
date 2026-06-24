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

func TestService_CloseSession_NotFound(t *testing.T) {
	svc := newTestService(t)
	if err := svc.CloseSession(context.Background(), "missing"); err == nil {
		t.Error("期望关闭不存在的会话返回错误")
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
