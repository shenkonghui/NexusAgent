import { Link } from 'react-router-dom'
import type { Session } from '../types'
import styles from './SessionSidebar.module.css'

interface SessionSidebarProps {
  sessions: Session[]
  currentId?: number
}

// 状态标识颜色
const statusColors: Record<string, string> = {
  active: styles.statusActive,
  closed: styles.statusClosed,
  error: styles.statusError,
}

const statusLabels: Record<string, string> = {
  active: '活跃',
  closed: '已关闭',
  error: '错误',
}

export default function SessionSidebar({ sessions, currentId }: SessionSidebarProps) {
  return (
    <div className={styles.sidebar}>
      <div className={styles.header}>
        <h2 className={styles.title}>会话列表</h2>
        <Link to="/sessions" className={styles.newBtn}>
          + 新建
        </Link>
      </div>
      <div className={styles.list}>
        {sessions.length === 0 ? (
          <p className={styles.empty}>暂无会话</p>
        ) : (
          sessions.map((session) => (
            <Link
              key={session.id}
              to={`/sessions/${session.id}`}
              className={`${styles.item} ${currentId === session.id ? styles.itemActive : ''}`}
            >
              <div className={styles.itemHeader}>
                <span className={styles.agentType}>{session.agent_type}</span>
                <span className={`${styles.status} ${statusColors[session.status] || ''}`}>
                  {statusLabels[session.status] || session.status}
                </span>
              </div>
              {session.last_prompt && (
                <p className={styles.lastPrompt}>{session.last_prompt}</p>
              )}
              <span className={styles.time}>
                {new Date(session.created_at).toLocaleString('zh-CN')}
              </span>
            </Link>
          ))
        )}
      </div>
    </div>
  )
}
