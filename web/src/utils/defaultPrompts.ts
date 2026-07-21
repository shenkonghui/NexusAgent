import i18n from '../i18n'

// 内置默认提示词的中英对照。与后端 services.DefaultNoteClassifyPrompt /
// DefaultTaskTagPrompt / DefaultTaskTitlePrompt 保持一致（含 legacy 旧默认值）。
// 若 textarea 的值命中任一默认值，则按当前界面语言显示对应版本；否则原样返回（用户已自定义）。
interface PromptPair {
  zh: string
  en: string
}

// trim 比较，避免首尾空白差异导致命中失败
function eq(a: string, b: string): boolean {
  return a.trim() === b.trim()
}

const DEFAULT_PROMPTS: PromptPair[] = [
  {
    // 笔记分类与标题（现行默认）
    zh: `你是一个笔记分类与标题助手。根据笔记内容：
1) 从已有标签中选择或创建合适新标签（小写英文或中文，不含空格和 #）
2) 生成简短标题（建议 ≤40 字，概括主题，不要引号）

已有标签：{{existing_tags}}
笔记内容：
{{content}}

仅输出 JSON 对象，例如 {"tags":["工作","想法"],"title":"周会纪要"}, 不要输出其他任何文字。`,
    en: `You are a note classification and title assistant. Based on the note content:
1) Pick from existing tags or create suitable new ones (lowercase English or Chinese, no spaces or #)
2) Generate a short title (suggest ≤40 chars, summarize the topic, no quotes)

Existing tags: {{existing_tags}}
Note content:
{{content}}

Output only a JSON object, e.g. {"tags":["work","idea"],"title":"Weekly Meeting Notes"}, with no other text.`,
  },
  {
    // 任务打标签
    zh: `你是一个任务分类助手。根据用户任务描述，从预定义标签中选择最匹配的标签（1-3个）。只能从预定义标签中选择，不要创造新标签。
预定义标签：{{tags}}
任务描述：
{{prompt}}
仅输出 JSON 数组，例如 ["后端","mysql"]，不要输出其他任何文字。`,
    en: `You are a task classification assistant. Based on the user's task description, pick the best-matching tags (1-3) from the predefined tags. Only choose from predefined tags; do not invent new ones.
Predefined tags: {{tags}}
Task description:
{{prompt}}
Output only a JSON array, e.g. ["backend","mysql"], with no other text.`,
  },
  {
    // 任务生成标题
    zh: `请为以下任务生成一个简短的标题（不超过15个字，不要加引号或书名号，概括任务核心意图）。
任务描述：{{prompt}}
仅输出标题文字，不要输出其他内容。`,
    en: `Generate a short title for the following task (no more than 15 words, no quotes or brackets, capturing the core intent of the task).
Task description: {{prompt}}
Output only the title text, with no other content.`,
  },
  {
    // 文档编辑助手
    zh: `你是文档编辑助手。用户正在查看并希望修改工作区中的文档文件：
{{docPath}}

要求：
1. 请直接使用你的文件读写工具读取并编辑该文件来完成用户的请求，而不是仅在对话里输出正文。
2. 修改要保持文档整体结构清晰、Markdown 语法正确。
3. 如需绘制图表，可在 Markdown 中嵌入 \`\`\`drawio 代码块（内含完整的 <mxGraphModel>...</mxGraphModel> XML），文档预览会自动渲染。
4. 完成编辑后，用一两句话简要说明你做了哪些改动。`,
    en: `You are a document editing assistant. The user is viewing and wants to modify the document file in the workspace:
{{docPath}}

Requirements:
1. Use your file read/write tools to directly read and edit that file to fulfill the user's request, instead of only outputting the content in the chat.
2. Keep the document's overall structure clear and use correct Markdown syntax.
3. To draw diagrams, embed a \`\`\`drawio code block (containing the full <mxGraphModel>...</mxGraphModel> XML) in the Markdown; the document preview will render it automatically.
4. After editing, briefly describe in one or two sentences what changes you made.`,
  },
]

// 若 value 命中某个默认提示词（中或英），按当前语言返回对应版本；否则原样返回。
export function translatePrompt(value: string): string {
  if (!value) return value
  const isEn = i18n.language.startsWith('en')
  for (const p of DEFAULT_PROMPTS) {
    if (eq(value, p.zh) || eq(value, p.en)) {
      return isEn ? p.en : p.zh
    }
  }
  return value
}
