package repository

import (
	"gorm.io/gorm"

	"nexusagent/internal/models"
)

// MessageRepository 是消息持久化仓库，提供消息 CRUD 操作。
type MessageRepository struct {
	db *gorm.DB
}

// NewMessageRepository 创建新的 MessageRepository。
func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// Create 写入单条消息。
func (r *MessageRepository) Create(m *models.Message) error {
	return r.db.Create(m).Error
}

// CreateBatch 批量写入消息。
func (r *MessageRepository) CreateBatch(messages []models.Message) error {
	return r.db.Create(&messages).Error
}

// FindByDBSessionID 按数据库会话主键查询全部消息，按 sequence 升序排列。
func (r *MessageRepository) FindByDBSessionID(dbSessionID uint) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("db_session_id = ?", dbSessionID).
		Order("sequence ASC").
		Find(&messages).Error
	return messages, err
}

// DeleteByDBSessionID 删除指定会话的全部消息。
func (r *MessageRepository) DeleteByDBSessionID(dbSessionID uint) error {
	return r.db.Where("db_session_id = ?", dbSessionID).
		Delete(&models.Message{}).Error
}

// MaxSequence 查询指定会话当前最大 sequence 值，无消息时返回 0。
func (r *MessageRepository) MaxSequence(dbSessionID uint) (int, error) {
	var result *int
	err := r.db.Model(&models.Message{}).
		Where("db_session_id = ?", dbSessionID).
		Select("MAX(sequence)").
		Scan(&result).Error
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, nil
	}
	return *result, nil
}
