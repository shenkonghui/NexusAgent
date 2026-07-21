import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Bug, ChevronDown, RefreshCw } from 'lucide-react'
import {
  getDebugMeta,
  listDebugEvents,
  listDebugRaw,
  type DebugEvent,
  type DebugMeta,
  type DebugRaw,
} from '../api/sessions'
import styles from './DebugPanel.module.css'

type TabKey = 'events' | 'raw'

interface DebugPanelProps {
  sessionId: number
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number, len = 2) => String(n).padStart(len, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`
}

function prettyJSON(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

function methodOf(line: unknown): string {
  if (line && typeof line === 'object' && 'method' in line) {
    return String((line as { method?: string }).method || '')
  }
  return ''
}

function rawTypeOf(r: DebugRaw): string {
  return methodOf(r.line) || r.direction || ''
}

export default function DebugPanel({ sessionId }: DebugPanelProps) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<TabKey>('events')
  const [meta, setMeta] = useState<DebugMeta | null>(null)
  const [events, setEvents] = useState<DebugEvent[]>([])
  const [raw, setRaw] = useState<DebugRaw[]>([])
  const [rawTypeFilter, setRawTypeFilter] = useState('')
  const [autoScroll, setAutoScroll] = useState(true)
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [error, setError] = useState<string | null>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const eventSince = useRef(0)

  const loadMeta = useCallback(async () => {
    try {
      const res = await getDebugMeta(sessionId)
      setMeta(res.data)
      setError(null)
      return res.data
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      return null
    }
  }, [sessionId])

  const poll = useCallback(async () => {
    const m = await loadMeta()
    if (!m?.enabled) return
    try {
      if (tab === 'events') {
        const res = await listDebugEvents(sessionId, eventSince.current)
        const list = res.data.events || []
        if (list.length > 0) {
          eventSince.current += list.length
          setEvents((prev) => [...prev, ...list].slice(-500))
        }
      } else {
        // 后端最多保留 100 条，每次全量拉取覆盖
        const res = await listDebugRaw(sessionId, 0, 0)
        setRaw(res.data.raw || [])
      }
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [loadMeta, sessionId, tab])

  useEffect(() => {
    eventSince.current = 0
    setEvents([])
    setRaw([])
    setRawTypeFilter('')
    setExpanded({})
  }, [sessionId])

  useEffect(() => {
    void poll()
    const timer = setInterval(() => void poll(), 2000)
    return () => clearInterval(timer)
  }, [sessionId, tab, poll])

  const rawTypes = useMemo(() => {
    const set = new Set<string>()
    for (const r of raw) {
      const typ = rawTypeOf(r)
      if (typ) set.add(typ)
    }
    return Array.from(set).sort()
  }, [raw])

  // 最新在最上面；按事件类型过滤
  const displayRaw = useMemo(() => {
    const list = rawTypeFilter
      ? raw.filter((r) => rawTypeOf(r) === rawTypeFilter)
      : raw
    return list.slice().reverse()
  }, [raw, rawTypeFilter])

  useEffect(() => {
    if (!autoScroll) return
    const el = bodyRef.current
    if (!el) return
    // 原始报文最新在顶部，自动滚到顶部；事件仍滚到底部
    el.scrollTop = tab === 'raw' ? 0 : el.scrollHeight
  }, [events, displayRaw, autoScroll, tab])

  const toggleExpand = (key: string) => {
    setExpanded((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  if (meta && !meta.enabled) {
    return (
      <div className={styles.panel}>
        <div className={styles.empty}>
          <Bug size={20} style={{ opacity: 0.4, marginBottom: 8 }} />
          <div>{t('debug.disabledHint')}</div>
        </div>
      </div>
    )
  }

  const items = tab === 'events' ? events : displayRaw

  return (
    <div className={styles.panel}>
      <div className={styles.toolbar}>
        <div className={styles.meta}>
          <Bug size={13} />
          <span>
            {meta
              ? `events ${meta.event_count} · raw ${meta.raw_count}`
              : t('debug.title')}
          </span>
        </div>
        <div className={styles.tabGroup}>
          <button
            type="button"
            className={`${styles.tab} ${tab === 'events' ? styles.tabActive : ''}`}
            onClick={() => setTab('events')}
          >
            {t('debug.events')}
          </button>
          <button
            type="button"
            className={`${styles.tab} ${tab === 'raw' ? styles.tabActive : ''}`}
            onClick={() => setTab('raw')}
          >
            {t('debug.raw')}
          </button>
        </div>
        <div className={styles.actions}>
          {tab === 'raw' && (
            <select
              className={styles.typeSelect}
              value={rawTypeFilter}
              onChange={(e) => setRawTypeFilter(e.target.value)}
              title={t('debug.filterType')}
            >
              <option value="">{t('debug.filterAll')}</option>
              {rawTypes.map((typ) => (
                <option key={typ} value={typ}>
                  {typ}
                </option>
              ))}
            </select>
          )}
          <button
            type="button"
            className={`${styles.iconBtn} ${autoScroll ? styles.iconBtnActive : ''}`}
            onClick={() => setAutoScroll((v) => !v)}
            title={t('debug.autoScroll')}
          >
            <ChevronDown size={14} />
          </button>
          <button
            type="button"
            className={styles.iconBtn}
            onClick={() => void poll()}
            title={t('debug.refresh')}
          >
            <RefreshCw size={13} />
          </button>
        </div>
      </div>

      {error && <div className={styles.error}>{error}</div>}

      <div className={styles.body} ref={bodyRef}>
        {items.length === 0 ? (
          <div className={styles.empty}>
            <Bug size={20} style={{ opacity: 0.4, marginBottom: 8 }} />
            <div>{t('debug.empty')}</div>
          </div>
        ) : tab === 'events' ? (
          events.map((e, i) => {
            const key = `e-${i}-${e.ts}`
            const open = !!expanded[key]
            return (
              <div key={key} className={styles.entry}>
                <button type="button" className={styles.entryHead} onClick={() => toggleExpand(key)}>
                  <span className={styles.time}>{formatTime(e.ts)}</span>
                  <span className={styles.eventType}>{e.event}</span>
                  <span className={styles.preview}>{open ? '▾' : '▸'}</span>
                </button>
                {open && (
                  <pre className={styles.json}>{prettyJSON(e.detail ?? e)}</pre>
                )}
              </div>
            )
          })
        ) : (
          displayRaw.map((r, i) => {
            const key = `r-${i}-${r.ts}`
            const open = !!expanded[key]
            const isSend = r.direction === 'send'
            return (
              <div key={key} className={styles.entry}>
                <button type="button" className={styles.entryHead} onClick={() => toggleExpand(key)}>
                  <span className={styles.time}>{formatTime(r.ts)}</span>
                  <span className={`${styles.dir} ${isSend ? styles.dirSend : styles.dirRecv}`}>
                    {isSend ? '↑' : '↓'} {isSend ? t('debug.send') : t('debug.receive')}
                  </span>
                  <span className={styles.eventType}>{rawTypeOf(r)}</span>
                  <span className={styles.preview}>{open ? '▾' : '▸'}</span>
                </button>
                {open && <pre className={styles.json}>{prettyJSON(r.line)}</pre>}
              </div>
            )
          })
        )}
      </div>
    </div>
  )
}
