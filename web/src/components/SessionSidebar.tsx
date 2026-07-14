import { useState, useEffect, useMemo } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { formatTimeAgo } from '../utils/time'
import { sessionUrl, tasksUrl, newTaskUrl, isTasksPath } from '../utils/routes'
import type { Session, ScheduledTask, AgentStatus } from '../types'
import { listScheduledTasks } from '../api/scheduledTasks'
import { listAgentStatus } from '../api/agents'
import { listSessions, listRunningSessions } from '../api/sessions'
import { PanelLeftClose, Star, Pencil, X, Check, SquarePlus, FileText, Calendar, ClipboardList, Timer, Settings, Zap, ScrollText, Loader2, CheckCircle2, XCircle, Clock3, CircleDashed } from 'lucide-react'
import styles from './SessionSidebar.module.css'
import LogPanel from './LogPanel'

interface SessionSidebarProps {
  sessions: Session[]
  workspaceId?: number
  currentId?: number
  onDelete?: (id: number) => void
  onRename?: (id: number, title: string) => void
  onCollapse?: () => void
  onNewScheduledTask?: () => void
}

const STORAGE_KEY = 'nexus.sidebar.collapsed'
const FAVS_KEY = 'nexus.favorites'

function loadCollapsed(): { favorites: boolean; manual: boolean; scheduled: boolean } {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore */ }
  return { favorites: false, manual: false, scheduled: false }
}

function loadFavorites(): number[] {
  try {
    const raw = localStorage.getItem(FAVS_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore */ }
  return []
}

function saveFavorites(ids: number[]) {
  try { localStorage.setItem(FAVS_KEY, JSON.stringify(ids)) } catch { /* ignore */ }
}

function TaskStatusIcon({ status }: { status: string }) {
  const size = 13
  const cls = styles.taskStatusIcon
  if (status === 'running') return <Loader2 size={size} className={`${cls} ${styles.taskStatusIconSpin}`} />
  if (status === 'success') return <CheckCircle2 size={size} className={`${cls} ${styles.taskStatusIconSuccess}`} />
  if (status === 'failed') return <XCircle size={size} className={`${cls} ${styles.taskStatusIconFailed}`} />
  if (status === 'skipped') return <Clock3 size={size} className={`${cls} ${styles.taskStatusIconSkipped}`} />
  return <CircleDashed size={size} className={`${cls} ${styles.taskStatusIconIdle}`} />
}

// SessionStatusIcon 普通会话的实时运行状态图标：
// 运行中(agent 正在生成)→ 蓝色旋转；error → 红色叉；否则 → 灰色对勾(完成/空闲)。
function SessionStatusIcon({ running, status }: { running: boolean; status: string }) {
  const size = 13
  const cls = styles.taskStatusIcon
  if (running) return <Loader2 size={size} className={`${cls} ${styles.taskStatusIconSpin}`} />
  if (status === 'error') return <XCircle size={size} className={`${cls} ${styles.taskStatusIconFailed}`} />
  return <CheckCircle2 size={size} className={`${cls} ${styles.taskStatusIconIdle}`} />
}

export default function SessionSidebar({ sessions, workspaceId, currentId, onDelete, onRename, onCollapse, onNewScheduledTask }: SessionSidebarProps) {
  const { t } = useTranslation()
  const [editingId, setEditingId] = useState<number | null>(null)
  const [showLogs, setShowLogs] = useState(false)
  const [editTitle, setEditTitle] = useState('')
  const location = useLocation()
  const navigate = useNavigate()
  const isSessionList = isTasksPath(location.pathname, workspaceId)

  const [collapsed, setCollapsed] = useState(loadCollapsed)
  const [favorites, setFavorites] = useState<number[]>(loadFavorites)
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [agentStatuses, setAgentStatuses] = useState<AgentStatus[]>([])
  const [runningIds, setRunningIds] = useState<Set<number>>(() => new Set())

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(collapsed)) } catch { /* ignore */ }
  }, [collapsed])

  useEffect(() => {
    let alive = true
    listScheduledTasks(workspaceId || undefined)
      .then((r) => { if (alive) setTasks(r.data.tasks || []) })
      .catch(() => { if (alive) setTasks([]) })
    return () => { alive = false }
  }, [location.pathname, workspaceId])

  useEffect(() => {
    let alive = true
    const load = () => {
      listAgentStatus()
        .then((r) => { if (alive) setAgentStatuses(r.data.agents || []) })
        .catch(() => { if (alive) setAgentStatuses([]) })
      listRunningSessions()
        .then((r) => { if (alive) setRunningIds(new Set(r.data.db_session_ids || [])) })
        .catch(() => {})
    }
    load()
    const timer = setInterval(load, 3000)
    return () => { alive = false; clearInterval(timer) }
  }, [])

  useEffect(() => {
    let alive = true
    listSessions()
      .then((r) => {
        if (!alive) return
        const ids = new Set((r.data.sessions || []).map((s) => s.id))
        setFavorites((prev) => {
          const next = prev.filter((id) => ids.has(id))
          if (next.length !== prev.length) saveFavorites(next)
          return next.length !== prev.length ? next : prev
        })
      })
      .catch(() => {})
    return () => { alive = false }
  }, [])

  const manualSessions = sessions.filter((s) => !s.source || s.source === 'manual')
  const favoriteSessions = useMemo(
    () => sessions.filter((s) => favorites.includes(s.id)),
    [sessions, favorites],
  )
  const recentTask = [...tasks]
    .filter((t) => t.last_run_at)
    .sort((a, b) => (a.last_run_at! < b.last_run_at! ? 1 : -1))[0]

  function toggleGroup(group: 'favorites' | 'manual' | 'scheduled') {
    setCollapsed((prev) => ({ ...prev, [group]: !prev[group] }))
  }

  function toggleFavorite(id: number, e: React.MouseEvent) {
    e.preventDefault(); e.stopPropagation()
    setFavorites((prev) => {
      const next = prev.includes(id) ? prev.filter((fid) => fid !== id) : [...prev, id]
      saveFavorites(next)
      return next
    })
  }

  function handleNewScheduledTask(e: React.MouseEvent | React.KeyboardEvent) {
    e.stopPropagation()
    if (onNewScheduledTask) {
      onNewScheduledTask()
      return
    }
    navigate('/scheduled-tasks', { state: { openCreate: true } })
  }

  return (
    <div className={styles.sidebar}>
      <Link to={tasksUrl(workspaceId)} className={styles.logo} title={t('nav.sessionList')}>
        <span className={styles.logoText}>NexusAgent</span>
        {onCollapse && (
          <button
            type="button"
            className={styles.collapseBtn}
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); onCollapse() }}
            title={t('common.close') + ' (⌘B)'}
          >
            <PanelLeftClose size={16} />
          </button>
        )}
      </Link>

      <div className={styles.groups}>
        {/* 收藏任务 */}
        <div className={styles.group}>
          <button type="button" className={styles.groupHeader} onClick={() => toggleGroup('favorites')}>
            <span className={styles.groupTitle}><Star size={13} style={{ marginRight: 4, verticalAlign: '-2px' }} />{t('session.favGroup')}</span>

          </button>
          {!collapsed.favorites && (
            <div className={styles.groupList}>
              {favoriteSessions.length === 0 ? (
                <p className={styles.empty}>{t('session.noFavorites')}</p>
              ) : (
                favoriteSessions.map((session) => (
                  <div key={session.id} className={`${styles.item} ${currentId === session.id ? styles.itemActive : ''}`}>
                    <Link to={sessionUrl(session.id, session.workspace_id)} className={styles.itemLink}>
                      <div className={styles.itemRow}>
                        <span className={styles.itemTitle}>
                          <SessionStatusIcon running={runningIds.has(session.id)} status={session.status} />
                          {session.title || session.agent_type}
                        </span>
                        <span className={styles.itemTime}>{formatTimeAgo(session.created_at, t)}</span>
                      </div>
                    </Link>
                    <div className={styles.itemActions}>
                      <button type="button" className={styles.favBtnActive}
                        title={t('session.favorited')}
                        onClick={(e) => toggleFavorite(session.id, e)}
                      ><Star size={13} fill="currentColor" strokeWidth={0} /></button>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        <div className={styles.group}>
          <button type="button" className={styles.groupHeader} onClick={() => toggleGroup('manual')}>
            <span className={styles.groupTitle}><FileText size={13} style={{ marginRight: 4, verticalAlign: '-2px' }} />{t('session.title')}</span>

            <span
              className={styles.addBtn} role="button" tabIndex={0}
              title={t('session.newSession')}
              onClick={(e) => { e.stopPropagation(); navigate(newTaskUrl(workspaceId)) }}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.stopPropagation(); navigate(newTaskUrl(workspaceId)) } }}
            ><SquarePlus size={14} /></span>
          </button>
          {!collapsed.manual && (
            <div className={styles.groupList}>
              {manualSessions.length === 0 ? (
                <p className={styles.empty}>{t('session.noSessions')}</p>
              ) : (
                manualSessions.map((session) => (
                  <div key={session.id} className={`${styles.item} ${currentId === session.id ? styles.itemActive : ''}`}>
                    {editingId === session.id ? (
                      <div className={styles.editRow}>
                        <input className={styles.editInput} value={editTitle}
                          onChange={(e) => setEditTitle(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') { const t = editTitle.trim(); if (t && onRename) onRename(session.id, t); setEditingId(null) }
                            else if (e.key === 'Escape') setEditingId(null)
                          }} autoFocus />
                        <button type="button" className={styles.editOkBtn}
                          onClick={() => { const t = editTitle.trim(); if (t && onRename) onRename(session.id, t); setEditingId(null) }}
                        ><Check size={13} /></button>
                      </div>
                    ) : (
                      <>
                        <Link to={sessionUrl(session.id, session.workspace_id)} className={styles.itemLink}>
                          <div className={styles.itemRow}>
                            <span className={styles.itemTitle}>
                              <SessionStatusIcon running={runningIds.has(session.id)} status={session.status} />
                              {session.title || session.agent_type}
                            </span>
                            <span className={styles.itemTime}>{formatTimeAgo(session.created_at, t)}</span>
                          </div>
                        </Link>
                        <div className={styles.itemActions}>
                          <button type="button" className={favorites.includes(session.id) ? styles.favBtnActive : styles.favBtn}
                            title={favorites.includes(session.id) ? t('session.favorited') : t('session.unfavorited')}
                            onClick={(e) => toggleFavorite(session.id, e)}
                          >{favorites.includes(session.id) ? <Star size={13} fill="currentColor" strokeWidth={0} /> : <Star size={13} />}</button>
                          {onRename && (
                            <button type="button" className={styles.renameBtn}
                              title={t('common.rename')} aria-label={t('common.rename')}
                              onClick={(e) => { e.preventDefault(); e.stopPropagation(); setEditTitle(session.title || session.agent_type); setEditingId(session.id) }}
                            ><Pencil size={13} /></button>
                          )}
                          {onDelete && (
                            <button type="button" className={styles.deleteBtn}
                              title={t('common.delete')} aria-label={t('common.delete')}
                              onClick={(e) => {
                                e.preventDefault(); e.stopPropagation()
                                if (window.confirm(t('session.deleteConfirm'))) {
                                  setFavorites((prev) => {
                                    const next = prev.filter((fid) => fid !== session.id)
                                    saveFavorites(next)
                                    return next
                                  })
                                  onDelete(session.id)
                                }
                              }}
                            ><X size={13} /></button>
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

        <div className={styles.group}>
          <button type="button" className={styles.groupHeader} onClick={() => toggleGroup('scheduled')}>
            <span className={styles.groupTitle}><Calendar size={13} style={{ marginRight: 4, verticalAlign: '-2px' }} />{t('nav.scheduledTasks')}</span>

            <span
              className={styles.addBtn} role="button" tabIndex={0}
              title={t('scheduledTask.newTask')}
              onClick={handleNewScheduledTask}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') handleNewScheduledTask(e) }}
            ><SquarePlus size={14} /></span>
          </button>
          {!collapsed.scheduled && (
            <div className={styles.groupList}>
              {recentTask && recentTask.db_session_id ? (
                <button type="button" className={styles.recentEntry}
                  onClick={() => navigate(sessionUrl(recentTask.db_session_id, recentTask.workspace_id))}
                  title={`${t('nav.recentRun')}: ${recentTask.name}`}
                >
                  <span className={styles.recentIcon}><Zap size={13} /></span>
                  <span className={styles.recentText}>{t('nav.recentRun')} · {recentTask.name}</span>
                </button>
              ) : null}
              {tasks.length === 0 ? (
                <p className={styles.empty}>{t('scheduledTask.noTasks')}</p>
              ) : (
                tasks.map((task) => (
                  <div key={task.id} className={`${styles.item} ${currentId === task.db_session_id ? styles.itemActive : ''}`}>
                    <button type="button" className={styles.itemLink}
                      onClick={() => task.db_session_id ? navigate(sessionUrl(task.db_session_id, task.workspace_id)) : undefined}
                      disabled={!task.db_session_id}
                      style={!task.db_session_id ? { cursor: 'default', opacity: 0.6 } : undefined}
                    >
                      <div className={styles.itemRow}>
                        <span className={styles.itemTitle}>
                          <TaskStatusIcon status={task.last_status} />
                          {task.name}
                        </span>
                        {task.last_run_at && (
                          <span className={styles.itemTime}>{formatTimeAgo(task.last_run_at, t)}</span>
                        )}
                      </div>
                    </button>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      </div>

      {agentStatuses.length > 0 && (
        <div className={styles.agentStatus}>
          {agentStatuses.map((s) => {
            const statusLabel = s.status === 'connected' ? t('status.connected') : s.status === 'connecting' ? t('status.connecting') : t('status.disconnected')
            const dotClass = s.status === 'connected' ? styles.agentDotOn : s.status === 'connecting' ? styles.agentDotConnecting : styles.agentDotOff
            const statusClass = s.status === 'connected' ? styles.agentStatusConnected : s.status === 'connecting' ? styles.agentStatusConnecting : styles.agentStatusDisconnected
            return (
              <div key={s.agent_type} className={styles.agentStatusItem}>
                <span className={`${styles.agentDot} ${dotClass}`} />
                <span className={styles.agentName}>{s.agent_type}</span>
                <span className={`${styles.agentStatusText} ${statusClass}`}>{statusLabel}</span>
                <span className={styles.agentCount}>{s.active_count}</span>
              </div>
            )
          })}
        </div>
      )}

      <div className={styles.footer}>
        <Link to={tasksUrl(workspaceId)} className={`${styles.navItem} ${isSessionList ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}><ClipboardList size={15} /></span>
          <span>{t('nav.sessionList')}</span>
        </Link>
        <Link to="/scheduled-tasks" className={`${styles.navItem} ${location.pathname === '/scheduled-tasks' ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}><Timer size={15} /></span>
          <span>{t('nav.scheduledTasks')}</span>
        </Link>
        <Link to="/notes" className={`${styles.navItem} ${location.pathname === '/notes' ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}><FileText size={15} /></span>
          <span>{t('nav.notes')}</span>
        </Link>
        <button type="button" className={`${styles.navItem} ${showLogs ? styles.navItemActive : ''}`}
          onClick={() => setShowLogs((v) => !v)}
        >
          <span className={styles.navIcon}><ScrollText size={15} /></span>
          <span>{t('log.openLogs')}</span>
        </button>
        <Link to="/settings" className={`${styles.navItem} ${location.pathname === '/settings' ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}><Settings size={15} /></span>
          <span>{t('common.settings')}</span>
        </Link>
      </div>

      {showLogs && <LogPanel onClose={() => setShowLogs(false)} />}
    </div>
  )
}
