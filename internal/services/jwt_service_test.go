package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "this-is-a-very-long-jwt-secret-key-32+bytes!"

func TestJWTService_GenerateAccess(t *testing.T) {
	svc := NewJWTService(testSecret, 15*time.Minute, time.Hour)
	token, err := svc.GenerateAccessToken(42, "alice", "admin")
	if err != nil {
		t.Fatalf("GenerateAccessToken 错误: %v", err)
	}
	if token == "" {
		t.Fatal("期望非空 token")
	}

	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse 错误: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("UserID = %d", claims.UserID)
	}
	if claims.Username != "alice" {
		t.Errorf("Username = %q", claims.Username)
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q", claims.Role)
	}
	if claims.TokenType != "access" {
		t.Errorf("TokenType = %q", claims.TokenType)
	}
}

func TestJWTService_GenerateRefresh(t *testing.T) {
	svc := NewJWTService(testSecret, 15*time.Minute, time.Hour)
	token, jti, err := svc.GenerateRefreshToken(42)
	if err != nil {
		t.Fatalf("错误: %v", err)
	}
	if jti == "" {
		t.Error("期望非空 jti")
	}
	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse 错误: %v", err)
	}
	if claims.TokenType != "refresh" {
		t.Errorf("TokenType = %q", claims.TokenType)
	}
	if claims.JTI != jti {
		t.Errorf("JTI 不匹配")
	}
}

func TestJWTService_Parse_Expired(t *testing.T) {
	svc := NewJWTService(testSecret, -1*time.Minute, time.Hour)
	token, _ := svc.GenerateAccessToken(1, "u", "user")
	if _, err := svc.Parse(token); err == nil {
		t.Error("期望过期 token 返回错误")
	}
}

func TestJWTService_Parse_WrongSecret(t *testing.T) {
	svc := NewJWTService(testSecret, 15*time.Minute, time.Hour)
	token, _ := svc.GenerateAccessToken(1, "u", "user")
	other := NewJWTService("another-long-secret-key-that-is-32+bytes-ok!", 15*time.Minute, time.Hour)
	if _, err := other.Parse(token); err == nil {
		t.Error("期望错误密钥校验失败")
	}
}

// 确保引用 jwt 包以避免未使用 import
var _ = jwt.ErrTokenExpired
