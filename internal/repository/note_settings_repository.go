package repository

import (
	"errors"

	"gorm.io/gorm"

	"nexusagent/internal/models"
)

var ErrNoteSettingsNotFound = errors.New("笔记设置不存在")

// NoteSettingsRepository 管理笔记分类设置。
type NoteSettingsRepository struct {
	db *gorm.DB
}

func NewNoteSettingsRepository(db *gorm.DB) *NoteSettingsRepository {
	return &NoteSettingsRepository{db: db}
}

// FindByUserID 返回用户的笔记设置，不存在时返回零值记录。
func (r *NoteSettingsRepository) FindByUserID(userID uint) (*models.NoteSettings, error) {
	var s models.NoteSettings
	err := r.db.Where("user_id = ?", userID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.NoteSettings{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Upsert 创建或更新用户笔记设置。
func (r *NoteSettingsRepository) Upsert(s *models.NoteSettings) error {
	var existing models.NoteSettings
	err := r.db.Where("user_id = ?", s.UserID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(s).Error
	}
	if err != nil {
		return err
	}
	s.ID = existing.ID
	s.ClassifySessionID = existing.ClassifySessionID
	s.ClassifyDBSessionID = existing.ClassifyDBSessionID
	return r.db.Save(s).Error
}

// UpdateSessionRef 保存用户的笔记分类任务会话引用。
func (r *NoteSettingsRepository) UpdateSessionRef(userID uint, sessionID string, dbSessionID uint) error {
	var existing models.NoteSettings
	err := r.db.Where("user_id = ?", userID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(&models.NoteSettings{
			UserID:              userID,
			ClassifySessionID:   sessionID,
			ClassifyDBSessionID: dbSessionID,
		}).Error
	}
	if err != nil {
		return err
	}
	existing.ClassifySessionID = sessionID
	existing.ClassifyDBSessionID = dbSessionID
	return r.db.Save(&existing).Error
}
