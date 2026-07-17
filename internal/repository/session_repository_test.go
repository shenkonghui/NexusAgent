package repository

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"nexusagent/internal/models"
)

func TestSessionRepo_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)

	s := &models.Session{
		SessionID:     "acp-session-1",
		AgentType:     "claude-code",
		Cwd:           "/tmp/work",
		Status:        models.SessionStatusActive,
		WorkspaceMode: "",
	}
	if err := repo.Create(s); err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if s.ID == 0 {
		t.Error("期望创建后 ID 非零")
	}
}

func TestSessionRepo_FindBySessionID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	_ = repo.Create(&models.Session{
		SessionID: "acp-session-2", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	})

	got, err := repo.FindBySessionID("acp-session-2")
	if err != nil {
		t.Fatalf("FindBySessionID 返回错误: %v", err)
	}
	if got.AgentType != "claude-code" {
		t.Errorf("AgentType = %q", got.AgentType)
	}
}

func TestSessionRepo_FindBySessionID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	if _, err := repo.FindBySessionID("missing"); err == nil {
		t.Error("期望未找到时返回错误")
	}
}

func TestSessionRepo_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	s := &models.Session{
		SessionID: "acp-session-3", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	}
	_ = repo.Create(s)

	now := time.Now()
	if err := repo.UpdateStatus(s.ID, models.SessionStatusClosed, &now); err != nil {
		t.Fatalf("UpdateStatus 返回错误: %v", err)
	}
	got, _ := repo.FindBySessionID("acp-session-3")
	if got.Status != models.SessionStatusClosed {
		t.Errorf("Status = %q, 期望 closed", got.Status)
	}
	if got.ClosedAt == nil {
		t.Error("期望 ClosedAt 非空")
	}
}

func TestSessionRepo_UpdateLastPrompt(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	s := &models.Session{
		SessionID: "acp-session-4", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	}
	_ = repo.Create(s)

	if err := repo.UpdateLastPrompt(s.ID, "写一个 hello world"); err != nil {
		t.Fatalf("UpdateLastPrompt 返回错误: %v", err)
	}
	got, _ := repo.FindBySessionID("acp-session-4")
	if got.LastPrompt != "写一个 hello world" {
		t.Errorf("LastPrompt = %q", got.LastPrompt)
	}
}

func TestSessionRepo_FindByUserID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	_ = repo.Create(&models.Session{SessionID: "u1-s1", AgentType: "claude-code", Cwd: "/tmp", Status: models.SessionStatusActive, WorkspaceMode: "", UserID: 1})
	_ = repo.Create(&models.Session{SessionID: "u1-s2", AgentType: "claude-code", Cwd: "/tmp", Status: models.SessionStatusActive, WorkspaceMode: "", UserID: 1})
	_ = repo.Create(&models.Session{SessionID: "u2-s1", AgentType: "claude-code", Cwd: "/tmp", Status: models.SessionStatusActive, WorkspaceMode: "", UserID: 2})

	sessions, err := repo.FindByUserID(1)
	if err != nil {
		t.Fatalf("FindByUserID 返回错误: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("期望 2 条会话，实际 %d", len(sessions))
	}
}

func TestSessionRepo_MarkActiveAsError(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	_ = repo.Create(&models.Session{SessionID: "err-1", AgentType: "claude-code", Cwd: "/tmp", Status: models.SessionStatusActive, WorkspaceMode: ""})
	_ = repo.Create(&models.Session{SessionID: "err-2", AgentType: "claude-code", Cwd: "/tmp", Status: models.SessionStatusClosed, WorkspaceMode: ""})

	if err := repo.MarkActiveAsError(); err != nil {
		t.Fatalf("MarkActiveAsError 返回错误: %v", err)
	}
	active, _ := repo.FindBySessionID("err-1")
	if active.Status != models.SessionStatusError {
		t.Errorf("期望 active 标记为 error，实际 %q", active.Status)
	}
	closed, _ := repo.FindBySessionID("err-2")
	if closed.Status != models.SessionStatusClosed {
		t.Errorf("已关闭的不应被修改，实际 %q", closed.Status)
	}
}

func TestSessionRepo_FindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	s := &models.Session{
		SessionID: "find-by-id-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	}
	_ = repo.Create(s)

	got, err := repo.FindByID(s.ID)
	if err != nil {
		t.Fatalf("FindByID 返回错误: %v", err)
	}
	if got.SessionID != "find-by-id-1" {
		t.Errorf("SessionID = %q", got.SessionID)
	}
}

func TestSessionRepo_FindByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	if _, err := repo.FindByID(99999); err == nil {
		t.Error("期望未找到时返回错误")
	}
}

func TestSessionRepo_UpdateAgentSessionID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	s := &models.Session{
		SessionID: "stable-session-id", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusError, WorkspaceMode: "",
	}
	_ = repo.Create(s)

	if err := repo.UpdateAgentSessionID(s.ID, "acp-agent-id"); err != nil {
		t.Fatalf("UpdateAgentSessionID 返回错误: %v", err)
	}

	got, err := repo.FindByID(s.ID)
	if err != nil {
		t.Fatalf("FindByID 返回错误: %v", err)
	}
	if got.SessionID != "stable-session-id" {
		t.Errorf("SessionID = %q, 期望 stable-session-id", got.SessionID)
	}
	if got.AgentSessionID != "acp-agent-id" {
		t.Errorf("AgentSessionID = %q, 期望 acp-agent-id", got.AgentSessionID)
	}
}

var _ = gorm.ErrRecordNotFound
