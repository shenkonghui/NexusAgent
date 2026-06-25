import { useEffect, useRef } from 'react'
import type { Message } from '../types'
import MessageBubble from './MessageBubble'
import styles from './MessageList.module.css'

interface MessageListProps {
  messages: Message[]
  loading?: boolean
}

export default function MessageList({ messages, loading }: MessageListProps) {
  const endRef = useRef<HTMLDivElement>(null)

  // 新消息时自动滚动到底部
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  return (
    <div className={styles.container}>
      {messages.length === 0 && !loading && (
        <div className={styles.empty}>
          <p>暂无消息，发送 prompt 开始对话</p>
        </div>
      )}
      {messages.map((msg) => (
        <MessageBubble key={msg.id || msg.sequence} message={msg} />
      ))}
      {loading && (
        <div className={styles.loading}>
          <span className={styles.dot} />
          <span className={styles.dot} />
          <span className={styles.dot} />
        </div>
      )}
      <div ref={endRef} />
    </div>
  )
}
