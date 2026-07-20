package repository

import (
	"errors"

	"gorm.io/gorm"

	"opennexus/internal/models"
)

// ErrTaskSettingsNotFound 任务设置不存在。
var ErrTaskSettingsNotFound = errors.New("任务设置不存在")

// TaskSettingsRepository 管理任务自动分类 / 标题生成设置。
type TaskSettingsRepository struct {
	db *gorm.DB
}

func NewTaskSettingsRepository(db *gorm.DB) *TaskSettingsRepository {
	return &TaskSettingsRepository{db: db}
}

// FindByUserID 返回用户的任务设置，不存在时返回零值记录。
func (r *TaskSettingsRepository) FindByUserID(userID uint) (*models.TaskSettings, error) {
	var s models.TaskSettings
	err := r.db.Where("user_id = ?", userID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.TaskSettings{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Upsert 创建或更新用户任务设置。
func (r *TaskSettingsRepository) Upsert(s *models.TaskSettings) error {
	var existing models.TaskSettings
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
