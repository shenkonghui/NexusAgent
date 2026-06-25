package database

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"nexusagent/internal/models"
)

func Connect(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.RefreshToken{}, &models.Session{}, &models.Message{}, &models.AgentConfig{}, &models.ScheduledTask{}); err != nil {
		return nil, fmt.Errorf("迁移数据库: %w", err)
	}
	return db, nil
}
