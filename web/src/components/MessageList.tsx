import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Message, Execution } from '../types'
import { aggregateChanges } from '../utils/diff'
import MessageBubble, { toolSummary } from './MessageBubble'
import ChangesSummary from './ChangesSummary'
import { ChevronDown, ChevronRight } from 'lucide-react'
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
    (msg) => (msg.content.trim() !== '' || msg.kind === 'plan') && msg.kind !== 'permission_request',
  )
}

function findLastUserIndex(messages: Message[]): number {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'user') return i
  }
  return -1
}

function isCollapsibleMessage(msg: Message): boolean {
  return msg.kind === 'agent_thought_chunk' || msg.role === 'tool'
}

function bubbleCollapseState(
  msg: Message,
  idx: number,
  lastUserIdx: number,
  loading: boolean,
  lastThoughtKey: string | number | null,
  key: string | number,
) {
  const inCurrentTurn = idx > lastUserIdx
  const collapsible = isCollapsibleMessage(msg)
  const isLastThoughtStreaming =
    !!loading &&
    inCurrentTurn &&
    msg.kind === 'agent_thought_chunk' &&
    key === lastThoughtKey
  const forceCollapsed = collapsible && (!inCurrentTurn || !loading)
  return { defaultOpen: isLastThoughtStreaming, forceCollapsed }
}

// 将连续的可折叠消息（思考 + 工具调用）聚合成一个分组段。
// 单条可折叠消息仍作为独立段渲染，仅当连续出现 2 条及以上时才分组，
// 这样既能压缩冗长的工具/思考序列，又不破坏单条消息的原有展示。
type Segment =
  | { type: 'single'; message: Message; idx: number }
  | { type: 'group'; messages: Message[]; firstIdx: number }

function segmentMessages(messages: Message[]): Segment[] {
  const segments: Segment[] = []
  let group: Message[] = []
  let groupStart = 0
  const flushGroup = () => {
    if (group.length >= 2) {
      segments.push({ type: 'group', messages: group, firstIdx: groupStart })
    } else if (group.length === 1) {
      segments.push({ type: 'single', message: group[0], idx: groupStart })
    }
    group = []
  }
  messages.forEach((msg, idx) => {
    if (isCollapsibleMessage(msg)) {
      if (group.length === 0) groupStart = idx
      group.push(msg)
    } else {
      flushGroup()
      segments.push({ type: 'single', message: msg, idx })
    }
  })
  flushGroup()
  return segments
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

// 计算分组内思考与工具调用的数量，以及最后一个工具调用的摘要
function groupStats(messages: Message[]) {
  let thoughtCount = 0
  let toolCount = 0
  let lastToolSummary = ''
  for (const m of messages) {
    if (m.kind === 'agent_thought_chunk') {
      thoughtCount++
    } else if (m.role === 'tool') {
      toolCount++
      lastToolSummary = toolSummary(m.content)
    }
  }
  return { thoughtCount, toolCount, lastToolSummary }
}

function findLastThoughtKey(messages: Message[]): string | number | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].kind === 'agent_thought_chunk') {
      return messages[i].id || messages[i].sequence
    }
  }
  return null
}

// 计算每条消息所属的对话轮次索引（用户消息开启新一轮）。
// 返回与 messages 等长的数组，turnOfMsg[i] = 第 i 条消息的轮次号（从 0 起）。
function computeTurnOfMsg(messages: Message[]): number[] {
  const turns: number[] = []
  let cur = -1
  for (const msg of messages) {
    if (msg.role === 'user') cur++
    turns.push(cur < 0 ? 0 : cur)
  }
  return turns
}

// 计算每个 segment 所属的轮次号，以及「每个轮次的最后一个 segment 索引」。
// segment 的轮次由其首条消息决定；turnEndSeg 是 turn -> 最后一个 segment idx 的映射。
function mapSegmentsToTurns(
  segments: Segment[],
  turnOfMsg: number[],
): { turnOfSeg: number[]; turnEndSeg: Map<number, number> } {
  const turnOfSeg: number[] = segments.map((seg) => {
    const firstMsgIdx = seg.type === 'single' ? seg.idx : seg.firstIdx
    return turnOfMsg[firstMsgIdx] ?? 0
  })
  const turnEndSeg = new Map<number, number>()
  turnOfSeg.forEach((turn, segIdx) => {
    turnEndSeg.set(turn, segIdx) // 同一轮次后出现的 segment 覆盖前者
  })
  return { turnOfSeg, turnEndSeg }
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
  const lastThoughtKey = findLastThoughtKey(displayMessages)

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
        <span className={styles.executionArrow}>{open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}</span>
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
          <SegmentList
            messages={displayMessages}
            lastUserIdx={findLastUserIndex(displayMessages)}
            loading={loading}
            lastThoughtKey={lastThoughtKey}
            sessionId={sessionId}
            cwd={cwd}
          />
          {status === 'failed' && errorMsg && (
            <div className={styles.execError}>{t('status.error')}：{errorMsg}</div>
          )}
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
  const lastUserIdx = findLastUserIndex(displayMessages)
  const lastThoughtKey = findLastThoughtKey(displayMessages)

  return (
    <div className={styles.container}>
      {displayMessages.length === 0 && !loading && (
        <div className={styles.empty}>
          <p>{t('chat.noMessages')}</p>
        </div>
      )}
      <SegmentList
        messages={displayMessages}
        lastUserIdx={lastUserIdx}
        loading={!!loading}
        lastThoughtKey={lastThoughtKey}
        sessionId={sessionId}
        cwd={cwd}
      />
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

// 根据 segment 列表渲染：单条消息走 MessageBubble，连续可折叠消息走 CollapsibleGroup
function SegmentList({
  messages,
  lastUserIdx,
  loading,
  lastThoughtKey,
  sessionId,
  cwd,
}: {
  messages: Message[]
  lastUserIdx: number
  loading: boolean
  lastThoughtKey: string | number | null
  sessionId?: number
  cwd?: string
}) {
  const segments = segmentMessages(messages)
  const lastMsg = messages[messages.length - 1]
  const lastKey = lastMsg ? (lastMsg.id || lastMsg.sequence) : null

  // 对话轮次检测：按用户消息切分轮次，找出每个轮次的末尾 segment。
  const { turnOfSeg, turnEndSeg } = useMemo(
    () => mapSegmentsToTurns(segments, computeTurnOfMsg(messages)),
    [segments, messages],
  )

  // 每轮的文件改动汇总（cwd 缺失时无法计算相对路径，跳过）。
  const turnChanges = useMemo(() => {
    const map = new Map<number, ReturnType<typeof aggregateChanges>>()
    if (!cwd) return map
    const turnOfMsg = computeTurnOfMsg(messages)
    const byTurn = new Map<number, Message[]>()
    messages.forEach((msg, i) => {
      const turn = turnOfMsg[i]
      const arr = byTurn.get(turn)
      if (arr) arr.push(msg)
      else byTurn.set(turn, [msg])
    })
    for (const [turn, msgs] of byTurn) {
      const changes = aggregateChanges(msgs, cwd)
      if (changes.length > 0) map.set(turn, changes)
    }
    return map
  }, [messages, cwd])

  return (
    <>
      {segments.map((seg, segIdx) => {
        // 计算该 segment 的主体（单条消息气泡 或 可折叠分组）
        let body: React.ReactNode
        if (seg.type === 'single') {
          const msg = seg.message
          const key = msg.id || msg.sequence
          const { defaultOpen, forceCollapsed } = bubbleCollapseState(
            msg, seg.idx, lastUserIdx, loading, lastThoughtKey, key,
          )
          const isStreamingAssistant =
            !!loading &&
            msg.kind === 'agent_message_chunk' &&
            key === lastKey
          body = (
            <MessageBubble
              message={msg}
              defaultOpen={defaultOpen}
              forceCollapsed={forceCollapsed}
              streaming={isStreamingAssistant}
              sessionId={sessionId}
              cwd={cwd}
            />
          )
        } else {
          const isLastSegment = segIdx === segments.length - 1
          const inCurrentTurn = seg.firstIdx > lastUserIdx
          body = (
            <CollapsibleGroup
              messages={seg.messages}
              inCurrentTurn={inCurrentTurn}
              isLastSegment={isLastSegment}
              loading={loading}
              sessionId={sessionId}
              cwd={cwd}
            />
          )
        }

        // 该轮次末尾追加文件改动汇总卡片（仅有改动时显示）
        const turn = turnOfSeg[segIdx]
        const isTurnEnd = turnEndSeg.get(turn) === segIdx
        const changes = isTurnEnd ? turnChanges.get(turn) : undefined

        return (
          <Fragment key={`seg-${segIdx}`}>
            {body}
            {changes && changes.length > 0 && (
              <ChangesSummary changes={changes} sessionId={sessionId} cwd={cwd} />
            )}
          </Fragment>
        )
      })}
    </>
  )
}

// 连续思考 + 工具调用消息的折叠分组。
// 流式中（本轮末尾分组）默认展开以展示实时进度；
// 助手返回回复或流式结束后自动压缩为一行摘要（如「思考中 ×2 · 工具调用 ×3」）。
function CollapsibleGroup({
  messages,
  inCurrentTurn,
  isLastSegment,
  loading,
  sessionId,
  cwd,
}: {
  messages: Message[]
  inCurrentTurn: boolean
  isLastSegment: boolean
  loading: boolean
  sessionId?: number
  cwd?: string
}) {
  const { t } = useTranslation()

  const forceCollapsed = !inCurrentTurn || !loading
  const streaming = loading && inCurrentTurn && isLastSegment
  const defaultOpen = streaming

  const [open, setOpen] = useState(defaultOpen && !forceCollapsed)
  useEffect(() => {
    if (forceCollapsed) {
      setOpen(false)
    } else {
      setOpen(defaultOpen)
    }
  }, [defaultOpen, forceCollapsed])

  const { thoughtCount, toolCount, lastToolSummary } = groupStats(messages)
  const parts: string[] = []
  if (thoughtCount > 0) parts.push(`${t('chat.thinking')} ×${thoughtCount}`)
  if (toolCount > 0) parts.push(`${t('chat.toolCall')} ×${toolCount}`)
  const summary = parts.join(' · ') || t('chat.toolCall')

  const localLastThoughtKey = findLastThoughtKey(messages)

  return (
    <div className={styles.groupContainer}>
      <div
        className={`${styles.groupHeader} ${open ? styles.groupHeaderOpen : ''}`}
        onClick={() => setOpen((v) => !v)}
      >
        <span className={styles.groupToggle}>{open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}</span>
        <span className={styles.groupSummary}>
          {summary}
          {toolCount > 0 && lastToolSummary && (
            <span className={styles.groupLastTool}> · {t(lastToolSummary)}</span>
          )}
        </span>
        {streaming && !forceCollapsed && <span className={styles.groupSpinner} />}
        <span className={styles.groupToggleHint}>
          {open ? t('chat.collapse') : t('chat.expand')}
        </span>
      </div>
      {open && (
        <div className={styles.groupBody}>
          {messages.map((msg) => {
            const key = msg.id || msg.sequence
            const isLocalLastThought =
              msg.kind === 'agent_thought_chunk' && key === localLastThoughtKey
            const innerDefaultOpen = streaming && isLocalLastThought
            return (
              <MessageBubble
                key={key}
                message={msg}
                defaultOpen={innerDefaultOpen}
                forceCollapsed={false}
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
