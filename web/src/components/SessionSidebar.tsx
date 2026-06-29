import { useState, useEffect } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import type { Session, ScheduledTask, AgentStatus } from '../types'
import { listScheduledTasks } from '../api/scheduledTasks'
import { listAgentStatus } from '../api/agents'
import styles from './SessionSidebar.module.css'

interface SessionSidebarProps {
  sessions: Session[]
  currentId?: number
  onDelete?: (id: number) => void
  onRename?: (id: number, title: string) => void
  /** 折叠侧边栏回调 */
  onCollapse?: () => void
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

export default function SessionSidebar({ sessions, currentId, onDelete, onRename, onCollapse }: SessionSidebarProps) {
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const location = useLocation()
  const navigate = useNavigate()
  const isHome = location.pathname === '/' || location.pathname.startsWith('/sessions')

  const [collapsed, setCollapsed] = useState(loadCollapsed)
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [agentStatuses, setAgentStatuses] = useState<AgentStatus[]>([])

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

  // 加载 agent 连接状态（轮询，每 3s 刷新一次，近实时显示）
  useEffect(() => {
    let alive = true
    const load = () => {
      listAgentStatus()
        .then((r) => {
          if (alive) setAgentStatuses(r.data.agents || [])
        })
        .catch(() => {
          if (alive) setAgentStatuses([])
        })
    }
    load()
    const timer = setInterval(load, 3000)
    return () => {
      alive = false
      clearInterval(timer)
    }
  }, [])

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
      {/* 顶部 Logo：点击回首页 + 折叠按钮 */}
      <Link to="/" className={styles.logo} title="返回首页">
        <span className={styles.logoIcon}>⚡</span>
        <span className={styles.logoText}>NexusAgent</span>
        {onCollapse && (
          <button
            type="button"
            className={styles.collapseBtn}
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              onCollapse()
            }}
            title="折叠侧边栏"
          >
            ◀
          </button>
        )}
      </Link>

      {/* Agent 连接状态 */}
      {agentStatuses.length > 0 && (
        <div className={styles.agentStatus}>
          {agentStatuses.map((s) => {
            const statusLabel = s.status === 'connected' ? '已连接' : s.status === 'connecting' ? '连接中' : '未连接'
            const dotClass = s.status === 'connected' ? styles.agentDotOn : s.status === 'connecting' ? styles.agentDotConnecting : styles.agentDotOff
            return (
              <div key={s.agent_type} className={styles.agentStatusItem} title={`${s.agent_type}：${statusLabel}，活跃会话 ${s.active_count}`}>
                <span className={`${styles.agentDot} ${dotClass}`} />
                <span className={styles.agentName}>{s.agent_type}</span>
                {s.status === 'connecting' && <span className={styles.agentStatusText}>{statusLabel}</span>}
                <span className={styles.agentCount}>{s.active_count}</span>
              </div>
            )
          })}
        </div>
      )}

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
            <span className={styles.groupTitle}>📝 我的任务</span>
            <span className={styles.groupCount}>{manualSessions.length}</span>
            <span
              className={styles.addBtn}
              role="button"
              tabIndex={0}
              title="新建会话"
              onClick={(e) => {
                e.stopPropagation()
                navigate('/')
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.stopPropagation()
                  navigate('/')
                }
              }}
            >
              +
            </span>
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
                    {editingId === session.id ? (
                      <div className={styles.editRow}>
                        <input
                          className={styles.editInput}
                          value={editTitle}
                          onChange={(e) => setEditTitle(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                              const t = editTitle.trim()
                              if (t && onRename) onRename(session.id, t)
                              setEditingId(null)
                            } else if (e.key === 'Escape') {
                              setEditingId(null)
                            }
                          }}
                          autoFocus
                        />
                        <button
                          type="button"
                          className={styles.editOkBtn}
                          onClick={() => {
                            const t = editTitle.trim()
                            if (t && onRename) onRename(session.id, t)
                            setEditingId(null)
                          }}
                        >
                          ✓
                        </button>
                      </div>
                    ) : (
                      <>
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
                        <div className={styles.itemActions}>
                          {onRename && (
                            <button
                              type="button"
                              className={styles.renameBtn}
                              title="重命名"
                              aria-label="重命名"
                              onClick={(e) => {
                                e.preventDefault()
                                e.stopPropagation()
                                setEditTitle(session.title || session.agent_type)
                                setEditingId(session.id)
                              }}
                            >
                              ✎
                            </button>
                          )}
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
                      </>
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
          <span className={styles.navIcon}>📋</span>
          <span>会话列表</span>
        </Link>
        <Link
          to="/scheduled-tasks"
          className={`${styles.navItem} ${location.pathname === '/scheduled-tasks' ? styles.navItemActive : ''}`}
        >
          <span className={styles.navIcon}>⏰</span>
          <span>定时任务配置</span>
        </Link>
        <Link
          to="/settings"
          className={`${styles.navItem} ${location.pathname === '/settings' ? styles.navItemActive : ''}`}
        >
          <span className={styles.navIcon}>🤖</span>
          <span>Agent 设置</span>
        </Link>
      </div>
    </div>
  )
}
