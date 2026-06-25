import { useMemo } from 'react'
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

export default function ContextStats({ messages }: ContextStatsProps) {
  const usage = useMemo(() => parseUsage(messages), [messages])

  if (!usage || usage.size <= 0) return null

  const percent = Math.min(100, (usage.used / usage.size) * 100)
  const isHigh = percent > 80
  const isMedium = percent > 50

  return (
    <div className={styles.container} title={`上下文窗口: ${usage.used} / ${usage.size} tokens`}>
      <span className={styles.label}>上下文</span>
      <div className={styles.bar}>
        <div
          className={`${styles.fill} ${isHigh ? styles.fillHigh : isMedium ? styles.fillMedium : ''}`}
          style={{ width: `${percent}%` }}
        />
      </div>
      <span className={`${styles.numbers} ${isHigh ? styles.numbersHigh : ''}`}>
        {formatTokens(usage.used)} / {formatTokens(usage.size)}
      </span>
    </div>
  )
}
