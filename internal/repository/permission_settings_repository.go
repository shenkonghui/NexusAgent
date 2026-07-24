package repository

import (
	"errors"

	"gorm.io/gorm"

	"opennexus/internal/models"
)

// ErrPermissionSettingsNotFound 权限设置不存在。
var ErrPermissionSettingsNotFound = errors.New("权限设置不存在")

// PermissionSettingsRepository 管理用户级全局权限规则配置。
type PermissionSettingsRepository struct {
	db *gorm.DB
}

func NewPermissionSettingsRepository(db *gorm.DB) *PermissionSettingsRepository {
	return &PermissionSettingsRepository{db: db}
}

// FindByUserID 返回用户的权限设置，不存在时返回零值记录（mode=normal，列表为空）。
func (r *PermissionSettingsRepository) FindByUserID(userID uint) (*models.PermissionSettings, error) {
	var s models.PermissionSettings
	err := r.db.Where("user_id = ?", userID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.PermissionSettings{UserID: userID, Mode: models.PermissionModeNormal}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Upsert 创建或更新用户权限设置。
func (r *PermissionSettingsRepository) Upsert(s *models.PermissionSettings) error {
	var existing models.PermissionSettings
	err := r.db.Where("user_id = ?", s.UserID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(s).Error
	}
	if err != nil {
		return err
	}
	s.ID = existing.ID
	s.CreatedAt = existing.CreatedAt
	return r.db.Save(s).Error
}

// FindFirst 返回任意一条权限设置记录（单用户场景用于启动时恢复）。
// 不存在时返回 nil。
func (r *PermissionSettingsRepository) FindFirst() (*models.PermissionSettings, error) {
	var s models.PermissionSettings
	err := r.db.First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
