import { useState, useEffect } from 'react'
import type { Message } from '../types'
import FilePanel from './FilePanel'
import ChangesPanel from './ChangesPanel'
import TerminalPanel from './Terminal'
import styles from './WorkspacePanel.module.css'

type TabKey = 'files' | 'terminal' | 'changes'

interface WorkspacePanelProps {
  sessionId: number
  cwd?: string
  messages: Message[]
  /** 改动文件数量（用于徽标） */
  changesCount: number
  /** 关闭整个工作区面板 */
  onClose: () => void
}

// WorkspacePanel 是右侧工作区面板，参考 OpenHands 用 Tab 切换文件/终端/改动。
// 各子面板的 onClose 由本组件接管（切换 Tab 而非真正关闭整个面板）。
export default function WorkspacePanel({
  sessionId,
  cwd,
  messages,
  changesCount,
  onClose,
}: WorkspacePanelProps) {
  const [activeTab, setActiveTab] = useState<TabKey>('files')

  // 改动数 > 0 时，若用户尚未手动选择过，自动切到 changes 提示（仅一次）
  const [autoSwitched, setAutoSwitched] = useState(false)
  useEffect(() => {
    if (changesCount > 0 && !autoSwitched) {
      setAutoSwitched(true)
    }
  }, [changesCount, autoSwitched])

  const tabs: { key: TabKey; label: string; icon: string; badge?: number }[] = [
    { key: 'files', label: '文件', icon: '📁' },
    { key: 'terminal', label: '终端', icon: '⌨' },
    { key: 'changes', label: '改动', icon: '✎', badge: changesCount },
  ]

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
            ▶
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
              messages={messages}
              sessionId={sessionId}
              cwd={cwd}
              onClose={() => setActiveTab('files')}
            />
          ) : (
            <div style={{ padding: 24, color: 'var(--text-muted)', textAlign: 'center' }}>
              无工作目录信息
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
