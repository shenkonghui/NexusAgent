# 会话消息持久化与会话恢复（P4）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 P1/P2/P3 基础上，将 Prompt 产生的每条 SessionUpdate 持久化到 messages 表，支持查询消息历史与会话恢复（重建 ACP session + 注入历史上下文）。

**架构：** 新增 `models.Message` GORM 模型与 `repository.MessageRepository`；新增纯函数 `acp.MapUpdate` 将 `acp.SessionUpdate` 映射为 `models.Message`；扩展 `acp.Service` 的 Prompt 流程增加持久化 goroutine，新增 `ListMessages` / `GetSessionByDBID` / `ResumeSession`；同步更新 `agent.Router` 的 Prompt 返回类型为 `<-chan models.Message` 并新增委托方法。

**技术栈：** Go、GORM、SQLite、`github.com/coder/acp-go-sdk`、内存 SQLite（`file::memory:?cache=shared`）

**对应规格：** `docs/superpowers/specs/2026-06-24-session-management-design.md`

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/models/message.go` | 创建：Message GORM 模型 + role/kind 常量 |
| `internal/database/database.go` | 修改：AutoMigrate 增加 Message |
| `internal/repository/message_repository.go` | 创建：MessageRepository（Create/CreateBatch/FindByDBSessionID/DeleteByDBSessionID/MaxSequence） |
| `internal/repository/message_repository_test.go` | 创建：MessageRepository 测试 |
| `internal/repository/session_repository.go` | 修改：新增 FindByID / UpdateSessionID |
| `internal/repository/session_repository_test.go` | 修改：新增 FindByID / UpdateSessionID 测试 |
| `internal/repository/user_repository_test.go` | 修改：setupTestDB 增加 DELETE FROM messages |
| `internal/acp/update_mapper.go` | 创建：MapUpdate 纯函数（SessionUpdate → Message） |
| `internal/acp/update_mapper_test.go` | 创建：MapUpdate 测试 |
| `internal/acp/service.go` | 修改：新增 messages 字段；Prompt 改返回 `<-chan models.Message`；新增 ListMessages/GetSessionByDBID/ResumeSession/getNextSequence |
| `internal/acp/service_test.go` | 修改：新增 messages 清理 + ListMessages/GetSessionByDBID/ResumeSession 测试 |
| `internal/agent/router.go` | 修改：Prompt 返回类型改为 `<-chan models.Message`；新增 GetSessionByDBID/ListMessages/ResumeSession 委托 |
| `internal/agent/router_test.go` | 修改：适配 Prompt 返回类型变更 |

**包名约定：** 目录名作包名（`models`、`repository`、`acp`、`agent`）。模块名 `nexusagent`，导入路径 `nexusagent/internal/...`。

**测试约定：** 内存 SQLite（DSN `file::memory:?cache=shared`），运行命令 `go test ./...`。

---

## 任务 1：Message 模型与迁移

**文件：**
- 创建：`internal/models/message.go`
- 修改：`internal/database/database.go`
- 修改：`internal/repository/user_repository_test.go`
- 修改：`internal/acp/service_test.go`

- [ ] **步骤 1：创建 Message 模型**

创建 `internal/models/message.go`：

```go
package models

import "time"

// 消息角色常量
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleTool      = "tool"
)

// 消息 kind 常量（对应 ACP SessionUpdate 的 sessionUpdate 判别字段）
const (
	MessageKindUserMessageChunk  = "user_message_chunk"
	MessageKindAgentMessageChunk = "agent_message_chunk"
	MessageKindAgentThoughtChunk = "agent_thought_chunk"
	MessageKindToolCall          = "tool_call"
	MessageKindToolCallUpdate    = "tool_call_update"
	MessageKindPlan              = "plan"
	MessageKindPlanUpdate        = "plan_update"
	MessageKindPlanRemoved       = "plan_removed"
	MessageKindSessionInfoUpdate = "session_info_update"
	MessageKindUsageUpdate       = "usage_update"
	MessageKindCurrentModeUpdate = "current_mode_update"
	MessageKindUnknown           = "unknown"
)

// Message 是会话消息持久化模型，存储 Prompt 产生的每条 SessionUpdate。
type Message struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SessionID   string    `gorm:"index;size:128;not null" json:"session_id"`
	DBSessionID uint      `gorm:"index;not null" json:"db_session_id"`
	Role        string    `gorm:"size:32;not null" json:"role"`
	Kind        string    `gorm:"size:64;not null" json:"kind"`
	Content     string    `gorm:"type:text" json:"content"`
	RawJSON     string    `gorm:"type:text;not null" json:"raw_json"`
	Sequence    int       `gorm:"index;not null" json:"sequence"`
	CreatedAt   time.Time `json:"created_at"`
}
```

- [ ] **步骤 2：修改数据库迁移注册**

修改 `internal/database/database.go`，将 `&models.Message{}` 加入 AutoMigrate。

修改前（第 17 行）：

```go
	if err := db.AutoMigrate(&models.User{}, &models.RefreshToken{}, &models.Session{}); err != nil {
```

修改后：

```go
	if err := db.AutoMigrate(&models.User{}, &models.RefreshToken{}, &models.Session{}, &models.Message{}); err != nil {
```

- [ ] **步骤 3：更新测试辅助函数清理 messages 表**

修改 `internal/repository/user_repository_test.go` 的 `setupTestDB` 函数，在 `db.Exec("DELETE FROM sessions")` 后增加一行：

```go
	db.Exec("DELETE FROM messages")
```

修改 `internal/acp/service_test.go` 的 `setupACPTestDB` 函数，在 `db.Exec("DELETE FROM sessions")` 后增加一行：

```go
	db.Exec("DELETE FROM messages")
```

- [ ] **步骤 4：编写迁移验证测试**

创建 `internal/models/message_test.go`：

```go
package models

import (
	"testing"

	"gorm.io/gorm"

	"nexusagent/internal/database"
)

func TestMessage_TableExists(t *testing.T) {
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM messages")

	m := &Message{
		SessionID:   "acp-test-1",
		DBSessionID: 1,
		Role:        MessageRoleUser,
		Kind:        MessageKindUserMessageChunk,
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

	var got Message
	if err := db.First(&got, m.ID).Error; err != nil {
		t.Fatalf("查询 Message 失败: %v", err)
	}
	if got.Content != "你好" {
		t.Errorf("Content = %q", got.Content)
	}
}

var _ = gorm.ErrRecordNotFound
```

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/models/... ./internal/database/...`
预期：PASS

- [ ] **步骤 6：Commit**

```bash
git add internal/models/message.go internal/models/message_test.go internal/database/database.go internal/repository/user_repository_test.go internal/acp/service_test.go
git commit -m "feat(models): 新增 Message 模型并注册数据库迁移"
```

---

## 任务 2：MessageRepository

**文件：**
- 创建：`internal/repository/message_repository.go`
- 创建：`internal/repository/message_repository_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/repository/message_repository_test.go`：

```go
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
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/repository/... -run TestMessageRepo`
预期：FAIL — `undefined: NewMessageRepository`（函数尚未定义）

- [ ] **步骤 3：编写实现**

创建 `internal/repository/message_repository.go`：

```go
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
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/repository/... -run TestMessageRepo`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add internal/repository/message_repository.go internal/repository/message_repository_test.go
git commit -m "feat(repository): 新增 MessageRepository 实现消息 CRUD"
```

---

## 任务 3：SessionRepository 扩展

**文件：**
- 修改：`internal/repository/session_repository.go`
- 修改：`internal/repository/session_repository_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `internal/repository/session_repository_test.go` 末尾追加以下测试（在 `var _ = gorm.ErrRecordNotFound` 之前）：

```go
func TestSessionRepo_FindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	s := &models.Session{
		SessionID: "find-by-id-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: models.WorkspaceModeExternal,
	}
	_ = repo.Create(s)

	got, err := repo.FindByID(s.ID)
	if err != nil {
		t.Fatalf("FindByID 返回错误: %v", err)
	}
	if got.SessionID != "find-by-id-1" {
		t.Errorf("SessionID = %q", got.SessionID)
	}
}

func TestSessionRepo_FindByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	if _, err := repo.FindByID(99999); err == nil {
		t.Error("期望未找到时返回错误")
	}
}

func TestSessionRepo_UpdateSessionID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSessionRepository(db)
	s := &models.Session{
		SessionID: "old-session-id", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusError, WorkspaceMode: models.WorkspaceModeExternal,
	}
	_ = repo.Create(s)

	if err := repo.UpdateSessionID(s.ID, "new-session-id"); err != nil {
		t.Fatalf("UpdateSessionID 返回错误: %v", err)
	}

	got, err := repo.FindByID(s.ID)
	if err != nil {
		t.Fatalf("FindByID 返回错误: %v", err)
	}
	if got.SessionID != "new-session-id" {
		t.Errorf("SessionID = %q, 期望 new-session-id", got.SessionID)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/repository/... -run "TestSessionRepo_FindByID|TestSessionRepo_UpdateSessionID"`
预期：FAIL — `repo.FindByID undefined` / `repo.UpdateSessionID undefined`

- [ ] **步骤 3：编写实现**

在 `internal/repository/session_repository.go` 的 `MarkActiveAsError` 方法后追加以下两个方法：

```go
// FindByID 按数据库主键查询会话。
func (r *SessionRepository) FindByID(id uint) (*models.Session, error) {
	var s models.Session
	if err := r.db.First(&s, id).Error; err != nil {
		return nil, ErrSessionNotFound
	}
	return &s, nil
}

// UpdateSessionID 更新会话的 ACP session ID（会话恢复时调用）。
func (r *SessionRepository) UpdateSessionID(id uint, newSessionID string) error {
	return r.db.Model(&models.Session{}).Where("id = ?", id).
		Update("session_id", newSessionID).Error
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/repository/...`
预期：PASS（全部测试通过）

- [ ] **步骤 5：Commit**

```bash
git add internal/repository/session_repository.go internal/repository/session_repository_test.go
git commit -m "feat(repository): SessionRepository 新增 FindByID 与 UpdateSessionID 方法"
```

---

## 任务 4：UpdateMapper

**文件：**
- 创建：`internal/acp/update_mapper.go`
- 创建：`internal/acp/update_mapper_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/acp/update_mapper_test.go`：

```go
package acp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"

	"nexusagent/internal/models"
)

func TestMapUpdate_UserMessageChunk(t *testing.T) {
	update := acp.SessionUpdate{
		UserMessageChunk: &acp.SessionUpdateUserMessageChunk{
			Content: acp.ContentBlock{
				Text: &acp.ContentBlockText{Text: "你好世界", Type: "text"},
			},
			SessionUpdate: "user_message_chunk",
		},
	}

	msg := MapUpdate("acp-session-x", 42, 7, update)

	if msg.SessionID != "acp-session-x" {
		t.Errorf("SessionID = %q", msg.SessionID)
	}
	if msg.DBSessionID != 42 {
		t.Errorf("DBSessionID = %d", msg.DBSessionID)
	}
	if msg.Sequence != 7 {
		t.Errorf("Sequence = %d", msg.Sequence)
	}
	if msg.Role != models.MessageRoleUser {
		t.Errorf("Role = %q, 期望 user", msg.Role)
	}
	if msg.Kind != models.MessageKindUserMessageChunk {
		t.Errorf("Kind = %q, 期望 user_message_chunk", msg.Kind)
	}
	if msg.Content != "你好世界" {
		t.Errorf("Content = %q", msg.Content)
	}
	if msg.RawJSON == "" {
		t.Error("RawJSON 不应为空")
	}
	if !strings.Contains(msg.RawJSON, "user_message_chunk") {
		t.Errorf("RawJSON 应包含 sessionUpdate 标识，实际 %s", msg.RawJSON)
	}
}

func TestMapUpdate_AgentMessageChunk(t *testing.T) {
	update := acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.ContentBlock{
				Text: &acp.ContentBlockText{Text: "我是回复", Type: "text"},
			},
			SessionUpdate: "agent_message_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
	if msg.Kind != models.MessageKindAgentMessageChunk {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "我是回复" {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestMapUpdate_AgentMessageChunk_TextNil(t *testing.T) {
	update := acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content:       acp.ContentBlock{},
			SessionUpdate: "agent_message_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Content != "" {
		t.Errorf("Text 为 nil 时 Content 应为空，实际 %q", msg.Content)
	}
}

func TestMapUpdate_AgentThoughtChunk(t *testing.T) {
	update := acp.SessionUpdate{
		AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{
			Content: acp.ContentBlock{
				Text: &acp.ContentBlockText{Text: "思考中...", Type: "text"},
			},
			SessionUpdate: "agent_thought_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
	if msg.Kind != models.MessageKindAgentThoughtChunk {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "思考中..." {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestMapUpdate_ToolCall(t *testing.T) {
	update := acp.SessionUpdate{
		ToolCall: &acp.SessionUpdateToolCall{
			Title:         "执行 grep 搜索",
			ToolCallId:    "tc-1",
			SessionUpdate: "tool_call",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleTool {
		t.Errorf("Role = %q, 期望 tool", msg.Role)
	}
	if msg.Kind != models.MessageKindToolCall {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "执行 grep 搜索" {
		t.Errorf("Content = %q, 期望取 Title", msg.Content)
	}
}

func TestMapUpdate_ToolCallUpdate(t *testing.T) {
	title := "更新后的标题"
	update := acp.SessionUpdate{
		ToolCallUpdate: &acp.SessionToolCallUpdate{
			Title:         &title,
			ToolCallId:    "tc-1",
			SessionUpdate: "tool_call_update",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Kind != models.MessageKindToolCallUpdate {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "更新后的标题" {
		t.Errorf("Content = %q, 期望取 Title", msg.Content)
	}
}

func TestMapUpdate_ToolCallUpdate_TitleNil(t *testing.T) {
	update := acp.SessionUpdate{
		ToolCallUpdate: &acp.SessionToolCallUpdate{
			ToolCallId:    "tc-1",
			SessionUpdate: "tool_call_update",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Content != "" {
		t.Errorf("Title 为 nil 时 Content 应为空，实际 %q", msg.Content)
	}
}

func TestMapUpdate_Plan(t *testing.T) {
	update := acp.SessionUpdate{
		Plan: &acp.SessionUpdatePlan{
			Entries:       []acp.PlanEntry{},
			SessionUpdate: "plan",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
	if msg.Kind != models.MessageKindPlan {
		t.Errorf("Kind = %q", msg.Kind)
	}
	if msg.Content != "" {
		t.Errorf("Plan 的 Content 应为空，实际 %q", msg.Content)
	}
}

func TestMapUpdate_UsageUpdate(t *testing.T) {
	update := acp.SessionUpdate{
		UsageUpdate: &acp.SessionUsageUpdate{
			Size:          1000,
			Used:          500,
			SessionUpdate: "usage_update",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Kind != models.MessageKindUsageUpdate {
		t.Errorf("Kind = %q", msg.Kind)
	}
}

func TestMapUpdate_Unknown(t *testing.T) {
	update := acp.SessionUpdate{}

	msg := MapUpdate("s1", 1, 1, update)

	if msg.Kind != models.MessageKindUnknown {
		t.Errorf("Kind = %q, 期望 unknown", msg.Kind)
	}
	if msg.Role != models.MessageRoleAssistant {
		t.Errorf("Role = %q, 期望 assistant", msg.Role)
	}
}

func TestMapUpdate_RawJSONIsValidJSON(t *testing.T) {
	update := acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content:       acp.ContentBlock{Text: &acp.ContentBlockText{Text: "hi", Type: "text"}},
			SessionUpdate: "agent_message_chunk",
		},
	}

	msg := MapUpdate("s1", 1, 1, update)

	var m map[string]any
	if err := json.Unmarshal([]byte(msg.RawJSON), &m); err != nil {
		t.Fatalf("RawJSON 不是有效 JSON: %v", err)
	}
	if m["sessionUpdate"] != "agent_message_chunk" {
		t.Errorf("RawJSON 中 sessionUpdate 字段 = %v", m["sessionUpdate"])
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/acp/... -run TestMapUpdate`
预期：FAIL — `undefined: MapUpdate`

- [ ] **步骤 3：编写实现**

创建 `internal/acp/update_mapper.go`：

```go
package acp

import (
	"encoding/json"

	"github.com/coder/acp-go-sdk"

	"nexusagent/internal/models"
)

// MapUpdate 将 acp.SessionUpdate 映射为 models.Message 的字段值。
// 检测 SessionUpdate 的哪个变体指针非 nil，提取 kind / role / content / raw_json。
func MapUpdate(sessionID string, dbSessionID uint, seq int, update acp.SessionUpdate) models.Message {
	kind, role := extractKindRole(update)
	content := extractContent(update)
	rawJSON := ""

	data, err := json.Marshal(update)
	if err == nil {
		rawJSON = string(data)
	}

	return models.Message{
		SessionID:   sessionID,
		DBSessionID: dbSessionID,
		Role:        role,
		Kind:        kind,
		Content:     content,
		RawJSON:     rawJSON,
		Sequence:    seq,
	}
}

// extractKindRole 根据 SessionUpdate 的变体指针提取 kind 和 role。
func extractKindRole(update acp.SessionUpdate) (kind, role string) {
	switch {
	case update.UserMessageChunk != nil:
		return models.MessageKindUserMessageChunk, models.MessageRoleUser
	case update.AgentMessageChunk != nil:
		return models.MessageKindAgentMessageChunk, models.MessageRoleAssistant
	case update.AgentThoughtChunk != nil:
		return models.MessageKindAgentThoughtChunk, models.MessageRoleAssistant
	case update.ToolCall != nil:
		return models.MessageKindToolCall, models.MessageRoleTool
	case update.ToolCallUpdate != nil:
		return models.MessageKindToolCallUpdate, models.MessageRoleTool
	case update.Plan != nil:
		return models.MessageKindPlan, models.MessageRoleAssistant
	case update.PlanUpdate != nil:
		return models.MessageKindPlanUpdate, models.MessageRoleAssistant
	case update.PlanRemoved != nil:
		return models.MessageKindPlanRemoved, models.MessageRoleAssistant
	case update.SessionInfoUpdate != nil:
		return models.MessageKindSessionInfoUpdate, models.MessageRoleAssistant
	case update.UsageUpdate != nil:
		return models.MessageKindUsageUpdate, models.MessageRoleAssistant
	case update.CurrentModeUpdate != nil:
		return models.MessageKindCurrentModeUpdate, models.MessageRoleAssistant
	default:
		return models.MessageKindUnknown, models.MessageRoleAssistant
	}
}

// extractContent 从 SessionUpdate 中提取可读文本内容。
// user/agent/thought chunk 取 Content.Text.Text；tool_call 取 Title；tool_call_update 取 Title 指针。
func extractContent(update acp.SessionUpdate) string {
	switch {
	case update.UserMessageChunk != nil:
		if update.UserMessageChunk.Content.Text != nil {
			return update.UserMessageChunk.Content.Text.Text
		}
	case update.AgentMessageChunk != nil:
		if update.AgentMessageChunk.Content.Text != nil {
			return update.AgentMessageChunk.Content.Text.Text
		}
	case update.AgentThoughtChunk != nil:
		if update.AgentThoughtChunk.Content.Text != nil {
			return update.AgentThoughtChunk.Content.Text.Text
		}
	case update.ToolCall != nil:
		return update.ToolCall.Title
	case update.ToolCallUpdate != nil:
		if update.ToolCallUpdate.Title != nil {
			return *update.ToolCallUpdate.Title
		}
	}
	return ""
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/acp/... -run TestMapUpdate`
预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add internal/acp/update_mapper.go internal/acp/update_mapper_test.go
git commit -m "feat(acp): 新增 UpdateMapper 实现 SessionUpdate 到 Message 的映射"
```

---

## 任务 5：Service 扩展

**文件：**
- 修改：`internal/acp/service.go`
- 修改：`internal/acp/service_test.go`

本任务分为多个子步骤。先修改 Service 结构体与构造函数，再改造 Prompt 方法，最后新增方法。

- [ ] **步骤 1：编写失败的测试**

在 `internal/acp/service_test.go` 末尾追加以下测试：

```go
func TestService_GetSessionByDBID(t *testing.T) {
	svc := newTestService(t)
	repo := repository.NewSessionRepository(setupACPTestDB(t))
	_ = repo.Create(&models.Session{
		SessionID: "db-id-test", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: models.WorkspaceModeExternal,
	})
	// 用 svc 自身的 sessions 仓库重新查（因为 newTestService 用的是同一个 db）
	sess, err := svc.GetSession("db-id-test")
	if err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}

	got, err := svc.GetSessionByDBID(sess.ID)
	if err != nil {
		t.Fatalf("GetSessionByDBID 返回错误: %v", err)
	}
	if got.SessionID != "db-id-test" {
		t.Errorf("SessionID = %q", got.SessionID)
	}
}

func TestService_GetSessionByDBID_NotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetSessionByDBID(99999); err == nil {
		t.Error("期望不存在的 DB ID 返回错误")
	}
}

func TestService_ListMessages(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	sess := &models.Session{
		SessionID: "msg-list-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: models.WorkspaceModeExternal,
	}
	_ = repo.Create(sess)

	msgRepo := repository.NewMessageRepository(db)
	_ = msgRepo.Create(&models.Message{SessionID: "msg-list-1", DBSessionID: sess.ID, Role: models.MessageRoleUser, Kind: models.MessageKindUserMessageChunk, Content: "问题", RawJSON: "{}", Sequence: 1})
	_ = msgRepo.Create(&models.Message{SessionID: "msg-list-1", DBSessionID: sess.ID, Role: models.MessageRoleAssistant, Kind: models.MessageKindAgentMessageChunk, Content: "回答", RawJSON: "{}", Sequence: 2})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
	msgs, err := svc.ListMessages("msg-list-1")
	if err != nil {
		t.Fatalf("ListMessages 返回错误: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息，实际 %d", len(msgs))
	}
	if msgs[0].Content != "问题" {
		t.Errorf("第一条消息 Content = %q", msgs[0].Content)
	}
	if msgs[1].Content != "回答" {
		t.Errorf("第二条消息 Content = %q", msgs[1].Content)
	}
}

func TestService_ListMessages_SessionNotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.ListMessages("nonexistent"); err == nil {
		t.Error("期望不存在的会话返回错误")
	}
}

func TestService_ListMessages_Empty(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	_ = repo.Create(&models.Session{
		SessionID: "empty-msg-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusActive, WorkspaceMode: models.WorkspaceModeExternal,
	})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
	msgs, err := svc.ListMessages("empty-msg-1")
	if err != nil {
		t.Fatalf("ListMessages 返回错误: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("期望 0 条消息，实际 %d", len(msgs))
	}
}

func TestService_ResumeSession_Closed(t *testing.T) {
	db := setupACPTestDB(t)
	repo := repository.NewSessionRepository(db)
	_ = repo.Create(&models.Session{
		SessionID: "closed-resume-1", AgentType: "claude-code", Cwd: "/tmp",
		Status: models.SessionStatusClosed, WorkspaceMode: models.WorkspaceModeExternal,
	})

	svc := NewService(db, config.WorkspaceConfig{DefaultMode: "external"})
	_, err := svc.ResumeSession(context.Background(), "closed-resume-1")
	if err == nil {
		t.Error("期望 closed 会话恢复返回错误")
	}
}

func TestService_ResumeSession_SessionNotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.ResumeSession(context.Background(), "nonexistent"); err == nil {
		t.Error("期望不存在的会话恢复返回错误")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/acp/... -run "TestService_GetSessionByDBID|TestService_ListMessages|TestService_ResumeSession"`
预期：FAIL — `svc.GetSessionByDBID undefined` / `svc.ListMessages undefined` / `svc.ResumeSession undefined`

- [ ] **步骤 3：修改 Service 结构体与构造函数**

修改 `internal/acp/service.go`。

在 error 变量块中新增 `ErrSessionClosed`（在 `ErrSessionNotActive` 后）：

修改前：

```go
var (
	ErrBackendNotFound  = errors.New("后端未注册")
	ErrSessionNotFound  = errors.New("会话不存在")
	ErrSessionNotActive = errors.New("会话不在活跃状态")
)
```

修改后：

```go
var (
	ErrBackendNotFound  = errors.New("后端未注册")
	ErrSessionNotFound  = errors.New("会话不存在")
	ErrSessionNotActive = errors.New("会话不在活跃状态")
	ErrSessionClosed    = errors.New("会话已关闭，无法恢复")
)
```

在 `Service` 结构体中新增 `messages` 字段（在 `sessions` 字段后）：

修改前：

```go
type Service struct {
	sessions *repository.SessionRepository
	backends map[string]Backend
	conns    map[string]*Connection
	mu       sync.RWMutex
	wsConfig config.WorkspaceConfig
}
```

修改后：

```go
type Service struct {
	sessions *repository.SessionRepository
	messages *repository.MessageRepository
	backends map[string]Backend
	conns    map[string]*Connection
	mu       sync.RWMutex
	wsConfig config.WorkspaceConfig
}
```

在 `NewService` 构造函数中初始化 `messages`：

修改前：

```go
func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig) *Service {
	return &Service{
		sessions: repository.NewSessionRepository(db),
		backends: make(map[string]Backend),
		conns:    make(map[string]*Connection),
		wsConfig: wsConfig,
	}
}
```

修改后：

```go
func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig) *Service {
	return &Service{
		sessions: repository.NewSessionRepository(db),
		messages: repository.NewMessageRepository(db),
		backends: make(map[string]Backend),
		conns:    make(map[string]*Connection),
		wsConfig: wsConfig,
	}
}
```

- [ ] **步骤 4：改造 Prompt 方法**

将 `internal/acp/service.go` 中整个 `Prompt` 方法替换为以下实现。

在 import 块中新增 `"encoding/json"` 和 `"github.com/coder/acp-go-sdk"`（如果尚未导入）。

修改前：

```go
// Prompt 向会话发送 prompt，返回流式 update channel。
func (s *Service) Prompt(ctx context.Context, sessionID, prompt string) (<-chan interface{}, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.SessionStatusActive {
		return nil, ErrSessionNotActive
	}

	s.mu.RLock()
	conn, ok := s.conns[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotActive
	}

	updates, err := conn.Prompt(ctx, sessionID, prompt)
	if err != nil {
		return nil, err
	}

	_ = s.sessions.UpdateLastPrompt(session.ID, prompt)

	out := make(chan interface{}, 256)
	go func() {
		defer close(out)
		for u := range updates {
			out <- u
		}
	}()

	return out, nil
}
```

修改后：

```go
// Prompt 向会话发送 prompt，返回流式 Message channel。
// 每条 SessionUpdate 会被映射为 models.Message 并持久化到数据库后转发给调用方。
func (s *Service) Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.SessionStatusActive {
		return nil, ErrSessionNotActive
	}

	s.mu.RLock()
	conn, ok := s.conns[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotActive
	}

	updates, err := conn.Prompt(ctx, sessionID, prompt)
	if err != nil {
		return nil, err
	}

	_ = s.sessions.UpdateLastPrompt(session.ID, prompt)

	out := make(chan models.Message, 256)
	go func() {
		defer close(out)

		seq := s.getNextSequence(session.ID)

		// 持久化用户发送的 prompt 作为 user_message_chunk
		seq++
		userUpdate := acp.SessionUpdate{
			UserMessageChunk: &acp.SessionUpdateUserMessageChunk{
				Content: acp.ContentBlock{
					Text: &acp.ContentBlockText{Text: prompt, Type: "text"},
				},
				SessionUpdate: "user_message_chunk",
			},
		}
		userMsg := MapUpdate(sessionID, session.ID, seq, userUpdate)
		_ = s.messages.Create(&userMsg)
		out <- userMsg

		// 读取 agent 返回的 update 流，逐条持久化并转发
		for u := range updates {
			seq++
			msg := MapUpdate(sessionID, session.ID, seq, u)
			_ = s.messages.Create(&msg)
			out <- msg
		}
	}()

	return out, nil
}
```

更新 `internal/acp/service.go` 的 import 块，确保包含 `encoding/json`（在步骤 5 的 ResumeSession 中使用）和 `github.com/coder/acp-go-sdk`：

修改前：

```go
import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"nexusagent/internal/config"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)
```

修改后：

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
	"gorm.io/gorm"

	"nexusagent/internal/config"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)
```

- [ ] **步骤 5：新增辅助方法与公开方法**

在 `internal/acp/service.go` 的 `RecoverActiveSessions` 方法后追加以下方法：

```go
// getNextSequence 获取指定会话当前最大 sequence 值（无消息时返回 0）。
func (s *Service) getNextSequence(dbSessionID uint) int {
	max, err := s.messages.MaxSequence(dbSessionID)
	if err != nil {
		return 0
	}
	return max
}

// ListMessages 查询会话的完整消息历史，按 sequence 升序返回。
func (s *Service) ListMessages(sessionID string) ([]models.Message, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return s.messages.FindByDBSessionID(session.ID)
}

// GetSessionByDBID 按数据库主键查询会话。
func (s *Service) GetSessionByDBID(id uint) (*models.Session, error) {
	sess, err := s.sessions.FindByID(id)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// ResumeSession 恢复已失效（error 状态）的会话：重建 ACP session、注入历史上下文、更新 session_id。
// active 且连接存在的会话直接返回；closed 会话返回错误。
func (s *Service) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// active 且连接存在 → 直接返回
	if session.Status == models.SessionStatusActive {
		s.mu.RLock()
		_, ok := s.conns[sessionID]
		s.mu.RUnlock()
		if ok {
			return session, nil
		}
	}

	// closed → 不可恢复
	if session.Status == models.SessionStatusClosed {
		return nil, ErrSessionClosed
	}

	// error 状态 → 尝试恢复
	backend, err := s.GetBackend(session.AgentType)
	if err != nil {
		return nil, err
	}

	conn, err := NewConnection(backend)
	if err != nil {
		return nil, fmt.Errorf("恢复会话-建立连接: %w", err)
	}

	if _, err := conn.Initialize(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-ACP 握手: %w", err)
	}

	newSessionID, err := conn.NewSession(ctx, session.Cwd)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-创建 ACP 会话: %w", err)
	}

	// 查询历史消息并注入上下文
	history, _ := s.messages.FindByDBSessionID(session.ID)
	contextText := formatHistory(history)
	if contextText != "" {
		// 异步注入历史上下文，不等结果
		go func() {
			_, _ = conn.Prompt(ctx, newSessionID, contextText)
		}()
	}

	// 更新 session_id 和状态
	if err := s.sessions.UpdateSessionID(session.ID, newSessionID); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-更新 session_id: %w", err)
	}
	if err := s.sessions.UpdateStatus(session.ID, models.SessionStatusActive, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("恢复会话-更新状态: %w", err)
	}

	// 存入连接池
	s.mu.Lock()
	s.conns[newSessionID] = conn
	s.mu.Unlock()

	// 返回更新后的 session
	return s.sessions.FindByID(session.ID)
}

// formatHistory 将历史消息格式化为对话上下文文本，最多取最近 50 条。
func formatHistory(messages []models.Message) string {
	if len(messages) == 0 {
		return ""
	}

	// 最多取最近 50 条
	const limit = 50
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	var sb strings.Builder
	sb.WriteString("以下是之前对话的历史记录，请基于这些上下文继续对话：\n\n")
	for _, m := range messages {
		switch m.Role {
		case models.MessageRoleUser:
			sb.WriteString("[User]: " + m.Content + "\n")
		case models.MessageRoleAssistant:
			sb.WriteString("[Assistant]: " + m.Content + "\n")
		case models.MessageRoleTool:
			sb.WriteString("[Tool]: " + m.Content + "\n")
		}
	}
	return sb.String()
}
```

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./internal/acp/...`
预期：PASS（全部测试通过，包括新增的 GetSessionByDBID / ListMessages / ResumeSession 测试）

> **注：** `ResumeSession` 对 error 状态会话的完整恢复路径（成功重建 ACP session）需要真实 agent 进程，无法在单元测试中覆盖。当前测试覆盖 closed 会话返回错误、不存在会话返回错误。error 状态的完整恢复路径需在集成测试中验证（需真实 claude-code 可执行文件）。

- [ ] **步骤 7：Commit**

```bash
git add internal/acp/service.go internal/acp/service_test.go
git commit -m "feat(acp): Service 扩展消息持久化、ListMessages、GetSessionByDBID 与 ResumeSession"
```

---

## 任务 6：Router 同步

**文件：**
- 修改：`internal/agent/router.go`
- 修改：`internal/agent/router_test.go`

- [ ] **步骤 1：修改 Router.Prompt 返回类型**

修改 `internal/agent/router.go` 的 `Prompt` 方法返回类型。

修改前：

```go
// Prompt 发送 prompt，委托 service。
func (r *Router) Prompt(ctx context.Context, sessionID, prompt string) (<-chan interface{}, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.Prompt(ctx, sessionID, prompt)
}
```

修改后：

```go
// Prompt 发送 prompt，委托 service。返回流式 Message channel。
func (r *Router) Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.Prompt(ctx, sessionID, prompt)
}
```

- [ ] **步骤 2：新增 Router 委托方法**

在 `internal/agent/router.go` 的 `ListAgents` 方法前追加以下委托方法：

```go
// GetSessionByDBID 按数据库主键查询会话，委托 service。
func (r *Router) GetSessionByDBID(id uint) (*models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.GetSessionByDBID(id)
}

// ListMessages 查询会话消息历史，委托 service。
func (r *Router) ListMessages(sessionID string) ([]models.Message, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ListMessages(sessionID)
}

// ResumeSession 恢复会话，委托 service。
func (r *Router) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ResumeSession(ctx, sessionID)
}
```

- [ ] **步骤 3：检查 router_test.go 是否需要适配**

现有 `internal/agent/router_test.go` 中没有直接调用 `Prompt` 方法的测试，因此无需修改现有测试。

在 `internal/agent/router_test.go` 末尾追加以下测试验证新方法在 service 为 nil 时返回错误：

```go
func TestRouter_NewMethods_NilService(t *testing.T) {
	r := NewRegistry()
	router := NewRouter(r, nil)

	if _, err := router.GetSessionByDBID(1); err == nil {
		t.Error("期望 GetSessionByDBID 在 service 为 nil 时返回错误")
	}
	if _, err := router.ListMessages("x"); err == nil {
		t.Error("期望 ListMessages 在 service 为 nil 时返回错误")
	}
	if _, err := router.ResumeSession(nil, "x"); err == nil {
		t.Error("期望 ResumeSession 在 service 为 nil 时返回错误")
	}
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/agent/...`
预期：PASS

- [ ] **步骤 5：运行全量测试**

运行：`go test ./...`
预期：PASS（全部测试通过，确认 Prompt 返回类型变更未破坏任何调用方）

- [ ] **步骤 6：Commit**

```bash
git add internal/agent/router.go internal/agent/router_test.go
git commit -m "feat(agent): Router 同步 Prompt 返回类型为 Message 并新增会话恢复委托方法"
```

---

## 完成检查清单

- [ ] `go test ./...` 全部通过
- [ ] `go vet ./...` 无警告
- [ ] Message 模型字段与 spec 5.1 对齐
- [ ] UpdateMapper 覆盖全部 kind 变体
- [ ] MessageRepository CRUD 全部覆盖
- [ ] Service.Prompt 返回 `<-chan models.Message`
- [ ] ResumeSession 对 active（连接存在）/closed/不存在 的路径已覆盖
- [ ] Router.Prompt 与 Service.Prompt 返回类型一致
- [ ] 数据库迁移包含 Message 表
