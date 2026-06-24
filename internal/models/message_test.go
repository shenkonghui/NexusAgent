package models_test

import (
	"testing"

	"gorm.io/gorm"

	"nexusagent/internal/database"
	"nexusagent/internal/models"
)

func TestMessage_TableExists(t *testing.T) {
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM messages")

	m := &models.Message{
		SessionID:   "acp-test-1",
		DBSessionID: 1,
		Role:        models.MessageRoleUser,
		Kind:        models.MessageKindUserMessageChunk,
		Content:     "你好",
		RawJSON:     `{"sessionUpdate":"user_message_chunk"}`,
		Sequence:    1,
	}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("写入 Message 失败: %v", err)
	}
	if m.ID == 0 {
		t.Error("期望创建后 ID 非零")
	}

	var got models.Message
	if err := db.First(&got, m.ID).Error; err != nil {
		t.Fatalf("查询 Message 失败: %v", err)
	}
	if got.Content != "你好" {
		t.Errorf("Content = %q", got.Content)
	}
}

var _ = gorm.ErrRecordNotFound
