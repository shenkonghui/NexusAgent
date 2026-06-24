package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"nexusagent/internal/models"
)

var ErrSessionNotFound = errors.New("会话不存在")

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(s *models.Session) error {
	return r.db.Create(s).Error
}

func (r *SessionRepository) FindBySessionID(sessionID string) (*models.Session, error) {
	var s models.Session
	if err := r.db.Where("session_id = ?", sessionID).First(&s).Error; err != nil {
		return nil, ErrSessionNotFound
	}
	return &s, nil
}

func (r *SessionRepository) FindByUserID(userID uint) ([]models.Session, error) {
	var sessions []models.Session
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&sessions).Error
	return sessions, err
}

func (r *SessionRepository) UpdateStatus(id uint, status string, closedAt *time.Time) error {
	return r.db.Model(&models.Session{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": status, "closed_at": closedAt}).Error
}

func (r *SessionRepository) UpdateLastPrompt(id uint, prompt string) error {
	return r.db.Model(&models.Session{}).Where("id = ?", id).
		Update("last_prompt", prompt).Error
}

func (r *SessionRepository) MarkActiveAsError() error {
	return r.db.Model(&models.Session{}).
		Where("status = ?", models.SessionStatusActive).
		Update("status", models.SessionStatusError).Error
}

// FindByID 按数据库主键查询会话。
func (r *SessionRepository) FindByID(id uint) (*models.Session, error) {
	var s models.Session
	if err := r.db.First(&s, id).Error; err != nil {
		return nil, ErrSessionNotFound
	}
	return &s, nil
}

// UpdateSessionID 更新会话的 ACP session ID（会话恢复时调用）。
func (r *SessionRepository) UpdateSessionID(id uint, newSessionID string) error {
	return r.db.Model(&models.Session{}).Where("id = ?", id).
		Update("session_id", newSessionID).Error
}
