package repository

import (
	"encoding/json"
	"testing"
)

func TestUserAgentPrefs_FindEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserAgentPrefsRepository(db)
	got, err := repo.FindByUserID(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != 1 {
		t.Fatalf("UserID=%d", got.UserID)
	}
	if got.LastAgentType != "" {
		t.Fatalf("LastAgentType=%q", got.LastAgentType)
	}
	if got.PrefsJSON != "" && got.PrefsJSON != "{}" {
		t.Fatalf("PrefsJSON=%q", got.PrefsJSON)
	}
}

func TestUserAgentPrefs_Patch_MergeAndDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserAgentPrefsRepository(db)

	last := "claude-code"
	got, err := repo.Patch(1, &last, "claude-code", map[string]string{
		"model": "m1",
		"mode":  "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.LastAgentType != "claude-code" {
		t.Fatalf("LastAgentType=%q", got.LastAgentType)
	}
	prefs := parsePrefs(t, got.PrefsJSON)
	if prefs["claude-code"]["model"] != "m1" || prefs["claude-code"]["mode"] != "default" {
		t.Fatalf("prefs=%v", prefs)
	}

	got, err = repo.Patch(1, nil, "claude-code", map[string]string{
		"model": "m2",
		"mode":  "",
	})
	if err != nil {
		t.Fatal(err)
	}
	prefs = parsePrefs(t, got.PrefsJSON)
	if prefs["claude-code"]["model"] != "m2" {
		t.Fatalf("model=%q", prefs["claude-code"]["model"])
	}
	if _, ok := prefs["claude-code"]["mode"]; ok {
		t.Fatalf("mode 应被删除: %v", prefs["claude-code"])
	}
	if got.LastAgentType != "claude-code" {
		t.Fatalf("未传 last 时不应清空: %q", got.LastAgentType)
	}

	got, err = repo.Patch(1, nil, "cursor", map[string]string{"model": "gpt"})
	if err != nil {
		t.Fatal(err)
	}
	prefs = parsePrefs(t, got.PrefsJSON)
	if prefs["claude-code"]["model"] != "m2" || prefs["cursor"]["model"] != "gpt" {
		t.Fatalf("prefs=%v", prefs)
	}
}

func parsePrefs(t *testing.T, raw string) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse prefs: %v", err)
	}
	return out
}
