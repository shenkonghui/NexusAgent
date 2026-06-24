# NexusAgent ACP 客户端与 Agent 编排（P1+P2）设计文档

- 日期：2026-06-24
- 子项目：P1 — ACP 客户端核心库 / P2 — Agent 注册与编排层
- 状态：待审查

## 1. 背景与定位

NexusAgent 是一个用 Go 开发的全栈平台，目标是通过 ACP（Agent Client Protocol）协议调用 Claude Code、Codex 等编码 agent 执行任务，并支持用户认证。

整个平台被拆分为 6 个可独立设计的子项目：

| # | 子项目 | 依赖 |
|----|--------|------|
| P1 | ACP 客户端核心库（本文档） | 无 |
| P2 | Agent 注册与编排层（本文档） | P1 |
| P3 | 用户认证系统（已完成） | 无 |
| P4 | 任务/会话管理 + 工作区 | P1, P2 |
| P5 | REST API 接入层 | P2, P3, P4 |
| P6 | Web UI | P5 |

本文档覆盖 **P1 ACP 客户端核心库** 与 **P2 Agent 注册与编排层**。P1 基于 [`coder/acp-go-sdk`](https://github.com/coder/acp-go-sdk) 封装 ACP 协议层，内置 Claude Code 后端，管理会话持久化与工作区。P2 在 P1 之上构建 agent 注册表与路由层。

P3（用户认证系统）已建立分层架构（config/database/models/repository/services/handlers/middleware/router）和技术栈（Gin+GORM+SQLite），P1/P2 复用该分层模式与技术栈。

## 2. 需求摘要

- 通过 ACP 协议与编码 agent（首期支持 Claude Code）通信，执行代码任务。
- 管理 agent 子进程生命周期（启动、停止）。
- 创建/关闭 ACP 会话，会话元数据落库 SQLite。
- 工作区双模式：外部传入 cwd 或自动创建临时目录（关闭时清理）。
- 流式接收 session 更新（agent 消息、工具调用、计划等）。
- 权限请求自动批准（服务端场景，无交互式权限提示）。
- Agent 类型注册表：注册/查询可用 agent，用户显式选择 agent 类型创建会话。
- 单 agent 路由：用户选择一个 agent → 创建会话 → 执行任务。

## 3. 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go（最新稳定版） |
| ACP SDK | `github.com/coder/acp-go-sdk` |
| ORM | GORM（复用 P3） |
| 数据库 | SQLite（复用 P3） |
| 进程管理 | `os/exec` |
| 配置 | yaml + 环境变量覆盖（复用 P3） |

## 4. 架构与目录结构

方案 B：厚 P1 + 薄 P2。P1 包含协议层 + 会话持久化 + 工作区管理；P2 只做注册表和路由。

```
internal/
  acp/                        # P1: ACP 客户端核心库
    client.go                 # 实现 acp.Client 接口（自动批准+更新转发）
    process.go                # agent 子进程生命周期管理
    connection.go             # 封装 acp.ClientSideConnection
    backend.go                # Backend 接口 + ClaudeCode 实现
    workspace.go              # 工作区管理（外部 cwd / 临时目录）
    service.go                # 高层服务：Initialize/NewSession/Prompt/Cancel + 持久化
    service_test.go
    backend_test.go
    workspace_test.go
  agent/                      # P2: Agent 注册与编排层
    registry.go               # agent 类型注册表
    router.go                 # 路由：选 agent → 委托 P1 Service
    router_test.go
  models/
    session.go                # 新增：ACP 会话 GORM 模型
  repository/
    session_repository.go     # 新增：会话仓库
    session_repository_test.go
```

职责边界：

- `acp` 包含协议封装 + 会话持久化 + 工作区管理，对外暴露 `Service`。
- `agent` 只做注册表和路由选择，调用 `acp.Service`。
- `models` / `repository` / `database` 复用 P3 分层，新增 session 模型与仓库。

## 5. 数据模型

### 5.1 `sessions` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | uint | PK，自增 | 主键 |
| `session_id` | string | unique，索引 | ACP 协议返回的会话 ID |
| `agent_type` | string | 非空 | agent 类型（如 `claude-code`） |
| `cwd` | string | 非空 | 工作目录 |
| `status` | string | 非空，默认 `active` | 枚举：`active` / `closed` / `error` |
| `user_id` | uint | 索引 | 所属用户（关联 P3 users 表，可为空表示系统级） |
| `workspace_mode` | string | 非空 | 枚举：`external` / `temporary` |
| `temp_dir` | string | | 临时目录路径（mode=temporary 时记录，用于清理） |
| `last_prompt` | text | | 最近一次 prompt 文本（审计） |
| `created_at` | time | | 创建时间 |
| `updated_at` | time | | 更新时间 |
| `closed_at` | time | | 关闭时间 |

设计要点：

- `session_id` 是 ACP 协议层的会话标识，与数据库主键 `id` 分离。
- `user_id` 关联 P3 的 users 表，实现会话与用户的绑定；零值表示系统级会话（无归属用户）。
- `workspace_mode` 记录工作区类型，`temporary` 模式下关闭会话时清理 `temp_dir`。
- 不存储会话内容（ACP 会话内容由 agent 进程管理），只存元数据。
- 服务重启恢复：启动时将 DB 中所有 `status=active` 的会话标记为 `status=error`（因对应 agent 进程已不存在，连接无法恢复），需用户手动关闭后重新创建。

## 6. P1 核心组件

### 6.1 Backend 接口 + ClaudeCode 实现

```go
type Backend interface {
    Name() string                              // "claude-code"
    Command() string                           // "npx"
    Args() []string                            // ["-y", "@zed-industries/claude-code-acp@latest"]
    Env() []string                             // 环境变量
    Authenticate(conn *Connection) error       // 认证（如需要）
}
```

`ClaudeCodeBackend` 实现：通过 `npx @zed-industries/claude-code-acp@latest` 启动，环境变量注入 `ANTHROPIC_API_KEY`。

### 6.2 Process — 子进程管理

```go
type Process struct {
    cmd     *exec.Cmd
    stdin   io.WriteCloser
    stdout  io.ReadCloser
    backend Backend
}

func NewProcess(backend Backend) (*Process, error)   // 启动子进程
func (p *Process) Stdin() io.WriteCloser
func (p *Process) Stdout() io.ReadCloser
func (p *Process) Stop() error                       // kill 子进程
```

### 6.3 Client — 实现 acp.Client 接口

```go
type Client struct {
    updates chan acp.SessionUpdate   // 转发 session update
}
```

- `RequestPermission`：自动批准所有权限请求（选择 allow 选项）。
- `SessionUpdate`：将 update 转发到 channel 供调用方消费。
- `WriteTextFile` / `ReadTextFile`：直接读写 cwd 下文件。
- 终端方法（`CreateTerminal` 等）：no-op，暂不实现真实终端。

### 6.4 Connection — 封装 acp.ClientSideConnection

```go
type Connection struct {
    conn    *acp.ClientSideConnection
    process *Process
    client  *Client
}

func NewConnection(backend Backend) (*Connection, error)
func (c *Connection) Initialize(ctx) (acp.InitializeResponse, error)
func (c *Connection) NewSession(ctx, cwd string) (string, error)      // 返回 sessionID
func (c *Connection) Prompt(ctx, sessionID, prompt string) (<-chan acp.SessionUpdate, error)
func (c *Connection) Cancel(ctx, sessionID string) error
func (c *Connection) Close() error                                     // 关闭连接+进程
```

`Prompt` 返回一个 channel，流式推送 session update，调用方按需消费。channel 在 prompt turn 完成后（收到最终 update 或出错）由 `Connection` 关闭，调用方通过 channel 关闭感知结束。

### 6.5 Workspace — 工作区管理

```go
type Workspace struct {
    Mode    string   // "external" | "temporary"
    Cwd     string
    TempDir string   // temporary 模式下的临时目录
}

func NewExternalWorkspace(cwd string) *Workspace
func NewTemporaryWorkspace(prefix string) (*Workspace, error)   // os.MkdirTemp
func (w *Workspace) Cleanup() error                             // temporary 模式下删除目录
```

### 6.6 Service — 高层服务

串联所有组件 + 持久化：

```go
type Service struct {
    sessions  *repository.SessionRepository
    backends  map[string]Backend           // 已注册的后端
    conns     map[string]*Connection       // 活跃会话的连接池
    mu        sync.RWMutex                 // 保护 conns
    wsConfig  config.WorkspaceConfig
}

func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig) *Service
func (s *Service) RegisterBackend(b Backend)
func (s *Service) CreateSession(ctx, agentType, cwd string, userID uint) (*models.Session, error)
func (s *Service) Prompt(ctx, sessionID, prompt string) (<-chan acp.SessionUpdate, error)
func (s *Service) CancelSession(ctx, sessionID string) error
func (s *Service) CloseSession(ctx, sessionID string) error
func (s *Service) ListSessions(userID uint) ([]models.Session, error)
func (s *Service) GetSession(sessionID string) (*models.Session, error)
```

流程：

- **CreateSession**：选 Backend → NewProcess → NewConnection → Initialize → NewSession → 创建 Workspace → 落库 → 存入 conns → 返回。
- **Prompt**：查库获取 session → 从 conns 找到 Connection → Prompt → 返回 update channel。
- **CloseSession**：Connection.Close → Workspace.Cleanup → DB 更新 status=closed, closed_at → 从 conns 移除。

连接池：`Service` 内部维护 `map[sessionID]*Connection`，活跃会话对应一个常驻连接，用 `sync.RWMutex` 保护并发访问。

## 7. P2 注册表与路由

### 7.1 Registry — Agent 类型注册表

```go
type AgentDescriptor struct {
    Type        string      // "claude-code"
    DisplayName string      // "Claude Code"
    Description string      // 描述
    Backend     acp.Backend
}

type Registry struct {
    agents map[string]*AgentDescriptor
}

func NewRegistry() *Registry
func (r *Registry) Register(desc *AgentDescriptor) error    // 重复注册返回错误
func (r *Registry) Get(agentType string) (*AgentDescriptor, error)
func (r *Registry) List() []*AgentDescriptor
```

### 7.2 Router — 路由选择 + 委托执行

```go
type Router struct {
    registry *Registry
    service  *acp.Service
}

func NewRouter(registry *Registry, service *acp.Service) *Router
func (r *Router) CreateSession(ctx, agentType, cwd string, userID uint) (*models.Session, error)
func (r *Router) Prompt(ctx, sessionID, prompt string) (<-chan acp.SessionUpdate, error)
func (r *Router) CancelSession(ctx, sessionID string) error
func (r *Router) CloseSession(ctx, sessionID string) error
func (r *Router) ListSessions(userID uint) ([]models.Session, error)
func (r *Router) ListAgents() []*AgentDescriptor
```

Router 是薄层：

- `CreateSession`：校验 agentType 在注册表中 → 委托 `service.CreateSession`。
- `Prompt`/`Cancel`/`Close`/`List`：直接委托 `service`。
- `ListAgents`：返回注册表中的 agent 列表。

### 7.3 默认注册

启动时注册 Claude Code：

```go
registry.Register(&AgentDescriptor{
    Type:        "claude-code",
    DisplayName: "Claude Code",
    Description: "Anthropic Claude Code 编码 agent",
    Backend:     acp.NewClaudeCodeBackend(cfg),
})
```

## 8. 配置

在现有 `config.yaml` 基础上新增 `agents` 段：

```yaml
server:
  port: 8080
  mode: debug
database:
  path: ./data/nexus.db
jwt:
  secret: ""
  access_ttl: 15m
  refresh_ttl: 168h
password:
  bcrypt_cost: 12
agents:                        # 新增
  workspace:
    default_mode: external     # external | temporary
    temp_dir_prefix: nexus-    # 临时目录前缀
  claude_code:
    enabled: true
    command: npx               # 可覆盖命令路径
    args: ["-y", "@zed-industries/claude-code-acp@latest"]
    api_key_env: ANTHROPIC_API_KEY   # 从哪个环境变量读 API key
    timeout: 300s              # 单次 prompt 超时
```

环境变量覆盖：

- `ANTHROPIC_API_KEY` — Claude Code API 密钥
- `AGENTS_WORKSPACE_DEFAULT_MODE` — 默认工作区模式
- `CLAUDE_CODE_COMMAND` — 覆盖 claude-code 命令路径

Config 结构体扩展：

```go
type AgentsConfig struct {
    Workspace   WorkspaceConfig    `yaml:"workspace"`
    ClaudeCode  ClaudeCodeConfig   `yaml:"claude_code"`
}
type WorkspaceConfig struct {
    DefaultMode   string `yaml:"default_mode"`    // "external" | "temporary"
    TempDirPrefix string `yaml:"temp_dir_prefix"`
}
type ClaudeCodeConfig struct {
    Enabled   bool          `yaml:"enabled"`
    Command   string        `yaml:"command"`
    Args      []string      `yaml:"args"`
    APIKeyEnv string        `yaml:"api_key_env"`
    Timeout   time.Duration `yaml:"timeout"`
}
```

`Validate()` 新增校验：`agents.workspace.default_mode` 必须是 `external` 或 `temporary`。

## 9. 测试策略

### P1 测试

**`backend_test.go`** — ClaudeCode 后端

- 测试 `Name()`/`Command()`/`Args()` 返回预期值。
- 测试环境变量注入。

**`workspace_test.go`** — 工作区管理

- `NewExternalWorkspace`：验证 cwd 正确记录。
- `NewTemporaryWorkspace`：验证目录创建、Cleanup 后目录删除。
- `Cleanup` 对 external 模式不删除目录。

**`service_test.go`** — 核心服务（内存 SQLite）

- `CreateSession`：验证会话落库、Connection 建立。
- `Prompt`：验证返回 channel、收到 update。
- `CloseSession`：验证 Connection 关闭、临时目录清理、DB status=closed。
- `ListSessions`/`GetSession`：验证查询。

**Mock Backend**：定义 `MockBackend` 实现 `Backend` 接口，用于不启动真实 agent 进程的单元测试。`MockBackend` 启动一个简单的 echo 进程（或用 SDK 的 `example/agent`）。

### P2 测试

**`router_test.go`**

- `CreateSession`：验证 agentType 校验（未知类型返回错误）。
- `ListAgents`：验证返回已注册 agent 列表。
- `Prompt`/`Cancel`/`Close`：验证委托调用（用 mock service）。

### 集成测试（可选）

- 标记 `// +build integration`，需有效 `ANTHROPIC_API_KEY`。
- 启动真实 Claude Code agent，发送简单 prompt，验证收到响应。

### 覆盖目标

- P1 核心流程（会话创建/关闭/工作区）100% 覆盖。
- P2 路由逻辑 100% 覆盖。
- 真实 agent 交互为集成测试，不要求 CI 必过。

## 10. 范围边界（不做）

- 不实现 HTTP handler / REST API（P5 负责）。
- 不实现前端 UI（P6 负责）。
- 不实现多 agent 协作编排（P2 只做单 agent 路由）。
- 不实现 agent 自动选择/智能调度（用户显式指定 agent 类型）。
- 不实现真实终端管理（`Client` 的终端方法为 no-op）。
- 不实现会话内容持久化（只存元数据，会话内容由 agent 进程管理）。
- 不实现会话恢复（`session/resume`）—— 留待 P4。
- 不实现 Codex / Gemini 后端（P1 只内置 Claude Code，其他后端后续添加）。
- 不实现权限交互（自动批准所有权限请求）。
- 不实现 MCP server 配置（`NewSessionRequest.McpServers` 传空数组）。

## 11. 成功标准

- `acp.Service` 可创建/关闭 ACP 会话，会话元数据落库 SQLite。
- `acp.ClaudeCodeBackend` 可启动 Claude Code 进程并完成 ACP 握手。
- `agent.Registry` 可注册/查询 agent 类型，`agent.Router` 可路由请求。
- 工作区双模式（external / temporary）可用，temporary 模式关闭时清理目录。
- `Prompt` 返回流式 update channel，调用方可消费 session 更新。
- 单元测试全部通过，核心流程 100% 覆盖。
- 集成测试（需 API key）可手动运行验证真实 agent 交互。
- 配置通过 `config.yaml` + 环境变量加载，`Validate()` 校验合法性。
- 复用 P3 的 database/models/repository 分层，不破坏现有功能。
