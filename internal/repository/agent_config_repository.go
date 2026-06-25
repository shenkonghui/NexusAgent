package repository

import (
	"errors"

	"gorm.io/gorm"

	"nexusagent/internal/models"
)

var ErrAgentConfigNotFound = errors.New("agent 配置不存在")

// AgentConfigRepository 管理 AgentConfig 的持久化。
type AgentConfigRepository struct {
	db *gorm.DB
}

func NewAgentConfigRepository(db *gorm.DB) *AgentConfigRepository {
	return &AgentConfigRepository{db: db}
}

// FindAll 返回全部 agent 配置，按 ID 升序。
func (r *AgentConfigRepository) FindAll() ([]models.AgentConfig, error) {
	var list []models.AgentConfig
	err := r.db.Order("id ASC").Find(&list).Error
	return list, err
}

// FindAllEnabled 返回所有启用的 agent 配置。
func (r *AgentConfigRepository) FindAllEnabled() ([]models.AgentConfig, error) {
	var list []models.AgentConfig
	err := r.db.Where("enabled = ?", true).Order("id ASC").Find(&list).Error
	return list, err
}

// FindByID 按主键查询。
func (r *AgentConfigRepository) FindByID(id uint) (*models.AgentConfig, error) {
	var cfg models.AgentConfig
	if err := r.db.First(&cfg, id).Error; err != nil {
		return nil, ErrAgentConfigNotFound
	}
	return &cfg, nil
}

// FindByType 按类型查询。
func (r *AgentConfigRepository) FindByType(agentType string) (*models.AgentConfig, error) {
	var cfg models.AgentConfig
	if err := r.db.Where("type = ?", agentType).First(&cfg).Error; err != nil {
		return nil, ErrAgentConfigNotFound
	}
	return &cfg, nil
}

// Create 落库。
func (r *AgentConfigRepository) Create(cfg *models.AgentConfig) error {
	return r.db.Create(cfg).Error
}

// Update 更新全部字段。
func (r *AgentConfigRepository) Update(cfg *models.AgentConfig) error {
	return r.db.Save(cfg).Error
}

// Delete 按主键删除。
func (r *AgentConfigRepository) Delete(id uint) error {
	return r.db.Delete(&models.AgentConfig{}, id).Error
}
