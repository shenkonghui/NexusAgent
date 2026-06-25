# NexusAgent 定时任务功能设计文档

- 日期：2026-06-25
- 子项目：P7 — 定时任务（Scheduled Tasks）
- 状态：已批准

## 1. 背景与定位

NexusAgent 已具备手动会话（P4/P5/P6）：用户选择 agent + cwd 创建会话，发送 prompt 与 agent 流式对话。本子项目在此基础上新增**定时任务**能力：用户配置 cron 表达式 + 固定 prompt + agent + cwd，由进程内调度器按计划自动触发执行。

关键决策（已与用户确认）：

- **执行/会话模型**：单任务单会话 + 执行分块。每个定时任务关联一个 Session，每次 cron 触发在该 session 内追加一轮对话；前端按执行块分块渲染。
- **调度配置**：预设 + cron 混合。前端提供常用预设下拉 + 自定义 cron 输入，后端统一存标准 5 字段 cron 表达式。
- **触发方式**：进程内 cron 调度器（`github.com/robfig/cron/v3`），随服务启停，重启后从库重新加载 enabled 任务。
- **Prompt 形式**：固定 prompt 文本。
- **查看页面**：复用现有聊天页 `/sessions/:id`，定时会话的消息按执行块分块渲染。

## 2. 需求摘要

- 左下角新增「定时任务配置」入口，进入配置页管理任务（增删改查 + 立即执行）。
- 侧边栏顶部改为双折叠分组：「手动会话」与「定时任务」，各自可折叠。定时任务分组展示各任务的关联会话，点击跳转聊天页查看历次执行。
- 定时任务分组顶部提供「最近执行」快捷入口，跳转到最近一次执行的任务会话。
- 进程内调度器按 cron 表达式自动触发：确保关联 session 存在（首次创建，已关闭则恢复），发送固定 prompt，同步消费消息流至结束。
- 同一任务执行重叠时跳过本次触发。
- 聊天页对定时会话按 `execution_id` 分块渲染执行记录（可折叠）。

## 3. 技术栈

| 组件 | 选型 |
|------|------|
| 调度器 | `github.com/robfig/cron/v3`（标准 5 字段 cron） |
| ORM | GORM（复用） |
| 数据库 | SQLite（复用） |
| 前端 | React + TypeScript（复用） |

## 4. 架构与目录结构

```
internal/
  models/scheduled_task.go              # 定时任务配置模型
  repository/scheduled_task_repository.go
  services/scheduler_service.go         # cron 调度器
  handlers/scheduled_task_handler.go    # REST handler
  router/router.go                      # 新增 /scheduled-tasks 路由组
  database/database.go                  # AutoMigrate 新表
  models/session.go                     # 新增 Source 字段
  models/message.go                     # 新增 ExecutionID 字段
  acp/service.go                        # CreateSession 支持 source；Prompt 支持 executionID
  repository/session_repository.go      # FindByUserID 支持 source 过滤
  repository/message_repository.go      # 新增按 execution_id 聚合查询
cmd/main.go                             # 启动/停止调度器
web/src/
  types.ts                              # 新增 ScheduledTask / Execution 类型
  api/scheduledTasks.ts                 # CRUD + 执行历史 API
  pages/ScheduledTasksPage.tsx          # 配置页
  pages/ScheduledTasksPage.module.css
  components/SessionSidebar.tsx         # 双折叠分组 + 左下角入口
  components/SessionSidebar.module.css
  components/MessageList.tsx            # 执行分块渲染
  components/MessageList.module.css
  App.tsx                               # 新增 /scheduled-tasks 路由
```

职责边界：

- `models.ScheduledTask` 仅定义数据结构。
- `repository.ScheduledTaskRepository` 负责持久化。
- `services.SchedulerService` 持有 `*cron.Cron`，管理任务调度与执行，依赖 `agent.Router`（通过接口）执行 prompt。
- `handlers.ScheduledTaskHandler` 暴露 REST API，校验参数与 cron 表达式。
- 前端配置页独立，聊天页与侧边栏在现有组件上扩展。

## 5. 数据模型

### 5.1 `scheduled_tasks` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | uint | PK，自增 | 主键 |
| `name` | string(128) | 非空 | 任务名称 |
| `agent_type` | string(64) | 非空 | agent 类型 |
| `cwd` | string(512) | 非空 | 工作目录（定时任务必填，不用临时目录） |
| `prompt` | text | 非空 | 每次执行的固定 prompt |
| `cron_expr` | string(128) | 非空 | 标准 5 字段 cron 表达式 |
| `enabled` | bool | 默认 true | 是否启用 |
| `user_id` | uint | 索引 | 所属用户 |
| `session_id` | string(128) | | 关联 ACP 会话 ID（首次执行回填） |
| `db_session_id` | uint | | 关联 Session 主键（用于跳转） |
| `last_run_at` | *time | | 最近一次执行时间 |
| `last_status` | string(32) | | 最近执行状态：`success`/`running`/`failed`/`skipped` |
| `last_error` | text | | 最近一次错误信息 |
| `created_at` | time | | |
| `updated_at` | time | | |

### 5.2 Session 模型扩展

`models.Session` 新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Source` | string(32) | `manual` / `scheduled`，默认 `manual` |

`ListSessions` API 增加可选 `source` query 过滤。

### 5.3 Message 模型扩展

`models.Message` 新增字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `ExecutionID` | *uint | 执行块 ID，同一次定时执行的所有消息共享；手动会话为 null |

执行块视图由后端按 `execution_id` 聚合 messages 计算：`execution_id`、`started_at`（min created_at）、`finished_at`（max created_at）、`message_count`、`status`。

> 不新建独立 executions 表，执行块与消息天然关联，避免冗余。

## 6. 后端 API

```
POST   /api/v1/scheduled-tasks              创建任务
GET    /api/v1/scheduled-tasks              列出当前用户任务
GET    /api/v1/scheduled-tasks/:id          任务详情
PUT    /api/v1/scheduled-tasks/:id          更新（name/prompt/cron/enabled/cwd/agent_type）
DELETE /api/v1/scheduled-tasks/:id          删除任务及其关联 session
POST   /api/v1/scheduled-tasks/:id/run      手动触发一次执行
GET    /api/v1/scheduled-tasks/:id/executions  执行历史（按 execution_id 聚合）
```

所有接口需认证，仅操作当前用户自己的任务。

`GET /api/v1/sessions?source=manual|scheduled` 支持 source 过滤。

## 7. 调度器服务

`SchedulerService`：

- 持有 `*cron.Cron` 实例与 `agent.Router`（通过 `SchedulerExecutor` 接口）引用。
- `Start()`：从库加载所有 `enabled=true` 任务，为每个任务 `AddJob(cron_expr, job)`，保存 cron entry ID 映射。
- Job 执行流程：
  1. 获取任务锁（per-task mutex），若已持有则跳过（`last_status=skipped`）。
  2. 确保关联 session 存在：`db_session_id==0` 时 `CreateSession(agent_type, cwd, user_id, source=scheduled)` 并回填 `session_id`/`db_session_id`；若 session 已 closed/error 则 `ResumeSession`。
  3. 分配本次 `execution_id`（任务内自增计数，存内存；或用全局递增——简化为任务内自增 + 时间戳确保唯一，实际用数据库 sequence：取该 session 当前 max execution_id + 1）。
  4. 调用 `router.Prompt(ctx, sessionID, prompt)`，同步消费 Message channel 至结束；每条消息写入 `execution_id`。
  5. 更新 `last_run_at`/`last_status`/`last_error`。
- `AddTask`/`UpdateTask`/`RemoveTask`/`SetEnabled`：动态增删/更新 cron entry。
- `Stop()`：`cron.Stop()`。

execution_id 分配：在 `acp.Service.Prompt` 增加可选 `executionID *uint` 参数（或新增 `PromptWithExecution` 方法），写入每条持久化消息。调度器调用时传入；手动 SSE prompt 传 nil。

并发控制：per-task `sync.Mutex`（非阻塞 TryLock，跳过重叠）。

## 8. 前端

### 8.1 侧边栏（SessionSidebar）

- 顶部 header 改为两个可折叠分组：「📝 手动会话」「⏰ 定时任务」，各带 ▶/▼ 折叠箭头。折叠状态存 localStorage。
- 「手动会话」展开显示 `source=manual` 的 session 列表（现有行为）。
- 「定时任务」展开显示 `source=scheduled` 的 session 列表（各任务的关联会话），每条显示任务名 + 最近执行状态 + last_run_at。点击跳转 `/sessions/:db_session_id`。
- 定时任务分组顶部「最近执行」快捷入口：跳转最近一次执行的任务会话。
- footer 新增「⏰ 定时任务配置」导航项 → `/scheduled-tasks`。

### 8.2 配置页（ScheduledTasksPage）

- 任务列表 + 新建/编辑表单：name、agent_type（AgentSelector）、cwd（DirectoryPicker）、cron 预设下拉 + 自定义输入、prompt 文本框、enabled 开关。
- cron 预设：每 5 分钟 / 每小时 / 每天 0 点 / 每天 9 点 / 每周一 9 点 / 自定义。
- 每条任务操作：立即执行、编辑、删除。

### 8.3 聊天页执行分块（MessageList）

- 当 `session.source === 'scheduled'` 时，按 `execution_id` 分组消息。
- 每组渲染可折叠块：头部显示执行序号 + started_at + 状态徽章；展开后显示原有消息流。
- 手动会话保持原渲染。

## 9. 错误处理与测试

- cron 表达式校验：`cron.ParseStandard`，失败返回 400。
- 调度器执行失败：捕获 panic，记录 `last_status=failed` + `last_error`。
- 任务删除：同时删除关联 session（调用 `router.DeleteSession`）。
- 后端测试：scheduler 触发逻辑、repository CRUD、handler 参数校验。

## 10. 范围

本设计可用一个实现计划覆盖，无需进一步拆分。
