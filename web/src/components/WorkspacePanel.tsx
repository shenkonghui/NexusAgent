import { useState, useEffect } from 'react'
import type { ReactNode } from 'react'
import FilePanel from './FilePanel'
import ChangesPanel from './ChangesPanel'
import TerminalPanel from './Terminal'
import DebugPanel from './DebugPanel'
import { getDebugMeta } from '../api/sessions'
import { listFileChanges } from '../api/filesystem'
import { Folder, SquareTerminal, Pencil, PanelRightClose, Bug } from 'lucide-react'
import styles from './WorkspacePanel.module.css'

type TabKey = 'files' | 'terminal' | 'changes' | 'debug'

interface WorkspacePanelProps {
  sessionId: number
  cwd?: string
  /** 关闭整个工作区面板 */
  onClose: () => void
  /** 恢复操作后的刷新触发器（变化时重新拉取改动数据） */
  refreshKey?: number
}

// WorkspacePanel 是右侧工作区面板，参考 OpenHands 用 Tab 切换文件/终端/改动。
// 各子面板的 onClose 由本组件接管（切换 Tab 而非真正关闭整个面板）。
export default function WorkspacePanel({
  sessionId,
  cwd,
  onClose,
  refreshKey,
}: WorkspacePanelProps) {
  const [activeTab, setActiveTab] = useState<TabKey>('files')
  const [debugEnabled, setDebugEnabled] = useState(false)
  const [changesCount, setChangesCount] = useState(0)

  // 从后端获取改动文件数（基于持久化快照消息）
  useEffect(() => {
    let cancelled = false
    listFileChanges(sessionId)
      .then((res) => {
        if (!cancelled) setChangesCount(res.data.count)
      })
      .catch(() => {
        if (!cancelled) setChangesCount(0)
      })
    return () => { cancelled = true }
  }, [sessionId, refreshKey])

  // 改动数 > 0 时，若用户尚未手动选择过，自动切到 changes 提示（仅一次）
  const [autoSwitched, setAutoSwitched] = useState(false)
  useEffect(() => {
    if (changesCount > 0 && !autoSwitched) {
      setAutoSwitched(true)
    }
  }, [changesCount, autoSwitched])

  useEffect(() => {
    let cancelled = false
    getDebugMeta(sessionId)
      .then((res) => {
        if (!cancelled) setDebugEnabled(!!res.data?.enabled)
      })
      .catch(() => {
        if (!cancelled) setDebugEnabled(false)
      })
    return () => {
      cancelled = true
    }
  }, [sessionId])

  const tabs: { key: TabKey; label: string; icon: ReactNode; badge?: number }[] = [
    { key: 'files', label: '文件', icon: <Folder size={14} /> },
    { key: 'terminal', label: '终端', icon: <SquareTerminal size={14} /> },
    { key: 'changes', label: '改动', icon: <Pencil size={14} />, badge: changesCount },
  ]
  if (debugEnabled) {
    tabs.push({ key: 'debug', label: '调试', icon: <Bug size={14} /> })
  }

  return (
    <div className={styles.workspace}>
      {/* Tab 导航 */}
      <div className={styles.tabNav}>
        {tabs.map((t) => (
          <button
            key={t.key}
            type="button"
            className={`${styles.tab} ${activeTab === t.key ? styles.tabActive : ''}`}
            onClick={() => setActiveTab(t.key)}
          >
            <span className={styles.tabIcon}>{t.icon}</span>
            {t.label}
            {t.badge ? <span className={styles.tabBadge}>{t.badge}</span> : null}
          </button>
        ))}
        <div className={styles.tabActions}>
          <button
            type="button"
            className={styles.collapseBtn}
            onClick={onClose}
            title="折叠工作区"
          >
            <PanelRightClose size={16} />
          </button>
        </div>
      </div>

      {/* Tab 内容：保持各子面板挂载（terminal 的 WebSocket 不中断），
          用 display 控制可见性 */}
      <div className={styles.tabBody}>
        <div style={{ display: activeTab === 'files' ? 'flex' : 'none', width: '100%', height: '100%', flexDirection: 'column' }}>
          <FilePanel sessionId={sessionId} onClose={() => setActiveTab('terminal')} />
        </div>
        <div style={{ display: activeTab === 'terminal' ? 'flex' : 'none', width: '100%', height: '100%', flexDirection: 'column' }}>
          <TerminalPanel sessionId={sessionId} onClose={() => setActiveTab('files')} />
        </div>
        <div style={{ display: activeTab === 'changes' ? 'flex' : 'none', width: '100%', height: '100%', flexDirection: 'column' }}>
          {cwd ? (
            <ChangesPanel
              sessionId={sessionId}
              onClose={() => setActiveTab('files')}
              refreshKey={refreshKey}
            />
          ) : (
            <div style={{ padding: 24, color: 'var(--text-muted)', textAlign: 'center' }}>
              无工作目录信息
            </div>
          )}
        </div>
        {debugEnabled && (
          <div style={{ display: activeTab === 'debug' ? 'flex' : 'none', width: '100%', height: '100%', flexDirection: 'column' }}>
            <DebugPanel sessionId={sessionId} />
          </div>
        )}
      </div>
    </div>
  )
}
