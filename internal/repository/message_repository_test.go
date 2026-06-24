package repository

import (
	"testing"

	"nexusagent/internal/models"
)

func TestMessageRepo_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)

	m := &models.Message{
		SessionID:   "acp-create-1",
		DBSessionID: 10,
		Role:        models.MessageRoleUser,
		Kind:        models.MessageKindUserMessageChunk,
		Content:     "hello",
		RawJSON:     `{"x":1}`,
		Sequence:    1,
	}
	if err := repo.Create(m); err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if m.ID == 0 {
		t.Error("期望创建后 ID 非零")
	}
}

func TestMessageRepo_CreateBatch(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)

	msgs := []models.Message{
		{SessionID: "batch-1", DBSessionID: 20, Role: models.MessageRoleUser, Kind: models.MessageKindUserMessageChunk, Content: "q", RawJSON: "{}", Sequence: 1},
		{SessionID: "batch-1", DBSessionID: 20, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "a", RawJSON: "{}", Sequence: 2},
		{SessionID: "batch-1", DBSessionID: 20, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "b", RawJSON: "{}", Sequence: 3},
	}
	if err := repo.CreateBatch(msgs); err != nil {
		t.Fatalf("CreateBatch 返回错误: %v", err)
	}

	got, err := repo.FindByDBSessionID(20)
	if err != nil {
		t.Fatalf("FindByDBSessionID 返回错误: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("期望 3 条消息，实际 %d", len(got))
	}
}

func TestMessageRepo_FindByDBSessionID_OrderedBySequence(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)

	// 故意乱序插入
	_ = repo.Create(&models.Message{SessionID: "order-1", DBSessionID: 30, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "third", RawJSON: "{}", Sequence: 3})
	_ = repo.Create(&models.Message{SessionID: "order-1", DBSessionID: 30, Role: models.MessageRoleUser, Kind: models.MessageKindUserMessageChunk, Content: "first", RawJSON: "{}", Sequence: 1})
	_ = repo.Create(&models.Message{SessionID: "order-1", DBSessionID: 30, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "second", RawJSON: "{}", Sequence: 2})

	got, err := repo.FindByDBSessionID(30)
	if err != nil {
		t.Fatalf("FindByDBSessionID 返回错误: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("期望 3 条消息，实际 %d", len(got))
	}
	if got[0].Content != "first" || got[1].Content != "second" || got[2].Content != "third" {
		t.Errorf("期望按 sequence 升序排列，实际 %s, %s, %s", got[0].Content, got[1].Content, got[2].Content)
	}
}

func TestMessageRepo_FindByDBSessionID_Empty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)

	got, err := repo.FindByDBSessionID(999)
	if err != nil {
		t.Fatalf("空结果不应返回错误: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("期望 0 条消息，实际 %d", len(got))
	}
}

func TestMessageRepo_DeleteByDBSessionID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)

	_ = repo.Create(&models.Message{SessionID: "del-1", DBSessionID: 40, Role: models.MessageRoleUser, Kind: models.MessageKindUserMessageChunk, Content: "x", RawJSON: "{}", Sequence: 1})
	_ = repo.Create(&models.Message{SessionID: "del-1", DBSessionID: 40, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "y", RawJSON: "{}", Sequence: 2})

	if err := repo.DeleteByDBSessionID(40); err != nil {
		t.Fatalf("DeleteByDBSessionID 返回错误: %v", err)
	}

	got, _ := repo.FindByDBSessionID(40)
	if len(got) != 0 {
		t.Errorf("期望删除后 0 条消息，实际 %d", len(got))
	}
}

func TestMessageRepo_MaxSequence_Empty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)

	max, err := repo.MaxSequence(50)
	if err != nil {
		t.Fatalf("MaxSequence 返回错误: %v", err)
	}
	if max != 0 {
		t.Errorf("空表期望 max=0，实际 %d", max)
	}
}

func TestMessageRepo_MaxSequence(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)

	_ = repo.Create(&models.Message{SessionID: "max-1", DBSessionID: 60, Role: models.MessageRoleUser, Kind: models.MessageKindUserMessageChunk, Content: "a", RawJSON: "{}", Sequence: 5})
	_ = repo.Create(&models.Message{SessionID: "max-1", DBSessionID: 60, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "b", RawJSON: "{}", Sequence: 12})
	_ = repo.Create(&models.Message{SessionID: "max-1", DBSessionID: 60, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "c", RawJSON: "{}", Sequence: 8})

	max, err := repo.MaxSequence(60)
	if err != nil {
		t.Fatalf("MaxSequence 返回错误: %v", err)
	}
	if max != 12 {
		t.Errorf("期望 max=12，实际 %d", max)
	}
}
