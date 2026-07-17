package database

import (
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"nexusagent/internal/models"
)

func Connect(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.RefreshToken{}, &models.Session{}, &models.Message{}, &models.AgentConfig{}, &models.ScheduledTask{}, &models.TaskExecution{}, &models.Workspace{}, &models.Note{}, &models.NoteSettings{}, &models.RunningTask{}, &models.TaskSettings{}, &models.UserAgentPrefs{}); err != nil {
		return nil, fmt.Errorf("迁移数据库: %w", err)
	}
	// 数据迁移：为旧 Session 创建对应 Workspace，填充 workspace_id
	if err := migrateOldSessionsToWorkspaces(db); err != nil {
		return nil, fmt.Errorf("迁移旧会话数据: %w", err)
	}
	return db, nil
}

func migrateOldSessionsToWorkspaces(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.Session{}).
		Where("workspace_id IS NULL OR workspace_id = 0").
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}

	type userCwd struct {
		UserID uint   `gorm:"column:user_id"`
		Cwd    string `gorm:"column:cwd"`
	}
	var pairs []userCwd
	if err := db.Model(&models.Session{}).
		Select("DISTINCT user_id, cwd").
		Where("workspace_id IS NULL OR workspace_id = 0").
		Find(&pairs).Error; err != nil {
		return err
	}

	for _, p := range pairs {
		ws := models.Workspace{
			UserID: p.UserID,
			Name:   filepath.Base(p.Cwd),
			Cwd:    p.Cwd,
			Mode:   models.WorkspaceModePersistent,
		}
		if err := db.Create(&ws).Error; err != nil {
			return fmt.Errorf("创建 workspace (user=%d, cwd=%s): %w", p.UserID, p.Cwd, err)
		}
		if err := db.Model(&models.Session{}).
			Where("user_id = ? AND cwd = ? AND (workspace_id IS NULL OR workspace_id = 0)", p.UserID, p.Cwd).
			Update("workspace_id", ws.ID).Error; err != nil {
			return fmt.Errorf("更新 session workspace_id: %w", err)
		}
	}

	return createDefaultWorkspacesForEmptyUsers(db)
}

func createDefaultWorkspacesForEmptyUsers(db *gorm.DB) error {
	var userIDs []uint
	if err := db.Model(&models.User{}).Pluck("id", &userIDs).Error; err != nil {
		return err
	}
	for _, uid := range userIDs {
		var count int64
		if err := db.Model(&models.Workspace{}).Where("user_id = ?", uid).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("获取用户主目录: %w", err)
			}
			baseDir := filepath.Join(home, ".nextAgent", "session")
			if err := os.MkdirAll(baseDir, 0o700); err != nil {
				return fmt.Errorf("创建临时根目录: %w", err)
			}
			tempDir, err := os.MkdirTemp(baseDir, "nexus-")
			if err != nil {
				return fmt.Errorf("创建临时目录: %w", err)
			}
			ws := &models.Workspace{
				UserID:  uid,
				Name:    "默认工作区",
				Cwd:     tempDir,
				Mode:    models.WorkspaceModeTemporary,
				TempDir: tempDir,
			}
			if err := db.Create(ws).Error; err != nil {
				return fmt.Errorf("保存默认 workspace: %w", err)
			}
		}
	}
	return nil
}
