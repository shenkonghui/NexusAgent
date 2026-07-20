# 笔记自动标题（摘要）设计文档

- 日期：2026-07-16
- 状态：已批准（已实现）

## 1. 背景与目标

openNexus 已具备笔记 CRUD、`#标签` 解析、以及基于 Agent 的自动分类（`classify_pending` 队列 → `NoteClassifyWorker` → 专用「笔记分类」会话）。创建/更新时目前用 `parseNoteMeta` 从正文首行抽取标题，导入导出只保留正文，无法完整往返 title/tags。

目标：

1. **自动生成标题**：复用笔记分类 Agent（同一 Agent/模型/会话/队列），一次调用同时返回标签与标题。
2. **创建时标题留空**，由 Agent 写入；已有标题不覆盖。
3. **导入导出**支持 title 与 tags（Markdown + YAML frontmatter），兼容旧无 frontmatter 文件。

## 2. 需求摘要

| 项 | 决定 |
|----|------|
| 落点 | 写入笔记 `title`（不新增 summary 字段） |
| 触发 | 与分类相同：入队 + 间隔后 worker 处理 |
| 覆盖策略 | Agent 返回非空 title 时写入（覆盖旧标题；笔记无独立标题编辑） |
| Agent 关系 | 完全复用：一次 prompt 返回 tags + title |
| 创建 | 不再用正文抽标题，`title=""` |
| 更新 | 保留原 title，不用正文覆盖 |
| 导入导出 | frontmatter 携带 title/tags；旧格式兼容 |
| 配置 | 不新增 settings 字段；更新默认 prompt 文案 |

## 3. 整体架构

```text
创建/更新/导入笔记
  → 解析正文 #标签
  → 创建/导入无 title：title=""
  → 更新：保留原 title
  → 已配置 Agent → classify_pending=true
       │
       ▼
NoteClassifyWorker（按用户间隔扫描）
       │
       ▼
NoteClassifier：ensureClassifySession + PromptWithExecution
  期望输出：{"tags":[...],"title":"..."}
       │
       ▼
合并 tags；若 title 为空则写入 title；清除 pending
```

不新增 worker、会话来源或 NoteSettings 字段。

## 4. Prompt 与解析

### 4.1 默认 prompt

替换 `DefaultNoteClassifyPrompt`：

```text
你是一个笔记分类与标题助手。根据笔记内容：
1) 从已有标签中选择或创建合适新标签（小写英文或中文，不含空格和 #）
2) 生成简短标题（建议 ≤40 字，概括主题，不要引号）

已有标签：{{existing_tags}}
笔记内容：
{{content}}

仅输出 JSON 对象，例如 {"tags":["工作","想法"],"title":"周会纪要"}, 不要输出其他任何文字。
```

设置页提示改为「分类 + 自动标题」；用户已保存的自定义 prompt **不强制迁移**。

### 4.2 解析规则

1. 优先解析 JSON 对象 `{"tags":[...],"title":"..."}`（支持 markdown code fence）。
2. 回退：若为 JSON 数组 → 仅作 tags（兼容旧自定义 prompt）。
3. AI `title`：`TrimSpace` 后经现有 `truncateTitle`（≤80 字）再写入。
4. Agent 返回非空 `title` 时写入（覆盖已有标题）。
5. tags 与正文手工标签经现有 `mergeTags` 合并。

### 4.3 失败与 pending

| 情况 | 行为 |
|------|------|
| Agent 失败 / 无返回 | 保留 pending，打日志，下轮重试（与现分类一致） |
| JSON 完全无法解析 | 本轮跳过，保留 pending，打日志 |
| 仅 tags 合法、title 缺失或空白 | 更新 tags，清 pending；title 留空（避免死循环） |
| 旧数组格式 | 更新 tags，清 pending；不改 title |

## 5. API 与数据行为

### 5.1 创建 `POST /api/v1/notes`

- 入参仍为 `content`。
- 从正文解析 `#标签` → `tags`。
- `title` 固定为 `""`（停止用首行抽标题）。
- 已配置分类 Agent 时 `classify_pending=true`。

### 5.2 更新 `PUT /api/v1/notes/:id`

- 更新 `content`；tags 按正文 `#标签` 解析结果写入（与现 Update 一致：以本次解析为准）。
- **保留原 `title`**，不用正文覆盖。
- 若更新后 `title == ""` 且已配置 Agent → 重新入队；已有标题不因内容变更清空。

### 5.3 导出 `GET /api/v1/notes/export`

仍为单个 Markdown，笔记间以独立成行的 `===` 分隔；每条带 YAML frontmatter：

```markdown
---
title: 周会纪要
tags:
  - 工作
  - 会议
---
正文内容...

===

---
title: 另一条
tags: []
---
正文...
```

- `title` / `tags` 来自数据库字段；正文为 `content`（不再把 title 塞进正文首行）。
- `Content-Type` 与文件名规则保持现有。

### 5.4 导入 `POST /api/v1/notes/import`

请求项扩展：

```json
{ "notes": [{ "content": "...", "title": "可选", "tags": ["可选"] }] }
```

规则：

- `content` 必填（去空白后为空则 skipped）。
- 去重仍按 `content`。
- `title`：有值则写入；缺省或空 → `""`（可交 AI）。
- `tags`：请求 tags ∪ 正文 `#标签`。
- `classify_pending` 与创建相同（已配 Agent 则入队）。
- 前端解析导出 frontmatter 填入 title/tags；无 frontmatter 的旧 `.md` 仅 content，兼容。

## 6. 前端

- 列表：`title` 为空时显示占位（i18n，如「生成标题中…」），配合现有 `classify_pending` 徽章与轮询。
- 设置：分类说明改为涵盖自动标题；默认 prompt 展示与后端默认一致。
- 导入：按 `===` 拆分后解析 YAML frontmatter；解析失败则整段当 content。

## 7. 非目标

- 不新增独立 summary 字段或「仅摘要」开关。
- 不新增独立标题 Agent / 会话 / 间隔配置。
- 不强制迁移用户已保存的自定义 classify_prompt。
- 不做标题手动编辑独立 API（用户可改 content；title 暂无单独编辑入口则保持现状，后续可另开需求）。

## 8. 测试要点

- 创建后 pending → Agent 返回对象 → title/tags 写入且 pending 清除。
- 已有 title 时 Agent 返回新 title → 不覆盖。
- 旧数组格式 → 只更新 tags。
- 仅 tags、无 title → 清 pending，title 仍空。
- 导出含 frontmatter；导入往返 title/tags。
- 无 frontmatter 旧文件可导入。
- 更新内容保留原 title。
