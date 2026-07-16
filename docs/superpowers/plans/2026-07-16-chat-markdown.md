# 聊天助手正文 Markdown 渲染 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 仅对 `agent_message_chunk` 在非流式状态下用现有 `MarkdownContent` 渲染；流式中保持纯文本。

**Architecture:** `MessageList` 根据 `loading` 与「是否展示列表最后一条」计算 `streaming`，传给 `MessageBubble`；气泡在 `agent_message_chunk && !streaming` 时复用笔记页的 `MarkdownContent` + `.markdown-body`。

**Tech Stack:** React、react-markdown、remark-gfm（已安装）

## Global Constraints

- 仅 `agent_message_chunk` 做 Markdown；用户/思考/工具/plan 不变
- 流式中纯文本，结束后 Markdown
- 不新增依赖；不改后端 API
- 实现尽量少改代码

---

### Task 1: MessageBubble 支持 Markdown / streaming

**Files:**
- Modify: `web/src/components/MessageBubble.tsx`
- Reuse: `web/src/components/MarkdownContent.tsx`

**Interfaces:**
- Consumes: `MarkdownContent({ content, className? })`
- Produces: `MessageBubbleProps.streaming?: boolean`（默认 `false`）

- [x] **Step 1: 为 MessageBubble 增加 streaming 与 Markdown 分支**

在 `MessageBubble.tsx`：

1. `import MarkdownContent from './MarkdownContent'`
2. props 增加 `streaming?: boolean`（默认 `false`）
3. 将 content 渲染替换为：

```tsx
{message.content && (
  message.kind === 'agent_message_chunk' && !streaming ? (
    <div className={`${styles.content} markdown-body`}>
      <MarkdownContent content={message.content} />
    </div>
  ) : (
    <div className={styles.content}>{message.content}</div>
  )
)}
```

注意：`.content` 有 `white-space: pre-wrap`，Markdown 模式下该属性可能干扰块级元素间距。若视觉异常，给 Markdown 容器用独立 class（如去掉 pre-wrap），纯文本分支仍用 `styles.content`。

推荐更干净写法：

```tsx
{message.content && (
  message.kind === 'agent_message_chunk' && !streaming ? (
    <MarkdownContent content={message.content} className={`${styles.content} markdown-body`} />
  ) : (
    <div className={styles.content}>{message.content}</div>
  )
)}
```

并在 `MessageBubble.module.css` 增加：

```css
.content.markdown-body,
:global(.markdown-body).content {
  white-space: normal;
}
```

或更简单：Markdown 不用 `styles.content`，只用 `markdown-body` + 字号：

```tsx
<MarkdownContent
  content={message.content}
  className={`markdown-body ${styles.mdContent}`}
/>
```

```css
.mdContent {
  font-size: 14px;
  line-height: 1.6;
  word-break: break-word;
}
```

采用最后一种（独立 `mdContent`），避免 CSS modules 与 global 冲突。

- [x] **Step 2: 本地类型检查**

Run: `cd web && npx tsc --noEmit`
Expected: 无错误（或仅既有无关错误）

- [x] **Step 3: Commit**

```bash
git add web/src/components/MessageBubble.tsx web/src/components/MessageBubble.module.css
git commit -m "feat: 助手正文气泡支持 Markdown 渲染"
```

---

### Task 2: MessageList 下发 streaming

**Files:**
- Modify: `web/src/components/MessageList.tsx`

**Interfaces:**
- Consumes: `MessageBubble` 的 `streaming?: boolean`
- Produces: 对展示列表最后一条且 `kind === 'agent_message_chunk'` 且 `loading` 时传 `streaming={true}`

- [x] **Step 1: 在 SegmentList 计算并传入 streaming**

在 `SegmentList` 内，分组合并后的 `messages` 即为展示列表。在 map 前：

```tsx
const lastMsg = messages[messages.length - 1]
```

渲染单条 `MessageBubble` 时：

```tsx
const isStreamingAssistant =
  !!loading &&
  msg.kind === 'agent_message_chunk' &&
  msg === lastMsg

// ...
<MessageBubble
  ...
  streaming={isStreamingAssistant}
/>
```

说明：`CollapsibleGroup` 内只有 thought/tool，不会是 `agent_message_chunk`，无需传 `streaming`（默认 false）。

用引用相等 `msg === lastMsg` 安全，因为 `seg.message` 来自同一 `messages` 数组。若担心合并后对象身份，可改用：

```tsx
const lastKey = lastMsg ? (lastMsg.id || lastMsg.sequence) : null
const isStreamingAssistant =
  !!loading &&
  msg.kind === 'agent_message_chunk' &&
  (msg.id || msg.sequence) === lastKey
```

采用 key 比较更稳妥。

- [x] **Step 2: 类型检查**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

- [x] **Step 3: Commit**

```bash
git add web/src/components/MessageList.tsx
git commit -m "feat: 流式中助手正文保持纯文本"
```

---

### Task 3: 手动验证与收尾

**Files:** 无代码改动（除非 CSS 微调）

- [x] **Step 1: 构建前端**

Run: `cd web && npm run build`
Expected: 成功

- [x] **Step 2: 手工验收清单**

1. 历史会话中助手消息：标题/列表/代码块渲染为 Markdown
2. 发送新 prompt 流式中：助手正文为纯文本
3. 流式结束后：切为 Markdown
4. 用户消息、思考、工具调用外观不变
5. 笔记页 Markdown 不受影响

（构建已通过；UI 手工项需在运行中会话确认）

- [x] **Step 3: 若有 CSS 微调则一并 commit，否则跳过**
