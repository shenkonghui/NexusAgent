import { useState } from 'react'
import type { Message } from '../types'
import styles from './MessageBubble.module.css'

interface MessageBubbleProps {
  message: Message
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

export default function MessageBubble({ message }: MessageBubbleProps) {
  const [showRaw, setShowRaw] = useState(false)

  const isUser = message.role === 'user'
  const isThought = message.kind === 'agent_thought_chunk'
  const isTool = message.role === 'tool'
  const isPlan = message.kind === 'plan'

  const bubbleClass = isUser
    ? styles.userBubble
    : isThought
      ? styles.thoughtBubble
      : isTool
        ? styles.toolBubble
        : styles.assistantBubble

  return (
    <div className={`${styles.container} ${isUser ? styles.containerUser : ''}`}>
      <div className={`${styles.bubble} ${bubbleClass}`}>
        <div className={styles.header}>
          <span className={styles.role}>{kindLabels[message.kind] || message.role}</span>
          {isPlan && <span className={styles.badge}>计划</span>}
        </div>
        {message.content && <div className={styles.content}>{message.content}</div>}
        {!message.content && !isPlan && (
          <div className={styles.contentMuted}>（无文本内容）</div>
        )}
        {message.raw_json && (
          <div className={styles.rawToggle}>
            <button
              className={styles.rawBtn}
              onClick={() => setShowRaw(!showRaw)}
              type="button"
            >
              {showRaw ? '隐藏详情' : '查看详情'}
            </button>
            {showRaw && (
              <pre className={styles.rawJson}>{message.raw_json}</pre>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
