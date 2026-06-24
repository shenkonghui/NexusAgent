# NexusAgent REST API 接入层（P5）设计文档

- 日期：2026-06-24
- 子项目：P5 — REST API 接入层
- 状态：待审查

## 1. 背景与定位

NexusAgent 是一个用 Go 开发的全栈平台，目标是通过 ACP（Agent Client Protocol）协议调用 Claude Code、Codex 等编码 agent 执行任务，并支持用户认证。

整个平台被拆分为 6 个可独立设计的子项目：

| # | 子项目 | 依赖 |
|----|--------|------|
| P1 | ACP 客户端核心库（已完成） | 无 |
| P2 | Agent 注册与编排层（已完成） | P1 |
| P3 | 用户认证系统（已完成） | 无 |
| P4 | 会话消息持久化与会话恢复（已完成） | P1, P2 |
| P5 | REST API 接入层（本文档） | P2, P3, P4 |
| P6 | Web UI | P5 |

本文档覆盖 **P5 REST API 接入层**。P5 将 P1/P2/P3/P4 的能力通过 HTTP REST + SSE（Server-Sent Events）暴露给前端。P3 已实现认证端点（register/login/refresh/logout/me）与中间件（AuthRequired/RequireRole），P5 复用这些组件，新增 agent 列表、会话 CRUD、prompt SSE 流、消息历史等端点。

## 2. 需求摘要

- 列出可用 agent 类型。
- 创建/查询/关闭会话。
- 通过 SSE 流式发送 prompt 并实时接收 agent 更新。
- 取消正在进行的 prompt。
- 恢复已失效的会话。
- 查询会话消息历史。
- 所有业务端点需登录，按用户隔离数据。

## 3. 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go（最新稳定版） |
| Web 框架 | Gin（复用 P3） |
| 流式传输 | SSE（Server-Sent Events），`text/event-stream` |
| 认证 | JWT 中间件（复用 P3） |
| 响应格式 | 统一 JSON（复用 P3 `handlers.Success`/`Fail`） |

## 4. 架构与目录结构

```
internal/
  handlers/
    session_handler.go          # 创建：会话 CRUD + prompt SSE + cancel + resume + messages
    session_handler_test.go     # 创建：handler 集成测试
    agent_handler.go            # 创建：agent 列表 handler
    agent_handler_test.go       # 创建：handler 测试
  router/
    router.go                   # 修改：注册会话/agent 路由，注入 agent.Router
```

职责边界：

- `handlers` 只做请求解析与响应组装，调用 `agent.Router`（P2）执行业务逻辑。
- `router` 注册路由组，将 `agent.Router` 注入到 handler。
- 复用 P3 的 `AuthRequired` 中间件保护业务端点。

## 5. API 端点设计

统一响应格式（复用 P3）：

- 成功：`{ "data": ... }`
- 失败：`{ "error": { "code": "...", "message": "..." } }`

### 5.1 Agent 端点（前缀 `/api/v1`，需登录）

| 方法 | 路径 | 说明 | 成功响应 |
|------|------|------|---------|
| GET | `/agents` | 列出可用 agent 类型 | 200，`{agents: [{type, display_name, description}]}` |

### 5.2 会话端点（前缀 `/api/v1`，需登录）

| 方法 | 路径 | 说明 | 请求体 | 成功响应 |
|------|------|------|--------|---------|
| POST | `/sessions` | 创建会话 | `{agent_type, cwd?}` | 201，`{session}` |
| GET | `/sessions` | 列出当前用户会话 | — | 200，`{sessions: [...]}` |
| GET | `/sessions/:id` | 获取会话详情 | — | 200，`{session}` |
| DELETE | `/sessions/:id` | 关闭会话 | — | 200，`{}` |
| POST | `/sessions/:id/prompt` | 发送 prompt（SSE 流） | `{prompt}` | 200，`text/event-stream` |
| POST | `/sessions/:id/cancel` | 取消当前 prompt | — | 200，`{}` |
| POST | `/sessions/:id/resume` | 恢复会话 | — | 200，`{session}` |
| GET | `/sessions/:id/messages` | 查询消息历史 | — | 200，`{messages: [...]}` |

`:id` 为 session 的数据库主键 `id`（uint），而非 ACP session_id。使用 DB 主键作为路由参数，因为会话恢复后 ACP session_id 会变，但 DB 主键不变。

### 5.3 SSE 流格式

`POST /api/v1/sessions/:id/prompt`：

请求体：
```json
{ "prompt": "写一个 hello world" }
```

响应头：
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

响应体（逐条推送）：
```
data: {"id":1,"session_id":"...","role":"assistant","kind":"agent_message_chunk","content":"Hello","raw_json":"...","sequence":1}

data: {"id":2,"session_id":"...","role":"tool","kind":"tool_call","content":"Read file","raw_json":"...","sequence":2}

data: [DONE]
```

每条 `data:` 行是一个 Message 的 JSON（P4 持久化后的消息结构），以 `\n\n` 分隔。流结束时发送 `data: [DONE]\n\n`。

### 5.4 错误码约定（新增）

| 场景 | HTTP | code |
|------|------|------|
| 会话不存在或不属于当前用户 | 404 | `SESSION_NOT_FOUND` |
| 会话不在活跃状态（prompt/cancel 时） | 409 | `SESSION_NOT_ACTIVE` |
| 未知 agent 类型 | 400 | `AGENT_NOT_FOUND` |
| prompt 为空 | 400 | `INVALID_REQUEST` |
| 会话已关闭（resume 时） | 409 | `SESSION_CLOSED` |
| 后端未注册 | 500 | `BACKEND_NOT_FOUND` |

## 6. 核心组件

### 6.1 AgentHandler

```go
type AgentHandler struct {
    router *agent.Router
}

func NewAgentHandler(router *agent.Router) *AgentHandler

// List GET /api/v1/agents
func (h *AgentHandler) List(c *gin.Context)
```

返回 `router.ListAgents()` 的结果，映射为 `{type, display_name, description}`。

### 6.2 SessionHandler

```go
type SessionHandler struct {
    router *agent.Router
}

func NewSessionHandler(router *agent.Router) *SessionHandler

// Create POST /api/v1/sessions
func (h *SessionHandler) Create(c *gin.Context)

// List GET /api/v1/sessions
func (h *SessionHandler) List(c *gin.Context)

// Get GET /api/v1/sessions/:id
func (h *SessionHandler) Get(c *gin.Context)

// Close DELETE /api/v1/sessions/:id
func (h *SessionHandler) Close(c *gin.Context)

// Prompt POST /api/v1/sessions/:id/prompt （SSE）
func (h *SessionHandler) Prompt(c *gin.Context)

// Cancel POST /api/v1/sessions/:id/cancel
func (h *SessionHandler) Cancel(c *gin.Context)

// Resume POST /api/v1/sessions/:id/resume
func (h *SessionHandler) Resume(c *gin.Context)

// Messages GET /api/v1/sessions/:id/messages
func (h *SessionHandler) Messages(c *gin.Context)
```

#### Create 流程

1. 解析请求体 `{agent_type, cwd?}`。
2. 从 context 获取 userID（中间件注入）。
3. 调用 `router.CreateSession(ctx, agentType, cwd, userID)`。
4. 返回 201 + session。

#### List 流程

1. 从 context 获取 userID。
2. 调用 `router.ListSessions(userID)`。
3. 返回 200 + sessions 数组。

#### Get 流程

1. 解析 `:id`（uint）。
2. 调用 `router.GetSessionByDBID(id)`，校验 userID 匹配。
3. 返回 200 + session。

> 注：P2 Router 当前通过 sessionID（ACP ID）查询。P5 需要通过 DB 主键 `id` 查询。需要在 P4 的 Service / Router 上新增按 DB 主键查询的方法（`GetSessionByDBID`），或在 handler 层先查库再校验归属。本 spec 选择在 Service/Router 新增 `GetSessionByDBID(id uint)` 方法，保持一致性。

#### Close 流程

1. 解析 `:id`。
2. 校验归属（同 Get）。
3. 调用 `router.CloseSession(ctx, sessionID)`。
4. 返回 200。

#### Prompt（SSE）流程

1. 解析 `:id`，校验 prompt 非空。
2. 校验会话归属。
3. 设置 SSE 响应头。
4. 调用 `router.Prompt(ctx, sessionID, prompt)` 获取 update channel。
5. 使用 `c.Stream()` 循环：
   - 从 channel 读取 update（类型为 `acp.SessionUpdate` 或 `interface{}`）。
   - 将 update 序列化为 Message JSON，写入 `c.Writer`（`data: <json>\n\n`）。
   - 触发 `c.Writer.Flush()`。
6. channel 关闭后，写入 `data: [DONE]\n\n`，结束流。

> Prompt 返回的 channel 元素类型：P4 持久化后，channel 元素为 `models.Message`（已映射）。P5 直接序列化 Message。

#### Cancel 流程

1. 解析 `:id`，校验归属。
2. 调用 `router.CancelSession(ctx, sessionID)`。
3. 返回 200。

#### Resume 流程

1. 解析 `:id`，校验归属。
2. 调用 `router.ResumeSession(ctx, sessionID)`。
3. 返回 200 + 更新后的 session。

#### Messages 流程

1. 解析 `:id`，校验归属。
2. 调用 `router.ListMessages(sessionID)`。
3. 返回 200 + messages 数组。

### 6.3 Router 扩展（P2/P4）

为支持按 DB 主键查询（P5 路由用 `:id` 作为 DB 主键），需在 Service/Router 新增方法：

```go
// acp.Service
func (s *Service) GetSessionByDBID(id uint) (*models.Session, error)

// agent.Router
func (r *Router) GetSessionByDBID(id uint) (*models.Session, error)
```

这些方法在 P4 spec 中未明确列出，P5 实现时一并补充。

### 6.4 Router 装配

修改 `internal/router/router.go` 的 `Setup` 函数签名，增加 `agentRouter *agent.Router` 参数：

```go
func Setup(authSvc *services.AuthService, jwtSvc *services.JWTService, agentRouter *agent.Router, mode string) *gin.Engine
```

路由注册：

```go
v1 := r.Group("/api/v1")
v1.Use(middleware.AuthRequired(jwtSvc))
{
    agentH := handlers.NewAgentHandler(agentRouter)
    v1.GET("/agents", agentH.List)

    sessionH := handlers.NewSessionHandler(agentRouter)
    v1.POST("/sessions", sessionH.Create)
    v1.GET("/sessions", sessionH.List)
    v1.GET("/sessions/:id", sessionH.Get)
    v1.DELETE("/sessions/:id", sessionH.Close)
    v1.POST("/sessions/:id/prompt", sessionH.Prompt)
    v1.POST("/sessions/:id/cancel", sessionH.Cancel)
    v1.POST("/sessions/:id/resume", sessionH.Resume)
    v1.GET("/sessions/:id/messages", sessionH.Messages)
}
```

同步修改 `cmd/server/main.go`：将 `agentRouter` 传入 `router.Setup`（当前 `_ = agentRouter`，改为实际传入）。

## 7. 配置

无新增配置项。P5 复用 P3 的 `server.mode`（debug/release）控制 Gin 模式。

## 8. 测试策略

### AgentHandler 测试（`agent_handler_test.go`）

- List：验证返回 agent 列表、JSON 结构正确。

### SessionHandler 测试（`session_handler_test.go`）

使用 `httptest` + Gin test mode。需要一个 mock 或真实 `agent.Router`（含内存 DB + 注册 backend）。

- Create：验证创建成功返回 201。
- Create 未知 agent：返回 400。
- List：验证返回当前用户会话。
- Get：验证返回会话详情。
- Get 不存在：返回 404。
- Close：验证关闭成功。
- Messages：验证返回消息列表。
- Prompt SSE：验证响应头为 `text/event-stream`，验证流式输出（使用能产生 update 的 mock）。

> Prompt SSE 完整测试需要 MockBackend 走通 ACP 握手。当前 P1 MockBackend 不足。handler 层可测试 SSE 响应头设置和错误路径（会话不存在/非 active）。

### 覆盖目标

- AgentHandler List 100% 覆盖。
- SessionHandler Create/List/Get/Close/Messages/Cancel/Resume 的成功与错误路径 100% 覆盖。
- Prompt SSE 错误路径（不存在/非 active）100% 覆盖。

## 9. 范围边界（不做）

- 不实现前端 UI（P6 负责）。
- 不实现 WebSocket（仅 SSE 单向流）。
- 不实现会话内容实时推送（非 prompt 期间的 agent 主动消息）。
- 不实现文件上传/下载端点（工作区文件由 agent 进程直接管理）。
- 不实现管理后台 API（用户管理、agent 配置管理等）。
- 不实现 API 限流/配额（留待未来）。

## 10. 成功标准

- `GET /api/v1/agents` 返回可用 agent 列表。
- 会话 CRUD 端点（创建/列表/详情/关闭）全部可用且通过测试。
- `POST /api/v1/sessions/:id/prompt` 通过 SSE 流式返回 agent 更新，流结束时发送 `[DONE]`。
- `POST /api/v1/sessions/:id/cancel` 可取消正在进行的 prompt。
- `POST /api/v1/sessions/:id/resume` 可恢复会话。
- `GET /api/v1/sessions/:id/messages` 可查询消息历史。
- 所有业务端点需登录，按用户隔离数据。
- 单元测试全部通过，handler 成功与错误路径 100% 覆盖。
- `cmd/server/main.go` 正确装配 `agentRouter` 并传入 `router.Setup`。
