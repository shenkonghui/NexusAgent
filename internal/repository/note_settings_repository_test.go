package repository

import (
	"errors"
	"testing"

	"opennexus/internal/models"
)

func TestNoteSettings_SetMCPTokenOnce_AndFind(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNoteSettingsRepository(db)

	if err := repo.SetMCPTokenOnce(1, "tok-abc"); err != nil {
		t.Fatalf("SetMCPTokenOnce: %v", err)
	}
	got, err := repo.FindByMCPToken("tok-abc")
	if err != nil {
		t.Fatalf("FindByMCPToken: %v", err)
	}
	if got.UserID != 1 {
		t.Fatalf("UserID = %d, 期望 1", got.UserID)
	}
	if err := repo.SetMCPTokenOnce(1, "tok-xyz"); !errors.Is(err, ErrMCPTokenAlreadySet) {
		t.Fatalf("第二次生成 err=%v, 期望 ErrMCPTokenAlreadySet", err)
	}
}

func TestNoteSettings_Upsert_PreservesMCPToken(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNoteSettingsRepository(db)
	if err := repo.SetMCPTokenOnce(1, "keep-me"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(&models.NoteSettings{UserID: 1, AgentType: "claude"}); err != nil {
		t.Fatal(err)
	}
	s, err := repo.FindByUserID(1)
	if err != nil {
		t.Fatal(err)
	}
	if s.McpToken != "keep-me" {
		t.Fatalf("Upsert 覆盖了 mcp_token: %q", s.McpToken)
	}
	if s.AgentType != "claude" {
		t.Fatalf("AgentType = %q, 期望 claude", s.AgentType)
	}
}
