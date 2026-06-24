package repository

import (
	"testing"
	"time"

	"nexusagent/internal/models"
)

func TestRefreshTokenRepo_CreateAndFindByJTI(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)

	rt := &models.RefreshToken{UserID: 1, TokenID: "jti-1", ExpiresAt: time.Now().Add(time.Hour), Revoked: false}
	if err := repo.Create(rt); err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}

	got, err := repo.FindByJTI("jti-1")
	if err != nil {
		t.Fatalf("FindByJTI 返回错误: %v", err)
	}
	if got.Revoked {
		t.Error("期望未吊销")
	}
}

func TestRefreshTokenRepo_FindByJTI_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)
	if _, err := repo.FindByJTI("missing"); err == nil {
		t.Error("期望未找到返回错误")
	}
}

func TestRefreshTokenRepo_Revoke(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)
	_ = repo.Create(&models.RefreshToken{UserID: 1, TokenID: "jti-2", ExpiresAt: time.Now().Add(time.Hour)})

	if err := repo.Revoke("jti-2"); err != nil {
		t.Fatalf("Revoke 返回错误: %v", err)
	}
	got, _ := repo.FindByJTI("jti-2")
	if !got.Revoked {
		t.Error("期望已吊销")
	}
}

func TestRefreshTokenRepo_RevokeAllByUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)
	_ = repo.Create(&models.RefreshToken{UserID: 7, TokenID: "a", ExpiresAt: time.Now().Add(time.Hour)})
	_ = repo.Create(&models.RefreshToken{UserID: 7, TokenID: "b", ExpiresAt: time.Now().Add(time.Hour)})
	_ = repo.Create(&models.RefreshToken{UserID: 9, TokenID: "c", ExpiresAt: time.Now().Add(time.Hour)})

	if err := repo.RevokeAllByUser(7); err != nil {
		t.Fatalf("RevokeAllByUser 返回错误: %v", err)
	}
	for _, jti := range []string{"a", "b"} {
		got, _ := repo.FindByJTI(jti)
		if !got.Revoked {
			t.Errorf("期望 %s 已吊销", jti)
		}
	}
	c, _ := repo.FindByJTI("c")
	if c.Revoked {
		t.Error("不应吊销其他用户的令牌")
	}
}
