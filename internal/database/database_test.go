package database

import (
	"testing"

	"nexusagent/internal/models"
)

func TestConnect_MigratesTables(t *testing.T) {
	db, err := Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Connect 返回错误: %v", err)
	}

	if !db.Migrator().HasTable(&models.User{}) {
		t.Error("期望 users 表已迁移，实际不存在")
	}
	if !db.Migrator().HasTable(&models.RefreshToken{}) {
		t.Error("期望 refresh_tokens 表已迁移，实际不存在")
	}
}

func TestConnect_MigratesSessionsTable(t *testing.T) {
	db, err := Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Connect 返回错误: %v", err)
	}
	if !db.Migrator().HasTable(&models.Session{}) {
		t.Error("期望 sessions 表已迁移，实际不存在")
	}
	if !db.Migrator().HasColumn(&models.Session{}, "agent_session_id") {
		t.Error("期望 sessions.agent_session_id 列已迁移")
	}
}

func TestConnect_MigrateAgentSessionID(t *testing.T) {
	db, err := Connect("file:migrate-agent-sid?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("Connect 返回错误: %v", err)
	}
	// 模拟旧数据：session_id 存的是 ACP id，agent_session_id 为空
	old := &models.Session{
		SessionID: "acp-old-1", AgentType: "cursor", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: "",
	}
	if err := db.Create(old).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	pending := &models.Session{
		SessionID: "uuid-pending", AgentType: "cursor", Cwd: "/tmp",
		Status: models.SessionStatusPending, WorkspaceMode: "",
	}
	if err := db.Create(pending).Error; err != nil {
		t.Fatalf("Create pending: %v", err)
	}

	if err := migrateAgentSessionID(db); err != nil {
		t.Fatalf("migrateAgentSessionID: %v", err)
	}

	var got models.Session
	if err := db.First(&got, old.ID).Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if got.SessionID != "acp-old-1" {
		t.Errorf("SessionID 不应被改: %q", got.SessionID)
	}
	if got.AgentSessionID != "acp-old-1" {
		t.Errorf("AgentSessionID = %q, 期望回填 acp-old-1", got.AgentSessionID)
	}

	var pend models.Session
	if err := db.First(&pend, pending.ID).Error; err != nil {
		t.Fatalf("First pending: %v", err)
	}
	if pend.AgentSessionID != "" {
		t.Errorf("pending 不应回填 AgentSessionID, got %q", pend.AgentSessionID)
	}
}
