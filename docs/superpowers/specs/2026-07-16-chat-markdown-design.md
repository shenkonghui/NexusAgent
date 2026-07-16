# 聊天助手正文 Markdown 渲染设计

日期：2026-07-16

## 1. 背景

Web UI 设计（`2026-06-24-web-ui-design.md`）已约定：`assistant` / `agent_message_chunk` 的 content 支持基本 Markdown。笔记页已通过 `MarkdownContent`（`react-markdown` + `remark-gfm`）渲染，但聊天 `MessageBubble` 仍将 content 作为纯文本输出。

## 2. 目标

- 助手正文（`agent_message_chunk`）支持 GFM Markdown 渲染（标题、列表、链接、代码块、表格等）。
- 流式输出过程中保持纯文本，本轮结束后再切换为 Markdown，避免未闭合 fence 导致布局抖动。

## 3. 非目标

- 用户消息、思考（`agent_thought_chunk`）、工具调用、plan 不做 Markdown。
- 不做 prompt 输入框的 Markdown 编辑器。
- 不新增语法高亮库或独立 XSS 消毒库（依赖 `react-markdown` 默认安全渲染）。
- 不做流式过程中的 Markdown 实时渲染或未闭合 fence 兜底。

## 4. 方案

复用现有 `MarkdownContent` 与 `global.css` 中的 `.markdown-body` 样式。

### 4.1 渲染规则

| 条件 | 渲染方式 |
|------|---------|
| `kind === 'agent_message_chunk'` 且非流式中 | `MarkdownContent` + `markdown-body` |
| `kind === 'agent_message_chunk'` 且流式中 | 纯文本（现有 `styles.content`） |
| 其他 kind | 保持现状（纯文本） |

### 4.2 流式判定

由 `MessageList` 计算并下发 `streaming`。对某条消息为真当且仅当：

- `loading === true`
- 该消息 `kind === 'agent_message_chunk'`
- 该消息是当前展示列表（分组合并后）中的**最后一条消息**

含义：仅「仍在追加的那条助手正文」走纯文本；同轮次中已被工具/思考打断的更早助手正文视为已完成，直接 Markdown。`loading` 变为 `false` 后全部切到 Markdown。

### 4.3 组件改动

1. **`MessageBubble`**
   - 新增可选 prop：`streaming?: boolean`（默认 `false`）。
   - 当 `message.kind === 'agent_message_chunk' && !streaming && message.content` 时，用 `MarkdownContent` 渲染；否则保持现有纯文本节点。

2. **`MessageList`**
   - 在渲染 `MessageBubble` 时，按 4.2 规则传入 `streaming`。
   - 分组内的单条气泡同样遵循该规则（助手正文一般不在可折叠分组内，但判定逻辑与现有 `loading` / 当前轮次一致）。

3. **样式**
   - 复用 `.markdown-body`；必要时仅微调气泡内边距/字号，不引入新主题。

## 5. 数据流

```
ChatPage (loading)
  → MessageList (计算 streaming)
    → MessageBubble
      → streaming ? 纯文本 : MarkdownContent
```

无后端或 API 变更。消息存储仍为原始 Markdown 字符串。

## 6. 错误与边界

- 空 content：与现有一致，显示「无数据」占位（非 plan）。
- 非法 / 残缺 Markdown：由 `react-markdown` 尽力渲染，不额外报错。
- 历史会话加载完成（非 loading）：一律 Markdown。

## 7. 测试要点

- 助手消息含标题、列表、行内代码、围栏代码块、链接时，结束后渲染正确。
- 流式过程中显示纯文本；结束后切换为 Markdown。
- 用户消息、思考、工具调用外观不变。
- 笔记页 `MarkdownContent` 行为不受影响。

## 8. 实现规模

预计改动：`MessageBubble.tsx`、`MessageList.tsx`；可选极小量 CSS。不新增依赖（`react-markdown` / `remark-gfm` 已存在）。
