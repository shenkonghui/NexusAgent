package repository

import (
	"gorm.io/gorm"

	"opennexus/internal/models"
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

// FindByID 按消息主键查询单条消息。
func (r *MessageRepository) FindByID(id uint) (*models.Message, error) {
	var msg models.Message
	err := r.db.First(&msg, id).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// CreateBatch 批量写入消息。
func (r *MessageRepository) CreateBatch(messages []models.Message) error {
	return r.db.Create(&messages).Error
}

// FindByDBSessionID 按数据库会话主键查询全部消息，按 sequence 升序排列。
// 注意：无 LIMIT，对超长会话会一次性载入全部 raw_json 到内存。
// 需要分页或限量时应优先使用 FindByDBSessionIDPaged / FindByDBSessionIDLastN。
func (r *MessageRepository) FindByDBSessionID(dbSessionID uint) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("db_session_id = ?", dbSessionID).
		Order("sequence ASC").
		Find(&messages).Error
	return messages, err
}

// FindByDBSessionIDPaged 分页查询消息，按 sequence 升序。
// limit<=0 时不分页（等价于全量加载）；offset 从 0 开始。
func (r *MessageRepository) FindByDBSessionIDPaged(dbSessionID uint, limit, offset int) ([]models.Message, error) {
	q := r.db.Where("db_session_id = ?", dbSessionID).Order("sequence ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	var messages []models.Message
	return messages, q.Find(&messages).Error
}

// FindByDBSessionIDLastN 返回最近 n 条消息（按 sequence 降序取 n 条再反转为升序）。
// n<=0 返回空切片，避免误用。用于注入历史上下文等仅需近期消息的场景。
func (r *MessageRepository) FindByDBSessionIDLastN(dbSessionID uint, n int) ([]models.Message, error) {
	if n <= 0 {
		return []models.Message{}, nil
	}
	var desc []models.Message
	if err := r.db.Where("db_session_id = ?", dbSessionID).
		Order("sequence DESC").
		Limit(n).
		Find(&desc).Error; err != nil {
		return nil, err
	}
	// 反转为升序，方便调用方按时间顺序消费
	for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
		desc[i], desc[j] = desc[j], desc[i]
	}
	return desc, nil
}

// FindByKind 查询指定会话中给定 kind 的全部消息，按 sequence 升序。
// 用于文件变更等只关心特定 kind 的场景，避免加载无关消息。
func (r *MessageRepository) FindByKind(dbSessionID uint, kind string) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("db_session_id = ? AND kind = ?", dbSessionID, kind).
		Order("sequence ASC").
		Find(&messages).Error
	return messages, err
}

// FindLastByKind 返回指定会话中给定 kind 的最新一条消息（sequence 最大）。
// 无匹配时返回 nil, nil。
func (r *MessageRepository) FindLastByKind(dbSessionID uint, kind string) (*models.Message, error) {
	var msg models.Message
	err := r.db.Where("db_session_id = ? AND kind = ?", dbSessionID, kind).
		Order("sequence DESC").
		First(&msg).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &msg, nil
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

// DeleteFromSequence 删除指定会话中 sequence 大于等于 fromSeq 的全部消息（用于会话回滚，含目标消息）。
func (r *MessageRepository) DeleteFromSequence(dbSessionID uint, fromSeq int) (int64, error) {
	result := r.db.Where("db_session_id = ? AND sequence >= ?", dbSessionID, fromSeq).
		Delete(&models.Message{})
	return result.RowsAffected, result.Error
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
