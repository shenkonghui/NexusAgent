import { useMemo, useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Eraser, ChevronDown, ChevronRight, Terminal, Wrench, Puzzle, Sparkles } from 'lucide-react'
import type { Message } from '../types'
import { clearContext } from '../api/sessions'
import {
  parseCategoryUsage,
  parseToolCalls,
  summarizeToolCalls,
  type ToolCategory,
} from '../utils/contextUsage'
import styles from './ContextStats.module.css'

interface ContextStatsProps {
  messages: Message[]
  sessionId?: number
  // 清理上下文成功后回调（供上层重新拉取消息，刷新 token 占用展示）
  onCleared?: () => void
}

// 从 usage_update 消息的 raw_json 中解析 token 用量
interface UsageData {
  size: number
  used: number
}

function parseUsage(messages: Message[]): UsageData | null {
  let latest: UsageData | null = null
  for (const msg of messages) {
    if (msg.kind !== 'usage_update') continue
    if (!msg.raw_json) continue
    try {
      const parsed = JSON.parse(msg.raw_json)
      if (typeof parsed.size === 'number' && typeof parsed.used === 'number') {
        latest = { size: parsed.size, used: parsed.used }
      }
    } catch {
      // 忽略解析失败
    }
  }
  return latest
}

// 格式化 token 数量
function formatTokens(n: number): string {
  if (n >= 1000) {
    return `${(n / 1000).toFixed(1)}K`
  }
  return String(n)
}

// 工具类别对应的图标
const categoryIcon: Record<ToolCategory, typeof Terminal> = {
  shell: Terminal,
  mcp: Puzzle,
  skill: Sparkles,
  tool: Wrench,
}

// 工具类别对应的 i18n 标签 key
const categoryLabelKey: Record<ToolCategory, string> = {
  shell: 'chat.toolShell',
  mcp: 'chat.toolMcp',
  skill: 'chat.toolSkill',
  tool: 'chat.toolGeneric',
}

const RADIUS = 9
const CIRC = 2 * Math.PI * RADIUS

export default function ContextStats({ messages, sessionId, onCleared }: ContextStatsProps) {
  const { t } = useTranslation()
  const usage = useMemo(() => parseUsage(messages), [messages])
  const categoryUsage = useMemo(() => parseCategoryUsage(messages), [messages])
  const toolCalls = useMemo(() => parseToolCalls(messages), [messages])
  const toolSummary = useMemo(() => summarizeToolCalls(toolCalls), [toolCalls])
  const [hover, setHover] = useState(false)
  const [open, setOpen] = useState(false)
  const [detailOpen, setDetailOpen] = useState(false)
  const [clearing, setClearing] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  // 点击浮层外部关闭已固定（点击打开）的浮层
  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
        setDetailOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  if (!usage || usage.size <= 0) return null

  const percent = Math.min(100, (usage.used / usage.size) * 100)
  // 环形进度：用过的部分绕一圈
  const dash = (percent / 100) * CIRC
  // 详情固定需点击展开，避免 hover 时误触；打开详情时锁定浮层为点击态
  const showPopover = hover || open
  const showDetail = detailOpen

  // 类别占比条：以类别之和为分母（更贴合明细语义）
  const catRows: { key: ToolCategory | 'systemPrompt' | 'conversation'; labelKey: string; value: number }[] = categoryUsage
    ? [
        { key: 'systemPrompt', labelKey: 'chat.catSystemPrompt', value: categoryUsage.systemPrompt },
        { key: 'tool', labelKey: 'chat.catTools', value: categoryUsage.tools },
        { key: 'mcp', labelKey: 'chat.catMcp', value: categoryUsage.mcp },
        { key: 'skill', labelKey: 'chat.catSkills', value: categoryUsage.skills },
        { key: 'conversation', labelKey: 'chat.catConversation', value: categoryUsage.conversation },
      ]
    : []
  const catMax = catRows.reduce((m, r) => Math.max(m, r.value), 0)

  async function handleClear() {
    if (!sessionId || clearing) return
    if (!window.confirm(t('chat.clearContextConfirm'))) return
    setClearing(true)
    try {
      await clearContext(sessionId)
      onCleared?.()
      setOpen(false)
    } catch {
      // 忽略：失败时不改变展示
    } finally {
      setClearing(false)
    }
  }

  return (
    <div
      ref={containerRef}
      className={styles.container}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
    >
      <button
        type="button"
        className={styles.trigger}
        title={t('chat.contextWindow')}
        aria-label={`${t('chat.contextWindow')}: ${usage.used} / ${usage.size} tokens`}
        onClick={() => setOpen((v) => !v)}
      >
        <svg width="22" height="22" viewBox="0 0 22 22" className={styles.pie}>
          {/* 底环（剩余空间） */}
          <circle cx="11" cy="11" r={RADIUS} fill="none" stroke="var(--border)" strokeWidth="3" />
          {/* 已用部分：黑色弧 */}
          <circle
            cx="11"
            cy="11"
            r={RADIUS}
            fill="none"
            stroke="currentColor"
            strokeWidth="3"
            strokeDasharray={`${dash} ${CIRC - dash}`}
            strokeLinecap="round"
            transform="rotate(-90 11 11)"
            className={styles.pieUsed}
          />
        </svg>
      </button>
      {showPopover && (
        <div className={styles.popover} role="tooltip">
          <div className={styles.popoverTitle}>{t('chat.contextWindow')}</div>
          <div className={styles.popoverRow}>
            <span className={styles.popoverLabel}>{t('chat.tokenUsage')}</span>
            <span className={styles.popoverValue}>{formatTokens(usage.used)} / {formatTokens(usage.size)}</span>
          </div>
          <div className={styles.popoverRow}>
            <span className={styles.popoverLabel}>{t('chat.usage')}</span>
            <span className={styles.popoverValue}>{percent.toFixed(1)}%</span>
          </div>
          <div className={styles.popoverDetail}>
            {usage.used.toLocaleString()} / {usage.size.toLocaleString()} tokens
          </div>

          {/* 查看详情：类别占用明细 + 工具调用记录 */}
          {(catRows.length > 0 || toolCalls.length > 0) && (
            <button
              type="button"
              className={styles.detailToggle}
              onClick={() => {
                // 锁定浮层为点击态，避免展开详情后因 hover 结束而消失
                setOpen(true)
                setDetailOpen((v) => !v)
              }}
            >
              {showDetail ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
              {t('chat.contextDetail')}
            </button>
          )}
          {showDetail && (
            <div className={styles.detailBody}>
              {catRows.length > 0 && (
                <div className={styles.detailSection}>
                  <div className={styles.detailSectionTitle}>{t('chat.contextByCategory')}</div>
                  {catRows.map((row) => (
                    <div key={row.key} className={styles.catRow}>
                      <span className={styles.catLabel}>{t(row.labelKey)}</span>
                      <span className={styles.catBarTrack}>
                        <span
                          className={styles.catBarFill}
                          style={{ width: `${catMax > 0 ? (row.value / catMax) * 100 : 0}%` }}
                        />
                      </span>
                      <span className={styles.catValue}>{formatTokens(row.value)}</span>
                    </div>
                  ))}
                </div>
              )}

              {toolCalls.length > 0 && (
                <div className={styles.detailSection}>
                  <div className={styles.detailSectionTitle}>
                    {t('chat.toolCalls')}
                    <span className={styles.detailCount}>{toolSummary.total}</span>
                  </div>
                  <div className={styles.toolList}>
                    {toolCalls.map((tc) => {
                      const Icon = categoryIcon[tc.category]
                      return (
                        <div key={tc.id} className={styles.toolItem} title={tc.command || tc.title}>
                          <Icon size={12} className={styles.toolIcon} />
                          <span className={styles.toolText}>{tc.command || tc.title || t(categoryLabelKey[tc.category])}</span>
                          <span className={`${styles.toolStatus} ${styles[`toolStatus_${tc.status}`] || ''}`} />
                        </div>
                      )
                    })}
                  </div>
                </div>
              )}
            </div>
          )}

          {sessionId && (
            <button
              type="button"
              className={styles.clearBtn}
              onClick={handleClear}
              disabled={clearing}
            >
              <Eraser size={13} />
              {clearing ? t('chat.clearingContext') : t('chat.clearContext')}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
