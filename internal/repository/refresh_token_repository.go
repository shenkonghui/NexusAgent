package repository

import (
	"errors"

	"gorm.io/gorm"

	"nexusagent/internal/models"
)

var ErrTokenNotFound = errors.New("令牌不存在")

type RefreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(rt *models.RefreshToken) error {
	return r.db.Create(rt).Error
}

func (r *RefreshTokenRepository) FindByJTI(jti string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	if err := r.db.Where("token_id = ?", jti).First(&rt).Error; err != nil {
		return nil, ErrTokenNotFound
	}
	return &rt, nil
}

func (r *RefreshTokenRepository) Revoke(jti string) error {
	return r.db.Model(&models.RefreshToken{}).
		Where("token_id = ?", jti).
		Update("revoked", true).Error
}

func (r *RefreshTokenRepository) RevokeAllByUser(userID uint) error {
	return r.db.Model(&models.RefreshToken{}).
		Where("user_id = ?", userID).
		Update("revoked", true).Error
}
