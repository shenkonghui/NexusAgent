import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Message, Execution } from '../types'
import MessageBubble from './MessageBubble'
import styles from './MessageList.module.css'

interface MessageListProps {
  messages: Message[]
  loading?: boolean
  scheduled?: boolean
  executions?: Execution[]
  sessionId?: number
  cwd?: string
}

const MERGEABLE_KINDS = new Set([
  'user_message_chunk',
  'agent_message_chunk',
  'agent_thought_chunk',
])

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

function filterDisplay(messages: Message[]): Message[] {
  return groupMessages(messages).filter(
    (msg) => msg.content.trim() !== '' || msg.kind === 'plan',
  )
}

interface ExecutionBlock {
  executionId: number
  startedAt: string
  messages: Message[]
}

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
  return order
    .sort((a, b) => a - b)
    .map((id) => map.get(id)!)
}

export default function MessageList({ messages, loading, scheduled, executions, sessionId, cwd }: MessageListProps) {
  const { t } = useTranslation()
  const endRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (!scheduled) {
    return <PlainList messages={messages} loading={loading} endRef={endRef} sessionId={sessionId} cwd={cwd} />
  }

  const blocks = groupByExecution(messages)
  const unblocked = messages.filter((m) => m.execution_id == null)
  const scheduledLoading = !!loading
  const execStatusMap = new Map<number, Execution>()
  if (executions) {
    for (const e of executions) {
      execStatusMap.set(e.execution_id, e)
    }
  }

  return (
    <div className={styles.container}>
      {blocks.length === 0 && unblocked.length === 0 && !loading && (
        <div className={styles.empty}>
          <p>{t('scheduledTask.noTasks')}</p>
        </div>
      )}
      {unblocked.length > 0 && (
        <PlainList messages={unblocked} loading={scheduledLoading} endRef={undefined} sessionId={sessionId} cwd={cwd} />
      )}
      {blocks.map((block, idx) => (
        <ExecutionBlockView
          key={block.executionId}
          block={block}
          index={idx + 1}
          total={blocks.length}
          loading={scheduledLoading && idx === blocks.length - 1}
          status={execStatusMap.get(block.executionId)?.status || ''}
          errorMsg={execStatusMap.get(block.executionId)?.error || ''}
          sessionId={sessionId}
          cwd={cwd}
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

function ExecutionBlockView({
  block,
  index,
  total,
  loading,
  status,
  errorMsg,
  sessionId,
  cwd,
}: {
  block: ExecutionBlock
  index: number
  total: number
  loading: boolean
  status: string
  errorMsg: string
  sessionId?: number
  cwd?: string
}) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(index === total)
  const displayMessages = filterDisplay(block.messages)

  let lastThoughtKey: string | number | null = null
  for (let i = displayMessages.length - 1; i >= 0; i--) {
    if (displayMessages[i].kind === 'agent_thought_chunk') {
      lastThoughtKey = displayMessages[i].id || displayMessages[i].sequence
      break
    }
  }

  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'
  const execStatusMap: Record<string, string> = {
    success: t('scheduledTask.statusSuccess'),
    running: t('scheduledTask.statusRunning'),
    failed: t('scheduledTask.statusFailed'),
    skipped: t('scheduledTask.statusCancelled'),
  }

  return (
    <div className={styles.executionBlock}>
      <button
        type="button"
        className={styles.executionHeader}
        onClick={() => setOpen((v) => !v)}
      >
        <span className={styles.executionArrow}>{open ? '▼' : '▶'}</span>
        <span className={styles.executionTitle}>{t('scheduledTask.execution')} #{index}</span>
        <span className={styles.executionTime}>
          {new Date(block.startedAt).toLocaleString(locale)}
        </span>
        {status && (
          <span className={`${styles.execStatusBadge} ${styles[`execStatus_${status}`] || ''}`}>
            {execStatusMap[status] || status}
          </span>
        )}
        {loading && <span className={styles.executionRunning}>{t('scheduledTask.statusRunning')}</span>}
      </button>
      {open && (
        <div className={styles.executionBody}>
          {status === 'failed' && errorMsg && (
            <div className={styles.execError}>{t('status.error')}：{errorMsg}</div>
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
                sessionId={sessionId}
                cwd={cwd}
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

function PlainList({
  messages,
  loading,
  endRef,
  sessionId,
  cwd,
}: {
  messages: Message[]
  loading?: boolean
  endRef?: React.RefObject<HTMLDivElement | null>
  sessionId?: number
  cwd?: string
}) {
  const { t } = useTranslation()
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
          <p>{t('chat.noMessages')}</p>
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
            sessionId={sessionId}
            cwd={cwd}
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
