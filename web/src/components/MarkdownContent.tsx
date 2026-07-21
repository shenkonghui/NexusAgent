import { memo } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import DrawioViewer from './DrawioViewer'

interface MarkdownContentProps {
  content: string
  className?: string
}

// 模块级常量：保持 remarkPlugins / components 引用稳定，避免每次渲染都让 ReactMarkdown 重建。
const REMARK_PLUGINS = [remarkGfm]

const MARKDOWN_COMPONENTS: Components = {
  pre({ children }) {
    return <pre>{children}</pre>
  },
  code({ className: langClass, children, ...props }) {
    // 识别 fenced 代码块的语言（行内 code 无 language-xxx 类）
    const match = /language-(\w+)/.exec(langClass || '')
    const lang = match?.[1]
    if (lang === 'drawio' || lang === 'draw') {
      const xml = String(children).replace(/\n$/, '')
      return <DrawioViewer xml={xml} />
    }
    return (
      <code className={langClass} {...props}>
        {children}
      </code>
    )
  },
}

// memo：content/className 不变时跳过整篇 Markdown 的重新解析与渲染。
// 文档预览与 AI 对话同处一棵组件树，父级（ChatPage）高频重渲染时这里可避免无谓重解析。
function MarkdownContent({ content, className }: MarkdownContentProps) {
  return (
    <div className={className}>
      <ReactMarkdown remarkPlugins={REMARK_PLUGINS} components={MARKDOWN_COMPONENTS}>
        {content}
      </ReactMarkdown>
    </div>
  )
}

export default memo(MarkdownContent)
