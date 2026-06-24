package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"nexusagent/internal/database"
	"nexusagent/internal/models"
)

func newAuthSvc(t *testing.T) (*AuthService, *gorm.DB) {
	t.Helper()
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM users")
	db.Exec("DELETE FROM refresh_tokens")
	jwtSvc := NewJWTService("this-is-a-very-long-jwt-secret-key-32+bytes!", 15*time.Minute, time.Hour)
	svc := NewAuthService(db, jwtSvc, 10) // bcrypt cost=10 加速测试
	return svc, db
}

func TestAuthService_Register_Success(t *testing.T) {
	svc, db := newAuthSvc(t)
	user, err := svc.Register("alice", "alice@example.com", "Password123")
	if err != nil {
		t.Fatalf("Register 错误: %v", err)
	}
	if user.ID == 0 {
		t.Error("期望 ID 非零")
	}
	if user.PasswordHash == "Password123" {
		t.Error("密码不应明文存储")
	}
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count != 1 {
		t.Errorf("期望 1 条用户，实际 %d", count)
	}
}

func TestAuthService_Register_WeakPassword(t *testing.T) {
	svc, _ := newAuthSvc(t)
	if _, err := svc.Register("bob", "bob@example.com", "short"); err != ErrWeakPassword {
		t.Errorf("期望 ErrWeakPassword，实际 %v", err)
	}
}

func TestAuthService_Register_DuplicateUsername(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("carol", "carol@example.com", "Password123")
	if _, err := svc.Register("carol", "other@example.com", "Password123"); err != ErrUserExists {
		t.Errorf("期望 ErrUserExists，实际 %v", err)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("dave", "dave@example.com", "Password123")
	if _, err := svc.Register("dave2", "dave@example.com", "Password123"); err != ErrUserExists {
		t.Errorf("期望 ErrUserExists，实际 %v", err)
	}
}
