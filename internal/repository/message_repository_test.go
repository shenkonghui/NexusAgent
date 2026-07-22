package repository

import (
	"testing"

	"opennexus/internal/models"
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

// 插入 seq=1..5 共 5 条消息，供分页/限量/按 kind 查询测试共用。
func seedPagingMessages(t *testing.T, repo *MessageRepository) {
	t.Helper()
	for seq := 1; seq <= 5; seq++ {
		kind := models.MessageKindAgentMessageChunk
		if seq == 3 {
			kind = models.MessageKindUsageUpdate
		}
		_ = repo.Create(&models.Message{
			SessionID: "pg-1", DBSessionID: 70, Role: models.MessageRoleAssistant,
			Kind: kind, Content: "m" + itoa(seq), RawJSON: `{"seq":` + itoa(seq) + `}`, Sequence: seq,
		})
	}
}

func TestMessageRepo_FindByDBSessionIDLastN(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)
	seedPagingMessages(t, repo)

	got, err := repo.FindByDBSessionIDLastN(70, 3)
	if err != nil {
		t.Fatalf("FindByDBSessionIDLastN 返回错误: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("期望 3 条，实际 %d", len(got))
	}
	// 必须升序，且为最近 3 条（seq 3,4,5）
	if got[0].Sequence != 3 || got[2].Sequence != 5 {
		t.Errorf("期望升序 [3,4,5]，实际 %d,%d,%d", got[0].Sequence, got[1].Sequence, got[2].Sequence)
	}

	// n 超过总数时返回全部
	all, _ := repo.FindByDBSessionIDLastN(70, 100)
	if len(all) != 5 {
		t.Errorf("n>total 时期望 5 条，实际 %d", len(all))
	}

	// n<=0 返回空切片
	zero, _ := repo.FindByDBSessionIDLastN(70, 0)
	if len(zero) != 0 {
		t.Errorf("n<=0 时期望空，实际 %d", len(zero))
	}
}

func TestMessageRepo_FindByDBSessionIDPaged(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)
	seedPagingMessages(t, repo)

	// 第一页：limit=2, offset=0 → seq 1,2
	page1, err := repo.FindByDBSessionIDPaged(70, 2, 0)
	if err != nil {
		t.Fatalf("FindByDBSessionIDPaged page1 返回错误: %v", err)
	}
	if len(page1) != 2 || page1[0].Sequence != 1 || page1[1].Sequence != 2 {
		t.Errorf("page1 错误: %+v", page1)
	}
	// 第二页：offset=2 → seq 3,4
	page2, _ := repo.FindByDBSessionIDPaged(70, 2, 2)
	if len(page2) != 2 || page2[0].Sequence != 3 || page2[1].Sequence != 4 {
		t.Errorf("page2 错误: %+v", page2)
	}
	// limit<=0 不分页（全量）
	all, _ := repo.FindByDBSessionIDPaged(70, 0, 0)
	if len(all) != 5 {
		t.Errorf("limit<=0 期望全量 5 条，实际 %d", len(all))
	}
}

func TestMessageRepo_FindByKind(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)
	seedPagingMessages(t, repo)

	got, err := repo.FindByKind(70, models.MessageKindUsageUpdate)
	if err != nil {
		t.Fatalf("FindByKind 返回错误: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("期望 1 条 usage_update，实际 %d", len(got))
	}
	if got[0].Sequence != 3 {
		t.Errorf("期望 seq=3，实际 %d", got[0].Sequence)
	}
}

func TestMessageRepo_FindLastByKind(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMessageRepository(db)
	seedPagingMessages(t, repo)

	// 再插一条更新的 usage_update（seq=6）使"最后一条"非唯一
	_ = repo.Create(&models.Message{
		SessionID: "pg-1", DBSessionID: 70, Role: models.MessageRoleAssistant,
		Kind: models.MessageKindUsageUpdate, Content: "u2", RawJSON: `{"seq":6}`, Sequence: 6,
	})

	got, err := repo.FindLastByKind(70, models.MessageKindUsageUpdate)
	if err != nil {
		t.Fatalf("FindLastByKind 返回错误: %v", err)
	}
	if got == nil {
		t.Fatal("期望非 nil")
	}
	if got.Sequence != 6 {
		t.Errorf("期望最后一条 seq=6，实际 %d", got.Sequence)
	}

	// 不存在的 kind 返回 nil, nil
	none, err := repo.FindLastByKind(70, "nonexistent-kind")
	if err != nil {
		t.Fatalf("不存在的 kind 不应返回错误: %v", err)
	}
	if none != nil {
		t.Errorf("期望 nil，实际 %+v", none)
	}
}

// itoa 是避免引入 strconv 仅用于测试的轻量实现。
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
