package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"opennexus/internal/models"
)

var ErrNoteSettingsNotFound = errors.New("笔记设置不存在")
var ErrMCPTokenAlreadySet = errors.New("mcp token 已存在")
var ErrMCPTokenNotFound = errors.New("mcp token 无效")

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

// FindByMCPToken 按 MCP Token 查找设置。
func (r *NoteSettingsRepository) FindByMCPToken(token string) (*models.NoteSettings, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrMCPTokenNotFound
	}
	var s models.NoteSettings
	err := r.db.Where("mcp_token = ?", token).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMCPTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindAllWithMcpToken 返回所有已生成 MCP Token 的笔记设置（token 非空）。
func (r *NoteSettingsRepository) FindAllWithMcpToken() ([]models.NoteSettings, error) {
	var list []models.NoteSettings
	err := r.db.Where("mcp_token IS NOT NULL AND mcp_token != ''").Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// SetMCPTokenOnce 为用户生成一次 MCP Token；已存在则拒绝。
func (r *NoteSettingsRepository) SetMCPTokenOnce(userID uint, token string) error {
	s, err := r.FindByUserID(userID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.McpToken) != "" {
		return ErrMCPTokenAlreadySet
	}
	if s.ID == 0 {
		s.UserID = userID
		s.McpToken = token
		return r.db.Create(s).Error
	}
	s.McpToken = token
	return r.db.Save(s).Error
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
	s.McpToken = existing.McpToken
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
