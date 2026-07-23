import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, CheckCircle2, XCircle } from 'lucide-react'
import { listAgents, listAgentStatus, probeAgentConfigs } from '../api/agents'
import type { Agent } from '../types'
import styles from './StartupWarmup.module.css'

// 每次「打开应用」（新标签页/新窗口/重载）展示一次；SPA 内导航不重复弹出。
const WARMUP_DONE_KEY = 'nexus_warmup_done'

// 轮询连接状态的间隔。比侧边栏的 3s 更快，因为这是启动关键路径，需要尽快感知「已连接」。
const STATUS_POLL_INTERVAL = 1000
// 连接超时：超过此时间仍未全部连接，标记失败并关闭弹窗（不阻塞用户）。
const CONNECT_TIMEOUT = 30_000

type WarmupState = 'connecting' | 'ready' | 'failed'

interface AgentWarmup {
  type: string
  name: string
  state: WarmupState
}

// StartupWarmup 启动预热弹窗：等待所有 ACP server 完成「连接」（与左下角状态栏同源），
// 全部连接就绪后自动关闭，也可随时点击「跳过」。
//
// 判定就绪的数据源是 GET /agents/status（读连接池状态），一旦连接握手完成即为 connected，
// 与左下角状态栏完全一致，避免出现「左下角已连接、弹窗还在转」的不同步。
// agent 配置探测（probe）改为后台静默触发，仅用于预热缓存，不阻塞弹窗关闭。
export default function StartupWarmup() {
  const { t } = useTranslation()
  const [visible, setVisible] = useState(false)
  const [items, setItems] = useState<AgentWarmup[]>([])
  const aliveRef = useRef(true)

  useEffect(() => {
    if (sessionStorage.getItem(WARMUP_DONE_KEY)) return
    aliveRef.current = true
    let pollTimer: ReturnType<typeof setInterval> | undefined
    let timeoutTimer: ReturnType<typeof setTimeout> | undefined
    let closeTimer: ReturnType<typeof setTimeout> | undefined
    let closed = false

    // finishOnce 在全部进入终态时延时关闭弹窗，保证只触发一次。
    const finishOnce = () => {
      if (closed || !aliveRef.current) return
      closed = true
      sessionStorage.setItem(WARMUP_DONE_KEY, '1')
      if (pollTimer) { clearInterval(pollTimer); pollTimer = undefined }
      closeTimer = setTimeout(() => {
        if (aliveRef.current) setVisible(false)
      }, 600)
    }

    async function run() {
      let agents: Agent[] = []
      try {
        const resp = await listAgents()
        agents = resp.data.agents || []
      } catch {
        agents = []
      }
      if (!aliveRef.current) return
      if (agents.length === 0) {
        sessionStorage.setItem(WARMUP_DONE_KEY, '1')
        return
      }

      setItems(
        agents.map((a) => ({
          type: a.type,
          name: a.display_name || a.type,
          state: 'connecting' as WarmupState,
        })),
      )
      setVisible(true)

      // 后台静默探测 agent 配置（预热缓存），不阻塞弹窗关闭。
      // probe 内部会复用已建立的共享连接，失败不影响弹窗判定。
      agents.forEach((a) => {
        probeAgentConfigs(a.type).catch(() => {})
      })

      // 轮询连接状态：连接成功即标记 ready，与左下角状态栏同源同步。
      const checkStatus = async () => {
        try {
          const r = await listAgentStatus()
          if (!aliveRef.current) return
          const map = new Map((r.data.agents || []).map((s) => [s.agent_type, s.status]))
          setItems((prev) => {
            // 计算更新后的状态，并判断是否全部进入终态
            const next = prev.map((it) => {
              const st = map.get(it.type)
              if (st === 'connected') return { ...it, state: 'ready' as WarmupState }
              return it
            })
            if (next.every((it) => it.state !== 'connecting')) finishOnce()
            return next
          })
        } catch {
          /* 状态拉取失败，等下一轮重试 */
        }
      }
      await checkStatus()
      if (!closed) pollTimer = setInterval(checkStatus, STATUS_POLL_INTERVAL)

      // 超时兜底：超过 CONNECT_TIMEOUT 仍未全部就绪，把剩余 connecting 标记为 failed 并关闭。
      timeoutTimer = setTimeout(() => {
        if (!aliveRef.current || closed) return
        setItems((prev) => prev.map((it) => (it.state === 'connecting' ? { ...it, state: 'failed' as WarmupState } : it)))
        finishOnce()
      }, CONNECT_TIMEOUT)
    }

    void run()

    return () => {
      aliveRef.current = false
      if (pollTimer) clearInterval(pollTimer)
      if (timeoutTimer) clearTimeout(timeoutTimer)
      if (closeTimer) clearTimeout(closeTimer)
    }
  }, [])

  // 跳过：立即关闭弹窗，后台探测继续（结果会进入缓存），本次会话不再弹出。
  function handleSkip() {
    sessionStorage.setItem(WARMUP_DONE_KEY, '1')
    setVisible(false)
  }

  if (!visible) return null

  const readyCount = items.filter((it) => it.state !== 'connecting').length
  const allDone = items.length > 0 && items.every((it) => it.state !== 'connecting')

  return (
    <div className={styles.overlay}>
      <div className={styles.dialog}>
        <h3 className={styles.title}>{t('warmup.title')}</h3>
        <p className={styles.subtitle}>{t('warmup.subtitle')}</p>
        <ul className={styles.list}>
          {items.map((it) => (
            <li key={it.type} className={styles.item}>
              <span className={styles.itemIcon}>
                {it.state === 'connecting' && <Loader2 size={16} className={styles.spin} />}
                {it.state === 'ready' && <CheckCircle2 size={16} className={styles.ok} />}
                {it.state === 'failed' && <XCircle size={16} className={styles.err} />}
              </span>
              <span className={styles.itemName}>{it.name}</span>
              <span className={styles.itemStatus}>
                {it.state === 'connecting' && t('warmup.connecting')}
                {it.state === 'ready' && t('warmup.ready')}
                {it.state === 'failed' && t('warmup.failed')}
              </span>
            </li>
          ))}
        </ul>
        <div className={styles.footer}>
          <span className={styles.progress}>
            {allDone ? t('warmup.done') : t('warmup.progress', { ready: readyCount, total: items.length })}
          </span>
          <button type="button" className={styles.skipBtn} onClick={handleSkip}>
            {t('warmup.skip')}
          </button>
        </div>
      </div>
    </div>
  )
}
