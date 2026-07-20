# 笔记 MCP Server 设计文档

- 日期：2026-07-16
- 状态：已批准（实现中）

## 1. 背景与目标

openNexus 已具备用户级笔记（CRUD、标签、Agent 自动分类）与 REST API（含 `?tag=` 过滤）。设置页「笔记设置」当前只配置分类 Agent，与 MCP 无关。ACP `NewSession` 的 `McpServers` 目前固定传空数组。

目标：暴露只读 Notes MCP（HTTP），同时服务：

1. **本系统 Agent**：创建会话时注入同一 MCP 端点。
2. **外部 MCP 客户端**（Cursor / Claude Desktop 等）：用 Endpoint + Token 连接。

## 2. 需求摘要

| 项 | 决定 |
|----|------|
| 使用方 | 本系统 Agent + 外部客户端 |
| 能力范围 | 只读 |
| 传输 | 进程内 HTTP（挂载 `/mcp/notes`） |
| 鉴权 | 笔记设置中的专用 MCP Token（与 JWT 分离） |
| Token 生命周期 | 只生成一次，不轮换、不作废 |
| 设置入口 | 「设置 → 笔记设置」查看 Endpoint / Token |

## 3. 整体架构

```text
外部 MCP 客户端 ──┐
                  │  Authorization: Bearer <note_mcp_token>
本系统 Agent ─────┼──► HTTP  /mcp/notes  ──► Notes MCP Handler
(NewSession 注入) │                              │
                  │                              ▼
                  │                     校验 Token → user_id
                  │                              │
                  │                              ▼
                  │                     NoteRepository（只读）
```

- 内外共用同一端点与同一套 tools。
- 空 Token = 未启用；有 Token = 启用。
- ACP：用户已有 Token 时写入 `McpServers`；否则仍传空。

## 4. MCP 工具

| Tool | 参数 | 行为 |
|------|------|------|
| `list_note_tags` | 无 | 返回当前用户全部标签（去重，按名排序） |
| `list_notes` | `tag`（必填） | 返回该 tag 下笔记：`id`、`title`、`tags`、`updated_at`；**若 `title` 为空，额外返回完整 `content`**；有标题则不返回正文 |
| `get_note` | `id`（必填） | 返回单条：`id`、`title`、`content`、`tags`、`updated_at` |

约束：

- 一律按 Token 对应的 `user_id` 隔离。
- 无写操作（创建 / 更新 / 删除均不做）。
- 越权或不存在的 `id`：tool 返回「未找到」，不区分「不存在」与「非本用户」。

## 5. Token 与鉴权

### 5.1 存储

在用户级 `NoteSettings` 增加字段 `mcp_token`：

- 明文存储并可回显（因不可轮换，设置页必须能再次复制）。
- 空值表示未启用 Notes MCP。

### 5.2 生成

- API：`POST /api/v1/notes/settings/mcp-token`
- 仅当 `mcp_token` 为空时允许生成；已存在则拒绝（如 `409`）。
- Token 为足够长的随机串（≥32 bytes，hex 或 base64url）。

### 5.3 请求鉴权

- MCP 请求头：`Authorization: Bearer <mcp_token>`
- 用 Token 反查 `user_id`；无效或空 → `401`
- 不走登录 JWT。

### 5.4 ACP 注入

`NewSession` 时若该用户 `mcp_token` 非空，注入例如：

- `type`: `http`（若目标 Agent 仅声明 SSE，则用 `sse`，以实现时 Agent 能力为准）
- `name`: `opennexus-notes`
- `url`: `{public_base}/mcp/notes`
- `headers`: `Authorization: Bearer <token>`

无 Token 时 `McpServers` 仍为空数组。

`public_base` 优先取服务配置中的对外 Base URL；缺省时可用请求可达的本机地址，设置页 Endpoint 用当前站点 `origin` 展示。

## 6. 设置页 UI

在「笔记设置」分类配置下方增加 **「笔记 MCP」** 区块：

| 状态 | UI |
|------|-----|
| 无 Token | 简短说明 + 「生成 Token」按钮 |
| 已有 Token | 只读展示 Endpoint、Token，各自「复制」；**不再显示生成按钮** |

展示：

- Endpoint：`{当前站点 origin}/mcp/notes`
- Token：完整可复制
- MCP 配置：展示包含真实 Token 的完整 `mcpServers` JSON，并支持一键复制到其他 Agent 配置文件
- 可选提示：外部客户端使用 `Authorization: Bearer <token>`

不做：轮换、作废、开关、手填 Token。

设置读取：`GET /api/v1/notes/settings` 增加返回 `mcp_token`（及可选 `mcp_endpoint`；Endpoint 也可仅由前端拼接）。

## 7. 错误处理

| 场景 | 行为 |
|------|------|
| 无/错 Token 访问 MCP | HTTP `401` |
| `list_notes` 缺 `tag` | MCP tool 参数错误 |
| `get_note` 缺/非法 `id` | MCP tool 参数错误 |
| 笔记不存在或非本用户 | tool「未找到」 |
| 重复生成 Token | HTTP `409`（或等价业务错误） |

## 8. 测试范围（最小）

- Token：未生成不可用；生成一次成功；再次生成拒绝；设置可回读。
- Tools：按 tag 列表；无 title 带全文；有 title 不带正文；`get_note` 成功；越权 id「未找到」。
- ACP：有 Token 时 `McpServers` 含 `opennexus-notes`；无 Token 时为空。

## 9. 明确不做（本版）

- 笔记写操作（create/update/delete）
- Token 轮换 / 作废
- stdio 传输
- 通用「多 MCP server」配置 UI
- `@note:{id}` 后端展开（可另开任务）

## 10. 主要改动面（实现指引）

| 区域 | 改动 |
|------|------|
| `models.NoteSettings` | 增加 `mcp_token` |
| `note` repository / handler | 生成 Token API；settings 返回 Token |
| 新 MCP handler / 路由 | `/mcp/notes` + tools |
| `internal/acp` | `NewSession` 按用户注入 `McpServers` |
| `SettingsPage` + i18n | 「笔记 MCP」区块 |
| 文档 | 更新原 ACP「不实现 MCP」约定为本功能例外 |

## 11. 成功标准

1. 用户在「笔记设置」生成一次 Token 后，可复制 Endpoint 与 Token 给外部客户端，并按 tag 只读拉取笔记。
2. 同一用户新建会话时，Agent 可通过注入的 Notes MCP 调用相同 tools。
3. 未生成 Token 时，MCP 不可用，ACP 不注入该 server。
