# Workspace 功能设计规格

> 日期：2026-06-30
> 状态：设计中
> 版本：v1

## 1. 概述

将当前 Session 级 1:1 绑定的工作目录（cwd）概念，升级为独立的 Workspace（工作区）实体，实现：

- Workspace 是用户私有的持久化实体，绑定固定文件系统目录
- 同一 Workspace 下的多个 Session 共享该目录
- 用户手动创建 Workspace，进入 Workspace 后创建 Session 不再需要选择目录
- 未选择 Workspace 时，使用临时目录模式（默认 workspace），后续可保存为正式 Workspace

## 2. 数据模型

### 2.1 新增 `workspaces` 表

```go
type Workspace struct {
    ID        uint      `gorm:"primaryKey" json:"id"`
    UserID    uint      `gorm:"index;not null" json:"user_id"`
    Name      string    `gorm:"size:128;not null" json:"name"`
    Cwd       string    `gorm:"size:512;not null" json:"cwd"`
    Mode      string    `gorm:"size:32;not null;default:persistent" json:"mode"`
    TempDir   string    `gorm:"size:512" json:"temp_dir"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

字段说明：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | uint | 主键自增 |
| `user_id` | uint | 归属用户 |
| `name` | string | 用户命名的 workspace 名称 |
| `cwd` | string | 绑定的固定目录路径 |
| `mode` | string | `"persistent"`（正式）或 `"temporary"`（临时默认） |
| `temp_dir` | string | temporary 模式下的临时目录路径，persistent 模式为空 |
| `created_at` / `updated_at` | time | 时间戳 |

模式枚举：

```go
const (
    WorkspaceModePersistent = "persistent"  // 用户指定目录
    WorkspaceModeTemporary  = "temporary"   // 临时目录（默认 workspace）
)
```

### 2.2 Session 表变更

```diff
- Cwd           string  `gorm:"size:512;not null" json:"cwd"`
- WorkspaceMode string  `gorm:"size:32;not null" json:"workspace_mode"`
- TempDir       string  `gorm:"size:512" json:"temp_dir"`
+ WorkspaceID   *uint   `gorm:"index" json:"workspace_id"`
+ Workspace     Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
```

- `cwd`、`workspace_mode`、`temp_dir` 从 Session 迁移至 Workspace
- Session 通过 `WorkspaceID` 关联 Workspace
- `WorkspaceID` 外键可为空（向后兼容旧数据）
- Session 需要 cwd 时通过 `workspace.Cwd` 获取

### 2.3 关系图

```
User (1) ──< (N) Workspace ──< (N) Session ──< (N) Message
```

## 3. API 设计

### 3.1 新增 Workspace 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/v1/workspaces` | 创建 workspace |
| `GET` | `/api/v1/workspaces` | 列出当前用户所有 workspace |
| `GET` | `/api/v1/workspaces/:id` | 获取 workspace 详情（含 Session 列表） |
| `PUT` | `/api/v1/workspaces/:id` | 更新 workspace（重命名） |
| `DELETE` | `/api/v1/workspaces/:id` | 删除 workspace 及其所有 Session |
| `POST` | `/api/v1/workspaces/:id/save` | 将 temporary workspace 保存为 persistent |

#### POST /api/v1/workspaces

```json
// 请求
{
  "name": "我的项目",
  "cwd": "/path/to/project"
}

// 响应
{
  "id": 1,
  "user_id": 1,
  "name": "我的项目",
  "cwd": "/path/to/project",
  "mode": "persistent",
  "temp_dir": "",
  "created_at": "2026-06-30T19:00:00Z",
  "updated_at": "2026-06-30T19:00:00Z"
}
```

- `cwd` 必须是存在的目录，否则返回 400
- 同一用户下不允许相同 cwd 的 persistent workspace

#### GET /api/v1/workspaces

```json
// 响应
{
  "workspaces": [
    {
      "id": 1,
      "name": "我的项目",
      "cwd": "/path/to/project",
      "mode": "persistent",
      "session_count": 5
    }
  ]
}
```

- 始终包含默认 temporary workspace（如存在）
- 返回每个 workspace 的会话数量

#### POST /api/v1/workspaces/:id/save

```json
// 请求
{
  "name": "保存后的项目",
  "cwd": "/path/to/project"
}

// 响应
{
  "id": 1,
  "mode": "persistent",
  "name": "保存后的项目",
  "cwd": "/path/to/project"
}
```

- 仅对 `mode: "temporary"` 的 workspace 有效
- 将临时目录内容移动/保留，mode 变为 persistent

### 3.2 Session 接口变更

#### POST /api/v1/sessions（创建会话）

```diff
// 请求
{
-  "cwd": "/path/to/project",
-  "workspace_mode": "external",
+  "workspace_id": 1,          // 可选
   "agent_type": "claude-code",
   "model": "claude-sonnet-4"
}
```

- `workspace_id` 可选，不传时使用默认 temporary workspace
- 如果默认 workspace 不存在则自动创建

#### GET /api/v1/sessions/:id（会话详情）

响应中增加 workspace 嵌套：

```json
{
  "id": 123,
  "agent_type": "claude-code",
  "workspace_id": 1,
  "workspace": {
    "id": 1,
    "name": "我的项目",
    "cwd": "/path/to/project",
    "mode": "persistent"
  },
  "messages": [...]
}
```

### 3.3 默认 Workspace 逻辑

- 用户首次请求（创建 Session 或获取 workspace 列表）时，后端自动创建 `mode: "temporary"` 的默认 workspace
- 临时目录路径沿用现有配置 `~/.openNexus/session/<uuid>`
- 用户可通过 `save` 接口将临时 workspace 转为 persistent
- temporary workspace 永不被自动删除（除非用户登出/删除账号）

### 3.4 Session 恢复接口变更

`POST /api/v1/sessions/:id/resume` 移除 `cwd_override` 参数，恢复后的 Session 沿用其 workspace 的 cwd。

## 4. 前端设计

### 4.1 路由变更

| 路径 | 页面 | 说明 |
|------|------|------|
| `/` | HomePage | workspace 列表 + 快速对话入口 |
| `/workspaces/:wid` | WorkspacePage | workspace 详情，Session 列表 + 快速对话 |
| `/workspaces/:wid/sessions/:sid` | ChatPage | 对话页（复用现有 ChatPage） |

旧路由 `/sessions/:id` 重定向至对应的 `/workspaces/:wid/sessions/:sid`。

### 4.2 组件变更

#### WorkspaceSidebar（新增）

- 显示当前用户所有 workspace 列表
- 每个 workspace 项可展开/折叠，显示其下的 Session 列表
- 顶部"新建 Workspace"按钮 → 弹窗输入名称 + 选择目录
- 右键菜单：重命名、删除、保存为正式（仅 temporary）

#### HomePage（重构）

当前 ChatPage 的无会话模式改造为独立 HomePage：
- 左侧 WorkspaceSidebar
- 右侧中央区域：Agent 选择 + 模型选择 + 输入框
- 发送消息时使用默认 workspace（或当前选中的 workspace）

#### WorkspacePage（新增）

- 顶部显示 workspace 名称、路径
- 最近 Session 列表
- 底部输入框：直接发 prompt 创建新 Session（不选目录）

#### ChatPage（改造）

- 参数改为 `workspaceId` + `sessionId`
- 移除 `handleFirstSend` 中的目录选择逻辑
- PromptInput 不再接收 cwd 相关 props

### 4.3 数据流

```
创建 Workspace:
  User → "新建" 弹窗 → POST /api/v1/workspaces → 刷新列表

进入 Workspace:
  User 点击 workspace → navigate /workspaces/:wid → GET workspace 详情 + Session 列表

创建 Session（在 workspace 内）:
  User 输入 prompt → POST /api/v1/sessions { workspace_id } → navigate 到 /workspaces/:wid/sessions/:sid

快速对话（首页）:
  User 输入 prompt → 使用默认 workspace → POST /api/v1/sessions { workspace_id: default } → navigate

保存临时 workspace:
  User 右键"保存为正式" → 弹窗确认名称/目录 → POST /workspaces/:id/save → workspace 变为 persistent
```

### 4.4 新增类型定义

```typescript
interface Workspace {
  id: number
  user_id: number
  name: string
  cwd: string
  mode: 'persistent' | 'temporary'
  temp_dir?: string
  session_count?: number
  created_at: string
  updated_at: string
}
```

## 5. 迁移策略

### 5.1 阶段一：后端数据层

1. 新增 `Workspace` 模型，GORM AutoMigrate 自动建表
2. 处理旧数据：
   - 扫描所有用户，为每个用户创建一个临时默认 workspace
   - 扫描已有 Session，按 `cwd` 去重，为每个唯一 cwd 创建 persistent workspace
   - 将这些 Session 的 `workspace_id` 指向对应 workspace
3. Session 旧字段（`cwd`/`workspace_mode`/`temp_dir`）标记 `json:"-"`，逐步废弃

### 5.2 阶段二：后端 API

1. 新增 workspace handler + router
2. 修改 session handler（创建/查询时传入/返回 workspace_id）
3. 默认 workspace 自动创建逻辑
4. 文件/终端操作：从 `session.Workspace.Cwd` 获取路径

### 5.3 阶段三：前端

1. 新增 `WorkspaceSidebar`、`WorkspacePage` 组件
2. 改造 `ChatPage`，新增 `HomePage`
3. 路由更新
4. i18n 新增 workspace 相关翻译

### 5.4 阶段四：清理

- 代码层面移除旧字段引用
- 数据库层面暂不删除列（保证向后兼容）

## 6. 文件变更清单

| 层级 | 文件 | 变更 |
|------|------|------|
| model | `internal/models/workspace.go` | 新增 |
| model | `internal/models/session.go` | 修改（增加 WorkspaceID） |
| database | `internal/database/database.go` | AutoMigrate 新增 Workspace |
| config | `config.yaml` | 可选：workspace 配置项 |
| handler | `internal/handlers/workspace_handler.go` | 新增 |
| handler | `internal/handlers/session_handler.go` | 修改 |
| handler | `internal/handlers/session_file_handler.go` | 修改（cwd 改为从 workspace 获取） |
| handler | `internal/handlers/terminal_handler.go` | 修改（同上） |
| acp | `internal/acp/service.go` | 修改（创建 session 时接收 workspace_id） |
| router | `internal/router/router.go` | 新增路由 |
| types | `web/src/types.ts` | 新增 Workspace 类型 |
| api | `web/src/api/workspaces.ts` | 新增 |
| api | `web/src/api/sessions.ts` | 修改 CreateSession 参数 |
| component | `web/src/components/WorkspaceSidebar.tsx` | 新增 |
| page | `web/src/pages/HomePage.tsx` | 新增 |
| page | `web/src/pages/WorkspacePage.tsx` | 新增 |
| page | `web/src/pages/ChatPage.tsx` | 改造 |
| route | `web/src/App.tsx` | 路由更新 |
| i18n | `web/src/i18n/zh.json` | 新增翻译 |

## 7. 异常场景处理

| 场景 | 处理方式 |
|------|---------|
| 创建时 cwd 不存在 | 返回 400 "目录不存在" |
| 同用户重复 cwd 创建 | 返回 400 "该目录已绑定 workspace" |
| 删除 workspace 时其下有活跃 Session | 先取消活跃 Session，再级联删除 |
| temporary workspace 的 cwd 被外部删除 | 后台重新创建临时目录 |
| 用户无 workspace 时创建 Session | 自动创建默认 temporary workspace |

## 8. 实现补充（2026-07）

相对原设计有以下调整：

| 项 | 原设计 | 当前实现 |
|----|--------|----------|
| PromptInput 与 cwd | 不再接收 cwd | 仍传入 `activeSession.workspace.cwd`，用于 `@` 文件浏览 |
| 删除 Session | 清理工作区目录 | **不**清理共享工作区目录；仅删除 workspace 时调用 `Cleanup()` |
| temporary 目录丢失 | 后台重新创建 | `EnsureWorkspaceDir`：恢复会话时自动 `MkdirAll` |
| persistent 目录丢失 | — | 恢复会话返回错误，提示目录不存在 |
