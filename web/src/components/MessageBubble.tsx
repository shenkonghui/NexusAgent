import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { Message } from '../types'
import { parseDiffsFromMessage } from '../utils/diff'
import DiffView from './DiffView'
import { ChevronDown, ChevronRight } from 'lucide-react'
import styles from './MessageBubble.module.css'

interface MessageBubbleProps {
  message: Message
  defaultOpen?: boolean
  forceCollapsed?: boolean
  sessionId?: number
  cwd?: string
}

const kindLabels: Record<string, string> = {
  user_message_chunk: 'chat.user',
  agent_message_chunk: 'chat.assistant',
  agent_thought_chunk: 'chat.thinking',
  tool_call: 'chat.toolCall',
  tool_call_update: 'chat.toolCallUpdate',
  plan: 'chat.plan',
  usage_update: 'chat.usage',
}

// 从工具调用 content 中提取首行作为摘要，回退到默认标签
export function toolSummary(content: string): string {
  const firstLine = content.split('\n')[0]?.trim() || ''
  if (firstLine) return firstLine
  return 'chat.toolCall'
}

export default function MessageBubble({ message, defaultOpen = false, forceCollapsed = false, sessionId, cwd }: MessageBubbleProps) {
  const { t } = useTranslation()
  const [showRaw, setShowRaw] = useState(false)
  const [open, setOpen] = useState(defaultOpen && !forceCollapsed)

  // 检测 tool_call 消息是否携带文件 diff
  const hasDiff = useMemo(
    () => (message.kind === 'tool_call' || message.kind === 'tool_call_update')
      && parseDiffsFromMessage(message).length > 0,
    [message],
  )

  // 流式思考：进行中展开，本轮结束后强制折叠
  useEffect(() => {
    if (forceCollapsed) {
      setOpen(false)
    } else {
      setOpen(defaultOpen)
    }
  }, [defaultOpen, forceCollapsed])

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
          {!isUser && <span className={styles.role}>{t(kindLabels[message.kind] || message.role)}</span>}
          {isPlan && <span className={styles.badge}>{t('chat.plan')}</span>}
          {isTool && (
            <span className={styles.summary}>{t(toolSummary(message.content))}</span>
          )}
          {collapsible && (
            <span className={styles.toggle}>{open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}</span>
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
              {showRaw ? t('chat.hideDetail') : t('chat.viewDetail')}
            </button>
          )}
        </div>
        {!collapsible || open ? (
          <>
            {message.content && <div className={styles.content}>{message.content}</div>}
            {!message.content && !isPlan && (
              <div className={styles.contentMuted}>{t('common.noData')}</div>
            )}
            {hasDiff && sessionId != null && cwd != null && (
              <DiffView
                message={message}
                sessionId={sessionId}
                cwd={cwd}
                defaultExpanded={defaultOpen && !forceCollapsed}
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
