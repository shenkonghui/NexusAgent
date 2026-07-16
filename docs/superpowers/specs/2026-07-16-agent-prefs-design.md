# Agent 最近使用偏好设计文档

- 日期：2026-07-16
- 状态：已批准

## 1. 背景与目标

新建任务页已能通过浏览器 `localStorage` 记住默认 agent 与各 agent 上次模型（`nexus.default.agent`、`nexus.agent.models`）。但模式、思考级别等其它探测配置不会恢复，且偏好无法跨浏览器/设备同步。

目标：

1. **记住最近使用的 agent**，下次新建任务自动选中。
2. **按 agent 记住新建页配置栏中所有可选项**（模型、模式、思考级别等），切换 agent 时自动恢复该 agent 最近配置。
3. **服务端持久化**，换浏览器仍可恢复。
4. **新建页改动立即写入**；**已有会话内改配置也同步更新**该 agent 的最近偏好。

## 2. 需求摘要

| 项 | 决定 |
|----|------|
| 记住范围 | 新建页配置栏所有 `select` 类 config（按 category） |
| 存储位置 | 服务端用户偏好表 |
| 写入时机 | 新建页改 agent/配置立即写；已有会话改配置成功后也写 |
| 恢复时机 | 新建任务选 last agent；切换 agent 恢复该 agent 的 prefs |
| 无效值 | category 不存在或 value 不在 options → 回退探测默认 |
| 设置页默认 Agent | 与 `last_agent_type` 统一为服务端同一字段 |
| localStorage | 弃用；若服务端为空可一次性迁移旧值后清除 |
| 范围外 | 定时任务/笔记分类的 agent·模型；不按工作区隔离 |

## 3. 整体架构

```text
新建任务页 / 已有会话改配置
  → PATCH /api/v1/agent-prefs（合并 last_agent + 该 agent 的 configs）
  → DB user_agent_prefs

进入新建任务页
  → GET /api/v1/agent-prefs
  → 选中 last_agent_type
  → probeAgentConfigs
  → 按 category 用 prefs 覆盖探测默认值（校验 options）

首次发送
  → createSession(..., model)
  → 对其余已选 config 依次 setConfigOption
  → PATCH 确保偏好已落库
```

## 4. 数据模型

表 `user_agent_prefs`（每用户一行，风格对齐 `NoteSettings` / `TaskSettings`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | uint PK | |
| `user_id` | uint，unique | 所属用户 |
| `last_agent_type` | string(64) | 最近使用的 agent |
| `prefs_json` | text | 各 agent 的配置偏好 |
| `created_at` / `updated_at` | time | |

`prefs_json` 结构：

```json
{
  "claude-code": {
    "model": "claude-sonnet-4",
    "mode": "default",
    "thought_level": "medium"
  },
  "cursor": {
    "model": "gpt-5"
  }
}
```

约定：

- 外层 key：agent type
- 内层 key：探测 config 的 **category**（`model` / `mode` / `thought_level` 等），不用 option id
- 只存用户实际选过的项；缺失时回退探测默认值
- 非法或不存在的 value 在恢复时忽略

## 5. API

### `GET /api/v1/agent-prefs`

返回当前用户偏好；无记录时：

```json
{
  "last_agent_type": "",
  "prefs": {}
}
```

### `PATCH /api/v1/agent-prefs`

部分更新（合并，不整表覆盖）：

```json
{
  "last_agent_type": "claude-code",
  "agent_type": "claude-code",
  "configs": {
    "model": "claude-sonnet-4",
    "mode": "default"
  }
}
```

约定：

- `last_agent_type` 可选：有则更新最近 agent
- `agent_type` + `configs` 可选：有则合并进该 agent 的 map（同 category 覆盖，其它保留）
- `configs` 中空字符串表示删除该 category 的记忆
- 返回更新后的完整 `{ last_agent_type, prefs }`

不做独立 DELETE。

## 6. 前端行为

### 6.1 新建任务页

1. 进入时 GET prefs，用 `last_agent_type` 选中 agent（不在可用列表则回退第一个）。
2. 探测 config 后：对每个 `select` 项，若 prefs 中有对应 category 且值仍在 options → 用记忆值，否则用探测默认。
3. 用户改 agent / 任一配置 → 本地立即更新 UI，debounce ~300ms 后 PATCH。
4. 首次发送：`createSession` 传 model；创建成功后对其余已选配置依次 `setConfigOption`；再 PATCH 一次完整偏好。

### 6.2 已有会话

`handleSetConfigOption` 成功后，按会话 `agent_type` + option `category` PATCH 偏好。

### 6.3 迁移与设置页

- 弃用 `nexus.agent.models`、`nexus.default.agent`；若服务端为空且本地有旧值，一次性迁移后清除本地。
- 设置页「默认 Agent」改为读写服务端 `last_agent_type`。

### 6.4 失败处理

PATCH 失败不打断主流程；可静默或轻提示。下次仍按服务端旧值恢复。

## 7. 边界情况

- 记忆的 category 在新探测结果中不存在 → 忽略
- 记忆的 value 不在 options 中 → 回退探测默认，不选无效项
- agent 已从系统移除 → 忽略 `last_agent_type`，选第一个可用
- 创建后部分 `setConfigOption` 失败 → 会话仍可用，已成功项保留；可提示但不回滚会话
- 多标签页同时改偏好 → 后写覆盖（可接受）

## 8. 测试要点

- Handler：GET 空默认；PATCH 合并 / 覆盖 / 空串删除；鉴权
- Repository：Upsert 与 JSON 合并
- 前端工具：按 category 解析 prefs、非法值回退（单测）
- 手动：新建恢复、切换 agent 恢复、会话内改模型后新建再验证

## 9. 实现范围（概要）

后端：model + repository + handler + router 注册 + AutoMigrate。

前端：API 封装；替换 `agentModel` localStorage 工具为服务端 prefs；`ChatPage` 恢复/写入/创建后应用非 model 配置；`SettingsPage` 默认 agent 改服务端。
