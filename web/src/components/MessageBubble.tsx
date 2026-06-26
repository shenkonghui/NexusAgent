import { useState, useEffect, useMemo } from 'react'
import type { Message } from '../types'
import { parseDiffsFromMessage } from '../utils/diff'
import DiffView from './DiffView'
import styles from './MessageBubble.module.css'

interface MessageBubbleProps {
  message: Message
  // 思考消息在流式输出中时默认展开，结束后自动折叠
  defaultOpen?: boolean
  // 会话 ID 与工作目录，用于文件 diff 渲染与磁盘对比
  sessionId?: number
  cwd?: string
}

// kind 标签映射
const kindLabels: Record<string, string> = {
  user_message_chunk: '用户',
  agent_message_chunk: '助手',
  agent_thought_chunk: '思考',
  tool_call: '工具调用',
  tool_call_update: '工具更新',
  plan: '计划',
  usage_update: '用量',
}

// 提取工具调用的单行摘要
function toolSummary(content: string): string {
  const firstLine = content.split('\n')[0]?.trim() || ''
  if (firstLine) return firstLine
  return '工具调用'
}

export default function MessageBubble({ message, defaultOpen = false, sessionId, cwd }: MessageBubbleProps) {
  const [showRaw, setShowRaw] = useState(false)
  const [open, setOpen] = useState(defaultOpen)

  // 检测 tool_call 消息是否携带文件 diff
  const hasDiff = useMemo(
    () => (message.kind === 'tool_call' || message.kind === 'tool_call_update')
      && parseDiffsFromMessage(message).length > 0,
    [message],
  )

  // 流式思考：开始时展开，结束后折叠
  useEffect(() => {
    setOpen(defaultOpen)
  }, [defaultOpen])

  const isUser = message.role === 'user'
  const isThought = message.kind === 'agent_thought_chunk'
  const isTool = message.role === 'tool'
  const isPlan = message.kind === 'plan'

  // 思考和工具调用可折叠
  const collapsible = isThought || isTool

  const bubbleClass = isUser
    ? styles.userBubble
    : isThought
      ? styles.thoughtBubble
      : isTool
        ? styles.toolBubble
        : styles.assistantBubble

  const headerClick = collapsible
    ? () => setOpen((v) => !v)
    : undefined

  return (
    <div className={`${styles.container} ${isUser ? styles.containerUser : ''}`}>
      <div className={`${styles.bubble} ${bubbleClass}`}>
        <div
          className={`${styles.header} ${collapsible ? styles.headerClickable : ''}`}
          onClick={headerClick}
        >
          {!isUser && <span className={styles.role}>{kindLabels[message.kind] || message.role}</span>}
          {isPlan && <span className={styles.badge}>计划</span>}
          {isTool && (
            <span className={styles.summary}>{toolSummary(message.content)}</span>
          )}
          {collapsible && (
            <span className={styles.toggle}>{open ? '▾' : '▸'}</span>
          )}
          {message.raw_json && (!collapsible || open) && (
            <button
              className={styles.rawBtn}
              onClick={(e) => {
                e.stopPropagation()
                setShowRaw(!showRaw)
              }}
              type="button"
            >
              {showRaw ? '隐藏详情' : '查看详情'}
            </button>
          )}
        </div>
        {/* 折叠态：仅显示 header；展开态：显示完整内容 */}
        {!collapsible || open ? (
          <>
            {message.content && <div className={styles.content}>{message.content}</div>}
            {!message.content && !isPlan && (
              <div className={styles.contentMuted}>（无文本内容）</div>
            )}
            {hasDiff && sessionId != null && cwd != null && (
              <DiffView
                message={message}
                sessionId={sessionId}
                cwd={cwd}
                defaultExpanded={defaultOpen}
              />
            )}
            {showRaw && message.raw_json && (
              <pre className={styles.rawJson}>{message.raw_json}</pre>
            )}
          </>
        ) : null}
      </div>
    </div>
  )
}
