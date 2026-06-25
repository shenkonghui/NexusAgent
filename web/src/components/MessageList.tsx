import { useEffect, useRef, useState } from 'react'
import type { Message } from '../types'
import MessageBubble from './MessageBubble'
import styles from './MessageList.module.css'

interface MessageListProps {
  messages: Message[]
  loading?: boolean
  // scheduled=true 时按 execution_id 分块渲染（定时会话）
  scheduled?: boolean
}

// 可合并为同一个气泡的文本流式 kind（同一 kind 的连续 chunk 拼接成一条）
const MERGEABLE_KINDS = new Set([
  'user_message_chunk',
  'agent_message_chunk',
  'agent_thought_chunk',
])

// groupMessages 将连续的同 kind 文本 chunk 合并为同一条消息，
// 使流式返回持续显示在同一个框内（参考 Cursor）。tool_call / plan / usage 等保持独立。
function groupMessages(messages: Message[]): Message[] {
  const grouped: Message[] = []
  for (const msg of messages) {
    const last = grouped[grouped.length - 1]
    if (
      last &&
      MERGEABLE_KINDS.has(msg.kind) &&
      last.kind === msg.kind &&
      last.role === msg.role
    ) {
      grouped[grouped.length - 1] = {
        ...last,
        content: last.content + msg.content,
        raw_json: last.raw_json && msg.raw_json
          ? `${last.raw_json}\n${msg.raw_json}`
          : last.raw_json || msg.raw_json,
      }
      continue
    }
    grouped.push(msg)
  }
  return grouped
}

// filterDisplay 分组后过滤掉无文本内容的消息（plan 例外）
function filterDisplay(messages: Message[]): Message[] {
  return groupMessages(messages).filter(
    (msg) => msg.content.trim() !== '' || msg.kind === 'plan',
  )
}

// ExecutionBlock 是按 execution_id 聚合的一组消息
interface ExecutionBlock {
  executionId: number
  startedAt: string
  messages: Message[]
}

// groupByExecution 按 execution_id 将消息分块。execution_id 为 null 的消息归入"无执行块"（手动）。
function groupByExecution(messages: Message[]): ExecutionBlock[] {
  const map = new Map<number, ExecutionBlock>()
  const order: number[] = []
  for (const msg of messages) {
    if (msg.execution_id == null) continue
    let block = map.get(msg.execution_id)
    if (!block) {
      block = {
        executionId: msg.execution_id,
        startedAt: msg.created_at,
        messages: [],
      }
      map.set(msg.execution_id, block)
      order.push(msg.execution_id)
    }
    block.messages.push(msg)
    if (msg.created_at < block.startedAt) block.startedAt = msg.created_at
  }
  // 按 execution_id 升序（最早执行在前）
  return order
    .sort((a, b) => a - b)
    .map((id) => map.get(id)!)
}

export default function MessageList({ messages, loading, scheduled }: MessageListProps) {
  const endRef = useRef<HTMLDivElement>(null)

  // 新消息时自动滚动到底部
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (!scheduled) {
    return <PlainList messages={messages} loading={loading} endRef={endRef} />
  }

  // 定时会话：按 execution_id 分块渲染
  const blocks = groupByExecution(messages)
  // 不属于任何执行块的消息（理论上定时会话不应出现）
  const unblocked = messages.filter((m) => m.execution_id == null)
  const scheduledLoading = !!loading

  return (
    <div className={styles.container}>
      {blocks.length === 0 && unblocked.length === 0 && !loading && (
        <div className={styles.empty}>
          <p>暂无执行记录</p>
        </div>
      )}
      {unblocked.length > 0 && (
        <PlainList messages={unblocked} loading={scheduledLoading} endRef={undefined} />
      )}
      {blocks.map((block, idx) => (
        <ExecutionBlockView
          key={block.executionId}
          block={block}
          index={idx + 1}
          total={blocks.length}
          loading={scheduledLoading && idx === blocks.length - 1}
        />
      ))}
      {scheduledLoading && blocks.length === 0 && (
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

// ExecutionBlockView 渲染单个执行块（可折叠）
function ExecutionBlockView({
  block,
  index,
  total,
  loading,
}: {
  block: ExecutionBlock
  index: number
  total: number
  loading: boolean
}) {
  // 最新一块默认展开，其余默认折叠
  const [open, setOpen] = useState(index === total)

  const displayMessages = filterDisplay(block.messages)

  // 找到最后一条思考消息
  let lastThoughtKey: string | number | null = null
  for (let i = displayMessages.length - 1; i >= 0; i--) {
    if (displayMessages[i].kind === 'agent_thought_chunk') {
      lastThoughtKey = displayMessages[i].id || displayMessages[i].sequence
      break
    }
  }

  return (
    <div className={styles.executionBlock}>
      <button
        type="button"
        className={styles.executionHeader}
        onClick={() => setOpen((v) => !v)}
      >
        <span className={styles.executionArrow}>{open ? '▼' : '▶'}</span>
        <span className={styles.executionTitle}>执行 #{index}</span>
        <span className={styles.executionTime}>
          {new Date(block.startedAt).toLocaleString('zh-CN')}
        </span>
        {loading && <span className={styles.executionRunning}>执行中…</span>}
      </button>
      {open && (
        <div className={styles.executionBody}>
          {displayMessages.map((msg) => {
            const key = msg.id || msg.sequence
            const isLastThoughtStreaming =
              !!loading &&
              msg.kind === 'agent_thought_chunk' &&
              key === lastThoughtKey
            return (
              <MessageBubble
                key={key}
                message={msg}
                defaultOpen={isLastThoughtStreaming}
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

// PlainList 渲染普通（非分块）消息列表
function PlainList({
  messages,
  loading,
  endRef,
}: {
  messages: Message[]
  loading?: boolean
  endRef?: React.RefObject<HTMLDivElement | null>
}) {
  const endRefObj = endRef as React.RefObject<HTMLDivElement> | undefined
  const displayMessages = filterDisplay(messages)

  let lastThoughtKey: string | number | null = null
  for (let i = displayMessages.length - 1; i >= 0; i--) {
    if (displayMessages[i].kind === 'agent_thought_chunk') {
      lastThoughtKey = displayMessages[i].id || displayMessages[i].sequence
      break
    }
  }

  return (
    <div className={styles.container}>
      {displayMessages.length === 0 && !loading && (
        <div className={styles.empty}>
          <p>暂无消息，发送 prompt 开始对话</p>
        </div>
      )}
      {displayMessages.map((msg) => {
        const key = msg.id || msg.sequence
        const isLastThoughtStreaming =
          !!loading &&
          msg.kind === 'agent_thought_chunk' &&
          key === lastThoughtKey
        return (
          <MessageBubble
            key={key}
            message={msg}
            defaultOpen={isLastThoughtStreaming}
          />
        )
      })}
      {loading && (
        <div className={styles.loading}>
          <span className={styles.dot} />
          <span className={styles.dot} />
          <span className={styles.dot} />
        </div>
      )}
      {endRefObj && <div ref={endRefObj} />}
    </div>
  )
}
