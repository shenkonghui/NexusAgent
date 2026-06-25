import { useState, useEffect } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import type { Session, ScheduledTask } from '../types'
import { listScheduledTasks } from '../api/scheduledTasks'
import styles from './SessionSidebar.module.css'

interface SessionSidebarProps {
  sessions: Session[]
  currentId?: number
  onDelete?: (id: number) => void
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

const STORAGE_KEY = 'nexus.sidebar.collapsed'

function loadCollapsed(): { manual: boolean; scheduled: boolean } {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw)
  } catch {
    /* ignore */
  }
  return { manual: false, scheduled: false }
}

export default function SessionSidebar({ sessions, currentId, onDelete }: SessionSidebarProps) {
  const location = useLocation()
  const navigate = useNavigate()
  const isHome = location.pathname === '/' || location.pathname.startsWith('/sessions')

  const [collapsed, setCollapsed] = useState(loadCollapsed)
  const [tasks, setTasks] = useState<ScheduledTask[]>([])

  // 折叠状态持久化
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(collapsed))
    } catch {
      /* ignore */
    }
  }, [collapsed])

  // 加载定时任务列表（用于定时任务分组展示）
  useEffect(() => {
    let alive = true
    listScheduledTasks()
      .then((r) => {
        if (alive) setTasks(r.data.tasks || [])
      })
      .catch(() => {
        if (alive) setTasks([])
      })
    return () => {
      alive = false
    }
  }, [location.pathname])

  // 按来源拆分会话（定时任务分组由 tasks 驱动展示，此处仅取手动会话）
  const manualSessions = sessions.filter((s) => !s.source || s.source === 'manual')

  // 最近执行的定时任务（按 last_run_at 降序，无则取首个）
  const recentTask = [...tasks]
    .filter((t) => t.last_run_at)
    .sort((a, b) => (a.last_run_at! < b.last_run_at! ? 1 : -1))[0]

  function toggleGroup(group: 'manual' | 'scheduled') {
    setCollapsed((prev) => ({ ...prev, [group]: !prev[group] }))
  }

  return (
    <div className={styles.sidebar}>
      {/* 顶部 Logo：点击回首页 */}
      <Link to="/" className={styles.logo} title="返回首页">
        <span className={styles.logoIcon}>⚡</span>
        <span className={styles.logoText}>NexusAgent</span>
      </Link>

      {/* 顶部：双折叠分组 */}
      <div className={styles.groups}>
        {/* 手动会话分组 */}
        <div className={styles.group}>
          <button
            type="button"
            className={styles.groupHeader}
            onClick={() => toggleGroup('manual')}
          >
            <span className={styles.groupArrow}>{collapsed.manual ? '▶' : '▼'}</span>
            <span className={styles.groupTitle}>📝 手动会话</span>
            <span className={styles.groupCount}>{manualSessions.length}</span>
          </button>
          {!collapsed.manual && (
            <div className={styles.groupList}>
              {manualSessions.length === 0 ? (
                <p className={styles.empty}>暂无会话</p>
              ) : (
                manualSessions.map((session) => (
                  <div
                    key={session.id}
                    className={`${styles.item} ${currentId === session.id ? styles.itemActive : ''}`}
                  >
                    <Link to={`/sessions/${session.id}`} className={styles.itemLink}>
                      <div className={styles.itemHeader}>
                        <span className={styles.agentType}>{session.title || session.agent_type}</span>
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
                    {onDelete && (
                      <button
                        type="button"
                        className={styles.deleteBtn}
                        title="删除会话"
                        aria-label="删除会话"
                        onClick={(e) => {
                          e.preventDefault()
                          e.stopPropagation()
                          if (window.confirm(`确定删除会话「${session.title || session.agent_type}」吗？此操作不可恢复。`)) {
                            onDelete(session.id)
                          }
                        }}
                      >
                        ×
                      </button>
                    )}
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        {/* 定时任务分组 */}
        <div className={styles.group}>
          <button
            type="button"
            className={styles.groupHeader}
            onClick={() => toggleGroup('scheduled')}
          >
            <span className={styles.groupArrow}>{collapsed.scheduled ? '▶' : '▼'}</span>
            <span className={styles.groupTitle}>⏰ 定时任务</span>
            <span className={styles.groupCount}>{tasks.length}</span>
          </button>
          {!collapsed.scheduled && (
            <div className={styles.groupList}>
              {/* 最近执行快捷入口 */}
              {recentTask && recentTask.db_session_id ? (
                <button
                  type="button"
                  className={styles.recentEntry}
                  onClick={() => navigate(`/sessions/${recentTask.db_session_id}`)}
                  title={`最近执行：${recentTask.name}`}
                >
                  <span className={styles.recentIcon}>⚡</span>
                  <span className={styles.recentText}>最近执行 · {recentTask.name}</span>
                </button>
              ) : null}

              {tasks.length === 0 ? (
                <p className={styles.empty}>暂无定时任务</p>
              ) : (
                tasks.map((task) => (
                  <div
                    key={task.id}
                    className={`${styles.item} ${currentId === task.db_session_id ? styles.itemActive : ''}`}
                  >
                    <button
                      type="button"
                      className={styles.itemLink}
                      onClick={() =>
                        task.db_session_id
                          ? navigate(`/sessions/${task.db_session_id}`)
                          : undefined
                      }
                      disabled={!task.db_session_id}
                      style={!task.db_session_id ? { cursor: 'default', opacity: 0.6 } : undefined}
                    >
                      <span className={styles.agentType}>{task.name}</span>
                    </button>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      </div>

      {/* 底部：次级导航 */}
      <div className={styles.footer}>
        <Link to="/" className={`${styles.navItem} ${isHome ? styles.navItemActive : ''}`}>
          📋 会话列表
        </Link>
        <Link
          to="/scheduled-tasks"
          className={`${styles.navItem} ${location.pathname === '/scheduled-tasks' ? styles.navItemActive : ''}`}
        >
          ⏰ 定时任务配置
        </Link>
        <Link
          to="/settings"
          className={`${styles.navItem} ${location.pathname === '/settings' ? styles.navItemActive : ''}`}
        >
          ⚙ 设置
        </Link>
      </div>
    </div>
  )
}
