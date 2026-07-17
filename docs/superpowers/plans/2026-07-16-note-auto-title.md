# 笔记自动标题与导入导出 title/tags 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 复用笔记分类 Agent，一次调用生成 tags + title；创建时 title 留空由 AI 填充；导入导出通过 YAML frontmatter 往返 title/tags。

**Architecture:** 扩展 `NoteClassifier` 解析 JSON 对象；`NoteHandler` 创建不再抽标题、更新保留标题、导出/导入带 frontmatter；前端占位文案与 frontmatter 解析。

**Tech Stack:** Go、Gin、GORM、`gopkg.in/yaml.v3`、React/TS

**对应规格：** `docs/superpowers/specs/2026-07-16-note-auto-title-design.md`

## Global Constraints

- 不新增 NoteSettings 字段 / worker / 会话来源
- 仅当 `title == ""` 时写入 AI 标题
- 兼容旧 JSON 数组输出
- 新方法 ≤50 行；修改函数超 100 行需拆分；尽量少改原有逻辑
- 文档与 UI 文案使用中文
- 提交仅在用户明确要求时执行（本计划 Commit 步骤可跳过）

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/services/note_classifier.go` | 默认 prompt、解析 `{tags,title}`、写入空 title |
| `internal/services/note_classifier_test.go` | 解析与截断单测 |
| `internal/handlers/note_handler.go` | 创建/更新/导入导出行为；frontmatter |
| `internal/handlers/note_export_test.go` | 导出导入 frontmatter 单测 |
| `web/src/pages/NotesPage.tsx` | 空标题占位；导入解析 frontmatter |
| `web/src/api/notes.ts` | import 类型含 title/tags |
| `web/src/i18n/zh.json` / `en.json` | 文案 |

---

### Task 1: Classifier 解析 tags + title

**Files:**
- Modify: `internal/services/note_classifier.go`
- Modify: `internal/services/note_classifier_test.go`

**Interfaces:**
- Produces: `parseClassifyResult(text string) (tags []string, title string, ok bool)`；`classifyNote` 在 `note.Title==""` 且 title 非空时写入

- [x] **Step 1: 写失败测试**

在 `note_classifier_test.go` 增加：

```go
func TestParseClassifyResult(t *testing.T) {
	tags, title, ok := parseClassifyResult(`{"tags":["工作"],"title":"周会"}`)
	if !ok || title != "周会" || !reflect.DeepEqual(tags, []string{"工作"}) {
		t.Fatalf("object: tags=%v title=%q ok=%v", tags, title, ok)
	}
	tags, title, ok = parseClassifyResult(`["a","b"]`)
	if !ok || title != "" || !reflect.DeepEqual(tags, []string{"a", "b"}) {
		t.Fatalf("array: tags=%v title=%q ok=%v", tags, title, ok)
	}
	_, _, ok = parseClassifyResult("invalid")
	if ok {
		t.Fatal("invalid should not ok")
	}
}
```

- [x] **Step 2: 实现解析与 classify 写入**

1. 替换 `DefaultNoteClassifyPrompt` 为规格中的对象输出模板。
2. 新增 `parseClassifyResult`：先对象（含 fence），再回退数组；`ok=false` 表示无法解析。
3. `classifyTags` 改为返回 `(tags []string, title string, err error)`：无返回文本或 `!ok` → error（保留 pending）；数组/对象均 `ok`。
4. `classifyNote`：合并 tags；若 `note.Title==""` 且 AI title 非空则 `truncateNoteTitle` 写入；`ClassifyPending=false`。
5. `parseClassifyTags` 可改为调用 `parseClassifyResult` 只取 tags，保持旧测试通过。

- [x] **Step 3: 跑测**

Run: `go test ./internal/services/ -run 'ParseClassify|MergeTags|NormalizeClassify' -count=1`

---

### Task 2: Handler 创建/更新 + 导入导出 frontmatter

**Files:**
- Modify: `internal/handlers/note_handler.go`
- Create: `internal/handlers/note_export_test.go`

**Interfaces:**
- Produces: `parseNoteTags(content) []string`；`formatNoteMarkdown(title, content string, tags []string) string`；`parseNoteMarkdown(raw string) (title, content string, tags []string)`
- Import item: `{content, title?, tags?}`

- [x] **Step 1: 写导出/导入解析测试**

```go
func TestNoteFrontmatterRoundTrip(t *testing.T) {
	md := formatNoteMarkdown("周会纪要", "正文 #work", []string{"工作", "会议"})
	title, content, tags := parseNoteMarkdown(md)
	if title != "周会纪要" || !strings.Contains(content, "正文") {
		t.Fatalf("got title=%q content=%q tags=%v", title, content, tags)
	}
	if len(tags) < 2 {
		t.Fatalf("tags=%v", tags)
	}
	_, c2, _ := parseNoteMarkdown("纯正文\n第二行")
	if c2 != "纯正文\n第二行" {
		t.Fatalf("legacy content=%q", c2)
	}
}
```

- [x] **Step 2: 实现 handler 行为**

1. `parseNoteMeta` → `parseNoteTags`：只抽 `#标签`（保持原首行逻辑即可），不再返回标题。
2. `Create`：`Title: ""`，tags 来自 `parseNoteTags`。
3. `Update`：保留 `n.Title`；更新 content/tags；若 `n.Title==""` 且 `shouldEnqueueClassify` → `ClassifyPending=true`。
4. `Export`：每条 `formatNoteMarkdown`，再 `===` 拼接。
5. `Import`：`noteImportItem` 增加 `Title string`、`Tags []string`；title 用请求值（可空）；tags = 请求 ∪ `parseNoteTags(content)`；不再 `parseNoteMeta` 抽标题。
6. frontmatter 用 `gopkg.in/yaml.v3` 编解码小结构体；`parseNoteMarkdown` 无 `---` 则整段为 content、title 空。

- [x] **Step 3: 跑测**

Run: `go test ./internal/handlers/ -run NoteFrontmatter -count=1`

---

### Task 3: 前端 UI / 导入 / i18n

**Files:**
- Modify: `web/src/pages/NotesPage.tsx`
- Modify: `web/src/api/notes.ts`
- Modify: `web/src/i18n/zh.json`、`web/src/i18n/en.json`

- [x] **Step 1: API 与解析**

`importNotes` 参数改为 `{ content: string; title?: string; tags?: string[] }[]`。

`NotesPage` 增加 `parseImportedChunk(raw)`：匹配 frontmatter，抽出 title/tags/content；失败则 `{ content: raw }`。

- [x] **Step 2: 列表占位与设置文案**

- 空 title 显示 `t('notes.titlePending')`（中：生成标题中… / 英：Generating title…）
- 更新 `settings.classifyHint`、`settings.noteClassifyPromptHint`（若有）说明含自动标题
- `classifyTaskHint` 可略改为分类与标题

- [x] **Step 3: 手动确认**

创建笔记后列表显示占位；分类完成后出现标题（依赖已配置 Agent）。

---

### Task 4: 规格状态

- [x] 将 `docs/superpowers/specs/2026-07-16-note-auto-title-design.md` 状态改为「已批准（已实现）」
- [x] 本计划文件标记任务完成

---

## Spec 覆盖自检

| 规格项 | Task |
|--------|------|
| 一次 prompt 返回 tags+title | 1 |
| 仅空 title 写入 | 1 |
| 旧数组兼容 | 1 |
| 创建 title="" | 2 |
| 更新保留 title / 空则入队 | 2 |
| 导出/导入 frontmatter | 2+3 |
| UI 占位与文案 | 3 |
