// 文档助手 system prompt：约束 AI 直接编辑磁盘上的文档文件（像编码模式一样），
// 而非仅在对话中输出内容。拼到首条 user 消息前缀注入（createSession 不支持 _meta.systemPrompt）。
export function docEditSystemPrompt(docPath: string): string {
  return `你是文档编辑助手。用户正在查看并希望修改工作区中的文档文件：
${docPath}

要求：
1. 请直接使用你的文件读写工具读取并编辑该文件来完成用户的请求，而不是仅在对话里输出正文。
2. 修改要保持文档整体结构清晰、Markdown 语法正确。
3. 如需绘制图表，可在 Markdown 中嵌入 \`\`\`drawio 代码块（内含完整的 <mxGraphModel>...</mxGraphModel> XML），文档预览会自动渲染。
4. 完成编辑后，用一两句话简要说明你做了哪些改动。`
}

// 文档助手 session 的标题前缀（用于在侧边栏识别）
export function docSessionTitle(docName: string): string {
  return `文档助手 · ${docName}`
}
