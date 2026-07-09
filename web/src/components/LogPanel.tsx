import { useState, useEffect, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Monitor, Server, Trash2, ChevronDown, ScrollText, X } from 'lucide-react'
import type { LogEntry, LogLevel } from '../types'
import { logger } from '../utils/logger'
import { streamBackendLogs } from '../api/logs'
import styles from './LogPanel.module.css'

type TabKey = 'frontend' | 'backend'

interface LogPanelProps {
  onClose: () => void
}

const LEVEL_ORDER: Record<LogLevel, number> = {
  debug: 10,
  info: 20,
  warn: 30,
  error: 40,
}

// level → CSS module 类名映射（避免动态键访问的 TS 类型问题）
const LEVEL_CLASS: Record<LogLevel, string> = {
  debug: styles.levelDebug,
  info: styles.levelInfo,
  warn: styles.levelWarn,
  error: styles.levelError,
}

// 格式化 ISO 时间为 HH:MM:SS.mmm，便于日志行紧凑展示
function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number, len = 2) => String(n).padStart(len, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
}

export default function LogPanel({ onClose }: LogPanelProps) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<TabKey>('frontend')

  // 前端日志状态
  const [frontendLogs, setFrontendLogs] = useState<LogEntry[]>(() => logger.getAll())

  // 后端日志状态
  const [backendLogs, setBackendLogs] = useState<LogEntry[]>([])
  const [backendError, setBackendError] = useState<string | null>(null)
  const lastBackendSeqRef = useRef(0)

  // 过滤与滚动设置（两个 Tab 共用）
  const [minLevel, setMinLevel] = useState<LogLevel>('debug')
  const [autoScroll, setAutoScroll] = useState(true)

  const bodyRef = useRef<HTMLDivElement>(null)

  // 订阅前端 logger 单例
  useEffect(() => {
    const unsub = logger.subscribe((entry) => {
      setFrontendLogs((prev) => {
        const next = prev.length >= 500 ? prev.slice(prev.length - 499) : prev
        return [...next, entry]
      })
    })
    return unsub
  }, [])

  // 后端 Tab 激活时建立 SSE 连接，切走/卸载时断开
  useEffect(() => {
    if (tab !== 'backend') return

    setBackendError(null)
    const cleanup = streamBackendLogs({
      onLog: (entry) => {
        if (entry.seq > lastBackendSeqRef.current) {
          lastBackendSeqRef.current = entry.seq
        }
        setBackendLogs((prev) => {
          const next = prev.length >= 500 ? prev.slice(prev.length - 499) : prev
          return [...next, entry]
        })
      },
      onError: (err) => {
        setBackendError(err.message)
      },
      level: minLevel,
      since: lastBackendSeqRef.current,
    })
    return cleanup
    // minLevel 变化时重连（重新订阅以应用新的等级过滤）
  }, [tab, minLevel])

  // 自动滚动到底部
  useEffect(() => {
    if (!autoScroll) return
    const el = bodyRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [frontendLogs, backendLogs, autoScroll, tab])

  const logs = tab === 'frontend' ? frontendLogs : backendLogs
  const filtered = minLevel === 'debug' ? logs : logs.filter((e) => LEVEL_ORDER[e.level] >= LEVEL_ORDER[minLevel])

  const handleClear = useCallback(() => {
    if (tab === 'frontend') {
      logger.clear()
      setFrontendLogs([])
    } else {
      setBackendLogs([])
      lastBackendSeqRef.current = 0
    }
  }, [tab])

  return createPortal(
    <div className={styles.panel}>
      {/* 工具栏 */}
      <div className={styles.toolbar}>
        <div className={styles.title}>
          <ScrollText size={14} />
          <span>{t('log.title')}</span>
        </div>
        <div className={styles.tabGroup}>
          <button
            type="button"
            className={`${styles.tab} ${tab === 'frontend' ? styles.tabActive : ''}`}
            onClick={() => setTab('frontend')}
            title={t('log.frontend')}
          >
            <Monitor size={13} />
            <span>{t('log.frontend')}</span>
          </button>
          <button
            type="button"
            className={`${styles.tab} ${tab === 'backend' ? styles.tabActive : ''}`}
            onClick={() => setTab('backend')}
            title={t('log.backend')}
          >
            <Server size={13} />
            <span>{t('log.backend')}</span>
          </button>
        </div>

        <div className={styles.actions}>
          <select
            className={styles.levelSelect}
            value={minLevel}
            onChange={(e) => setMinLevel(e.target.value as LogLevel)}
            title={t('log.filter')}
          >
            <option value="debug">{t('log.level.debug')}</option>
            <option value="info">{t('log.level.info')}</option>
            <option value="warn">{t('log.level.warn')}</option>
            <option value="error">{t('log.level.error')}</option>
          </select>
          <button
            type="button"
            className={`${styles.iconBtn} ${autoScroll ? styles.iconBtnActive : ''}`}
            onClick={() => setAutoScroll((v) => !v)}
            title={t('log.autoScroll')}
          >
            <ChevronDown size={15} />
          </button>
          <button
            type="button"
            className={styles.iconBtn}
            onClick={handleClear}
            title={t('log.clear')}
          >
            <Trash2 size={14} />
          </button>
          <button
            type="button"
            className={styles.iconBtn}
            onClick={onClose}
            title={t('common.close')}
          >
            <X size={15} />
          </button>
        </div>
      </div>

      {/* 日志列表 */}
      <div className={styles.body} ref={bodyRef}>
        {filtered.length === 0 ? (
          <div className={styles.empty}>
            <ScrollText size={20} style={{ opacity: 0.4, marginBottom: 8 }} />
            <div>{t('log.empty')}</div>
          </div>
        ) : (
          filtered.map((e) => (
            <div key={`${tab}-${e.seq}`} className={styles.entry}>
              <span className={styles.time}>{formatTime(e.timestamp)}</span>
              <span className={`${styles.level} ${LEVEL_CLASS[e.level]}`}>
                {e.level}
              </span>
              {e.source && <span className={styles.source} title={e.source}>{e.source}</span>}
              <span className={styles.message}>{e.message}</span>
            </div>
          ))
        )}
      </div>

      {/* 后端连接错误提示 */}
      {tab === 'backend' && backendError && (
        <div className={styles.statusBar}>{backendError}</div>
      )}
    </div>,
    document.body,
  )
}
