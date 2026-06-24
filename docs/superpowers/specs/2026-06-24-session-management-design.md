# NexusAgent 会话消息持久化与会话恢复（P4）设计文档

- 日期：2026-06-24
- 子项目：P4 — 会话消息持久化与会话恢复
- 状态：待审查

## 1. 背景与定位

NexusAgent 是一个用 Go 开发的全栈平台，目标是通过 ACP（Agent Client Protocol）协议调用 Claude Code、Codex 等编码 agent 执行任务，并支持用户认证。

整个平台被拆分为 6 个可独立设计的子项目：

| # | 子项目 | 依赖 |
|----|--------|------|
| P1 | ACP 客户端核心库（已完成） | 无 |
| P2 | Agent 注册与编排层（已完成） | P1 |
| P3 | 用户认证系统（已完成） | 无 |
| P4 | 会话消息持久化与会话恢复（本文档） | P1, P2 |
| P5 | REST API 接入层 | P2, P3, P4 |
| P6 | Web UI | P5 |

本文档覆盖 **P4 会话消息持久化与会话恢复**。P1 已实现会话 CRUD（创建/查询/关闭）与 Prompt 流式推送，但 Prompt 产生的 SessionUpdate 流不持久化，服务重启后会话无法恢复。P4 在 P1 基础上补充：消息历史持久化 + 会话恢复（重建 ACP session + 注入历史上下文）。

P4 复用 P1/P2/P3 的分层架构（config/database/models/repository）和技术栈（Go + GORM + SQLite）。

**设计结论**：Task = Session（不引入独立 Task 表）。一次会话即一个任务，用户在同一会话内可多轮 Prompt。

## 2. 需求摘要

- 将 Prompt 产生的每条 SessionUpdate 持久化到数据库，保留原始 JSON 与可读文本。
- 支持查询会话的完整消息历史。
- 支持会话恢复：对已失效（error/closed）的会话，重启 agent 进程、创建新 ACP session，并将历史消息作为上下文注入，使对话得以延续。
- 会话恢复后，session 记录的 `session_id` 更新为新的 ACP session ID，状态恢复为 active。

## 3. 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go（最新稳定版） |
| ORM | GORM（复用 P1/P3） |
| 数据库 | SQLite（复用 P1/P3） |
| JSON | `encoding/json`（序列化 SessionUpdate） |
| ACP SDK | `github.com/coder/acp-go-sdk`（复用 P1） |

## 4. 架构与目录结构

```
internal/
  acp/
    service.go          # 修改：Prompt 流程增加消息持久化；新增 ResumeSession / ListMessages
    update_mapper.go    # 创建：SessionUpdate → Message 的映射逻辑（提取 kind/role/content/raw_json）
    service_test.go     # 修改：新增消息持久化、ResumeSession、ListMessages 测试
    update_mapper_test.go # 创建：映射逻辑测试
  models/
    message.go          # 创建：Message GORM 模型
  repository/
    message_repository.go     # 创建：消息仓库 CRUD
    message_repository_test.go # 创建：消息仓库测试
    session_repository.go     # 修改：新增 UpdateSessionID 方法（恢复时替换 session_id）
```

职责边界：

- `acp.Service` 扩展：Prompt 时增加持久化 goroutine；新增 ResumeSession（重建会话+注入历史）；新增 ListMessages（查历史）。
- `acp.update_mapper`：纯函数模块，将 `acp.SessionUpdate` 映射为 `models.Message`，可独立测试。
- `models.Message`：消息持久化模型。
- `repository.MessageRepository`：消息 CRUD，不含业务逻辑。

## 5. 数据模型

### 5.1 `messages` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | uint | PK，自增 | 主键 |
| `session_id` | string | 非空，索引 | 关联 sessions.session_id（ACP 会话 ID） |
| `db_session_id` | uint | 非空，索引 | 关联 sessions.id（DB 主键） |
| `role` | string | 非空 | 枚举：`user` / `assistant` / `tool` |
| `kind` | string | 非空 | ACP sessionUpdate 类型（见 5.2） |
| `content` | text | | 可读文本内容（text 类型的消息正文，其他类型为空或摘要） |
| `raw_json` | text | 非空 | 原始 SessionUpdate 的 JSON 序列化 |
| `sequence` | int | 非空，索引 | 会话内序号（从 1 递增，排序用） |
| `created_at` | time | | 创建时间 |

设计要点：

- `session_id` 关联 ACP 协议层的会话标识。会话恢复后 session_id 会变，但历史消息保留旧 session_id 不变（仍可通过 `db_session_id` 关联到 DB 主键）。
- `db_session_id` 关联 sessions 表主键，恢复后不变，是查询消息历史的稳定关联键。
- `sequence` 保证消息顺序。每次 Prompt 的 update 流按到达顺序递增。
- `raw_json` 完整保留原始数据，支持前端精确回放或后续扩展。

### 5.2 `kind` 枚举值

来自 ACP SDK `SessionUpdate` 的 `sessionUpdate` 判别字段：

| kind | 对应 ACP 类型 | role |
|------|--------------|------|
| `user_message_chunk` | `SessionUpdateUserMessageChunk` | `user` |
| `agent_message_chunk` | `SessionUpdateAgentMessageChunk` | `assistant` |
| `agent_thought_chunk` | `SessionUpdateAgentThoughtChunk` | `assistant` |
| `tool_call` | `SessionUpdateToolCall` | `tool` |
| `tool_call_update` | `SessionToolCallUpdate` | `tool` |
| `plan` | `SessionUpdatePlan` | `assistant` |
| `plan_update` | `SessionPlanUpdate` | `assistant` |
| `plan_removed` | `SessionUpdatePlanRemoved` | `assistant` |
| `session_info_update` | `SessionSessionInfoUpdate` | `assistant` |
| `usage_update` | `SessionUsageUpdate` | `assistant` |
| `current_mode_update` | `SessionCurrentModeUpdate` | `assistant` |
| `unknown` | 未知类型 | `assistant` |

### 5.3 `content` 提取规则

- `user_message_chunk` / `agent_message_chunk` / `agent_thought_chunk`：提取 `Content.Text.Text`（ContentBlock 的 Text 变体的文本内容）。若 Text 为 nil，content 为空字符串。
- `tool_call`：取 `Title` 字段。
- `tool_call_update`：取 `Title` 字段。
- 其他类型：content 为空字符串。完整数据在 `raw_json` 中。

## 6. 核心组件

### 6.1 UpdateMapper — SessionUpdate 映射

```go
// MapUpdate 将 acp.SessionUpdate 映射为 Message 的字段值。
func MapUpdate(sessionID string, dbSessionID uint, seq int, update acp.SessionUpdate) models.Message
```

纯函数：接收 SessionUpdate，检测哪个变体指针非 nil，提取 kind / role / content / raw_json。

`raw_json` 通过 `json.Marshal(update)` 序列化整个 SessionUpdate 获得。

### 6.2 MessageRepository — 消息仓库

```go
type MessageRepository struct {
    db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) *MessageRepository
func (r *MessageRepository) Create(m *models.Message) error
func (r *MessageRepository) CreateBatch(messages []models.Message) error          // 批量写入
func (r *MessageRepository) FindByDBSessionID(dbSessionID uint) ([]models.Message, error)  // 按会话查全部，按 sequence 排序
func (r *MessageRepository) DeleteByDBSessionID(dbSessionID uint) error           // 删除会话全部消息
```

查询按 `db_session_id` 而非 `session_id`，因为恢复后 session_id 会变，但 db_session_id（DB 主键）不变。

### 6.3 SessionRepository 扩展

新增方法：

```go
func (r *SessionRepository) UpdateSessionID(id uint, newSessionID string) error
```

会话恢复时调用，将 sessions 表的 `session_id` 更新为新的 ACP session ID。

### 6.4 Service 扩展

Service 结构体新增 `messages *repository.MessageRepository` 字段。`NewService` 构造函数内部创建 `MessageRepository`（与 `SessionRepository` 一样从 `db` 创建，无需改签名）。

#### Prompt 流程改造

现有 `Prompt` 方法在收到 update channel 后，用一个 goroutine 转发给调用方。改造后，该 goroutine 同时持久化每条 update：

返回类型从 `<-chan interface{}` 改为 `<-chan models.Message`，让调用方（P5 handler）直接拿到已映射的 Message（含持久化后的 id/sequence）：

```go
func (s *Service) Prompt(ctx context.Context, sessionID, prompt string) (<-chan models.Message, error)
```

> 此签名变更影响 `agent.Router.Prompt` 和 `agent.Router`（P2），需同步更新返回类型。

流程：

1. 查 session → 校验 active → 获取 conn（同现有逻辑）。
2. 调用 `conn.Prompt` 获取 update channel。
3. 更新 last_prompt（同现有逻辑）。
4. **新增**：启动持久化 goroutine，从 update channel 读取每条 update：
   - 通过 `MapUpdate` 映射为 Message。
   - 写入 MessageRepository（含 sequence 自增）。
   - 同时转发到返回给调用方的 out channel。
5. 将用户发送的 prompt 本身也作为一条 `user_message_chunk` 类型的 Message 持久化（sequence=该会话当前最大值+1）。

sequence 管理：每次 Prompt 开始时，查询该 session 当前最大 sequence，从 +1 开始递增。

#### ResumeSession — 会话恢复

```go
func (s *Service) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error)
```

流程：

1. 查 DB 获取 session。若 status 为 `active` 且 conn 存在，直接返回（无需恢复）。
2. 若 status 为 `closed`，返回错误（已显式关闭的会话不恢复）。
3. 获取 backend（通过 session.AgentType）。
4. 创建新 Connection（NewConnection → Initialize → NewSession(cwd)）。
5. 获取新 ACP session ID。
6. 查询历史消息（通过 `db_session_id`，排除 user_message_chunk 中的注入标记）。
7. 将历史消息格式化为对话上下文文本：
   ```
   以下是之前对话的历史记录，请基于这些上下文继续对话：

   [User]: <content>
   [Assistant]: <content>
   [Tool: <title>]: <content>
   ...
   ```
8. 将上下文作为首条 Prompt 发送给新 session（异步，不等结果）。
9. 更新 sessions 表：`UpdateSessionID(session.ID, newSessionID)`，`UpdateStatus(session.ID, active, nil)`。
10. 存入 conns 连接池（以新 sessionID 为 key）。
11. 返回更新后的 session（含新 session_id）。

历史注入限制：为避免上下文过长，最多注入最近 50 条消息（按 sequence 倒序取最后 50 条再正序排列）。

#### ListMessages — 查询消息历史

```go
func (s *Service) ListMessages(sessionID string) ([]models.Message, error)
```

通过 sessionID 查 DB 获取 session，再用 `db_session_id` 查 messages 表，按 sequence 升序返回。

#### GetSessionByDBID — 按 DB 主键查询（供 P5 路由使用）

```go
func (s *Service) GetSessionByDBID(id uint) (*models.Session, error)
```

SessionRepository 新增 `FindByID(id uint)` 方法支持此功能。用于 P5 REST API 以 DB 主键 `:id` 作为路由参数。

## 7. 配置

无新增配置项。P4 使用 P1 已有的 `agents.workspace` 配置。

## 8. 测试策略

### UpdateMapper 测试（`update_mapper_test.go`）

- `user_message_chunk`：验证 role=user、kind 正确、content 提取文本、raw_json 非空。
- `agent_message_chunk`：验证 role=assistant、content 提取。
- `tool_call`：验证 role=tool、content=title。
- 未知类型：验证 kind=unknown、role=assistant。

### MessageRepository 测试（`message_repository_test.go`）

- Create + FindByDBSessionID：验证写入和按 sequence 排序查询。
- CreateBatch：验证批量写入。
- DeleteByDBSessionID：验证删除。
- FindByDBSessionID 空结果。

### Service 测试（`service_test.go` 扩展）

- ListMessages：验证查询返回消息列表。
- ListMessages 不存在的会话：返回错误。
- ResumeSession 对 active 会话：直接返回不重建。
- ResumeSession 对 closed 会话：返回错误。
- ResumeSession 对 error 会话：验证 session_id 更新、status 恢复 active、历史注入（验证 conn 存在于 conns）。

> 注：ResumeSession 的完整测试需要 MockBackend 能走通 ACP 握手，当前 P1 的 MockBackend 仅 echo 进程不足以支持。这些测试依赖集成测试或改进 MockBackend。P4 单元测试覆盖到"error 会话调用 ResumeSession 时正确创建 Connection 并更新 DB"这一层面（使用 MockBackend，即使 ACP 握手失败也能验证错误处理路径）。

### 覆盖目标

- UpdateMapper 映射逻辑 100% 覆盖。
- MessageRepository CRUD 100% 覆盖。
- ListMessages 查询 100% 覆盖。
- ResumeSession 错误路径（active/closed 会话的处理）100% 覆盖。

## 9. 范围边界（不做）

- 不实现 HTTP handler / REST API（P5 负责）。
- 不实现前端 UI（P6 负责）。
- 不实现 Task 独立抽象（Task = Session，不额外建表）。
- 不实现消息全文搜索（按 sequence 查询即可，全文搜索留待未来）。
- 不实现消息编辑/删除（只支持按会话批量删除）。
- 不实现历史注入的智能摘要（直接拼接文本，不做 AI 摘要）。
- 不保证恢复后 agent 完美"记住"全部历史（注入文本作为上下文提示，效果取决于 agent 能力）。

## 10. 成功标准

- Prompt 产生的 SessionUpdate 流逐条持久化到 messages 表，保留原始 JSON 与可读文本。
- `ListMessages` 可查询会话完整消息历史，按 sequence 排序。
- `ResumeSession` 可对 error 状态的会话重建 ACP session、更新 session_id、注入历史上下文。
- 恢复后的会话可正常接收新 Prompt（status=active，conn 存在于连接池）。
- 单元测试全部通过，映射逻辑与仓库 CRUD 100% 覆盖。
- 复用 P1/P3 的分层，不破坏现有功能。
