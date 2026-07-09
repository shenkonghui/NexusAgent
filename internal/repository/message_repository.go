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

// FindByDBSessionIDAfter 查询指定会话中 sequence 大于 afterSeq 的消息，按 sequence 升序排列。
// 用于 SSE 断点续传：客户端携带 Last-Event-ID 重连时，补齐遗漏的消息。
func (r *MessageRepository) FindByDBSessionIDAfter(dbSessionID uint, afterSeq int) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("db_session_id = ? AND sequence > ?", dbSessionID, afterSeq).
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

// ExecutionAggregate 是按 execution_id 聚合的执行块统计。
// StartedAt/FinishedAt 用 string 接收，因为 SQLite 的 MIN/MAX 聚合返回字符串而非 time.Time。
type ExecutionAggregate struct {
	ExecutionID  uint   `gorm:"column:execution_id" json:"execution_id"`
	StartedAt    string `gorm:"column:started_at" json:"started_at"`
	FinishedAt   string `gorm:"column:finished_at" json:"finished_at"`
	MessageCount int    `gorm:"column:message_count" json:"message_count"`
	Status       string `json:"status"` // 来自 TaskExecution 表，运行时合并
	Error        string `json:"error"`  // 来自 TaskExecution 表
}

// AggregateExecutions 按 execution_id 聚合指定会话的执行块，按 started_at 降序（最新优先）。
// 仅统计 execution_id 非空的消息。
func (r *MessageRepository) AggregateExecutions(dbSessionID uint) ([]ExecutionAggregate, error) {
	var list []ExecutionAggregate
	err := r.db.Model(&models.Message{}).
		Select("execution_id, MIN(created_at) AS started_at, MAX(created_at) AS finished_at, COUNT(*) AS message_count").
		Where("db_session_id = ? AND execution_id IS NOT NULL", dbSessionID).
		Group("execution_id").
		Order("started_at DESC").
		Scan(&list).Error
	return list, err
}

// MaxExecutionID 返回指定会话当前最大 execution_id，无定时执行时返回 0。
func (r *MessageRepository) MaxExecutionID(dbSessionID uint) (uint, error) {
	var result *uint
	err := r.db.Model(&models.Message{}).
		Where("db_session_id = ? AND execution_id IS NOT NULL", dbSessionID).
		Select("MAX(execution_id)").
		Scan(&result).Error
	if err != nil {
		return 0, err
	}
	if result == nil {
		return 0, nil
	}
	return *result, nil
}
