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
}
