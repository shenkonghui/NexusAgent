import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Message } from '../types'
import styles from './ContextStats.module.css'

interface ContextStatsProps {
  messages: Message[]
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

const RADIUS = 9
const CIRC = 2 * Math.PI * RADIUS

export default function ContextStats({ messages }: ContextStatsProps) {
  const { t } = useTranslation()
  const usage = useMemo(() => parseUsage(messages), [messages])
  const [hover, setHover] = useState(false)

  if (!usage || usage.size <= 0) return null

  const percent = Math.min(100, (usage.used / usage.size) * 100)
  // 环形进度：用过的部分绕一圈
  const dash = (percent / 100) * CIRC

  return (
    <div
      className={styles.container}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
    >
      <div
        className={styles.trigger}
        title={t('chat.contextWindow')}
        aria-label={`${t('chat.contextWindow')}: ${usage.used} / ${usage.size} tokens`}
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
      </div>
      {hover && (
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
        </div>
      )}
    </div>
  )
}
