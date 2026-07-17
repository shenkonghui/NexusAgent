package repository

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"nexusagent/internal/models"
)

// UserAgentPrefsRepository 管理用户 agent 最近使用偏好。
type UserAgentPrefsRepository struct {
	db *gorm.DB
}

func NewUserAgentPrefsRepository(db *gorm.DB) *UserAgentPrefsRepository {
	return &UserAgentPrefsRepository{db: db}
}

// FindByUserID 返回用户偏好，不存在时返回零值记录。
func (r *UserAgentPrefsRepository) FindByUserID(userID uint) (*models.UserAgentPrefs, error) {
	var s models.UserAgentPrefs
	err := r.db.Where("user_id = ?", userID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.UserAgentPrefs{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Patch 合并更新 last_agent 与某个 agent 的 configs（空字符串删除 category）。
func (r *UserAgentPrefsRepository) Patch(userID uint, lastAgent *string, agentType string, configs map[string]string) (*models.UserAgentPrefs, error) {
	s, err := r.FindByUserID(userID)
	if err != nil {
		return nil, err
	}
	if lastAgent != nil {
		s.LastAgentType = strings.TrimSpace(*lastAgent)
	}
	if agentType = strings.TrimSpace(agentType); agentType != "" && configs != nil {
		prefs := parseAgentPrefsMap(s.PrefsJSON)
		cur := prefs[agentType]
		if cur == nil {
			cur = map[string]string{}
		}
		for k, v := range configs {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			v = strings.TrimSpace(v)
			if v == "" {
				delete(cur, k)
			} else {
				cur[k] = v
			}
		}
		if len(cur) == 0 {
			delete(prefs, agentType)
		} else {
			prefs[agentType] = cur
		}
		b, _ := json.Marshal(prefs)
		s.PrefsJSON = string(b)
	}
	if err := r.upsert(s); err != nil {
		return nil, err
	}
	return r.FindByUserID(userID)
}

func (r *UserAgentPrefsRepository) upsert(s *models.UserAgentPrefs) error {
	var existing models.UserAgentPrefs
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

func parseAgentPrefsMap(raw string) map[string]map[string]string {
	out := map[string]map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return map[string]map[string]string{}
	}
	return out
}
