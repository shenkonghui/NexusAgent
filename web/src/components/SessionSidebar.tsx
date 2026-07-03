import { useState, useEffect, useRef, useMemo } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import i18n from '../i18n'
import { formatTimeAgo } from '../utils/time'
import type { Session, ScheduledTask, AgentStatus } from '../types'
import { listScheduledTasks } from '../api/scheduledTasks'
import { listAgentStatus } from '../api/agents'
import { listSessions } from '../api/sessions'
import styles from './SessionSidebar.module.css'

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

// 直接构造会话最终 URL，避免经过 SessionRedirect 的中间跳转
function sessionUrl(sessionId: number, workspaceId?: number | null): string {
  if (workspaceId) return `/workspaces/${workspaceId}/sessions/${sessionId}`
  return `/sessions/${sessionId}`
}

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

export default function SessionSidebar({ sessions, workspaceId, currentId, onDelete, onRename, onCollapse, onNewScheduledTask }: SessionSidebarProps) {
  const { t } = useTranslation()
  const [editingId, setEditingId] = useState<number | null>(null)
  const [showLangMenu, setShowLangMenu] = useState(false)
  const langRef = useRef<HTMLDivElement>(null)
  const [editTitle, setEditTitle] = useState('')
  const location = useLocation()
  const navigate = useNavigate()
  const isSessionList = location.pathname === '/'

  const [collapsed, setCollapsed] = useState(loadCollapsed)
  const [favorites, setFavorites] = useState<number[]>(loadFavorites)
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [agentStatuses, setAgentStatuses] = useState<AgentStatus[]>([])

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(collapsed)) } catch { /* ignore */ }
  }, [collapsed])

  useEffect(() => {
    if (!showLangMenu) return
    function handleClick(e: MouseEvent) {
      if (langRef.current && !langRef.current.contains(e.target as Node)) setShowLangMenu(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [showLangMenu])

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
  const classifySession = sessions.find((s) => s.source === 'classify')
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
      <Link to="/" className={styles.logo} title={t('nav.sessionList')}>
        <span className={styles.logoIcon}>⚡</span>
        <span className={styles.logoText}>NexusAgent</span>
        {onCollapse && (
          <button
            type="button"
            className={styles.collapseBtn}
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); onCollapse() }}
            title={t('common.close')}
          >
            ◀
          </button>
        )}
      </Link>

      <div className={styles.groups}>
        {/* 收藏任务 */}
        <div className={styles.group}>
          <button type="button" className={styles.groupHeader} onClick={() => toggleGroup('favorites')}>
            <span className={styles.groupTitle}>⭐ {t('session.favGroup')}</span>
            <span className={styles.groupCount}>{favoriteSessions.length}</span>
            <span className={styles.groupArrow}>{collapsed.favorites ? '▶' : '▼'}</span>
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
                        <span className={styles.itemTitle}>{session.title || session.agent_type}</span>
                        <span className={styles.itemTime}>{formatTimeAgo(session.created_at, t)}</span>
                      </div>
                    </Link>
                    <div className={styles.itemActions}>
                      <button type="button" className={styles.favBtnActive}
                        title={t('session.favorited')}
                        onClick={(e) => toggleFavorite(session.id, e)}
                      >★</button>
                    </div>
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        <div className={styles.group}>
          <button type="button" className={styles.groupHeader} onClick={() => toggleGroup('manual')}>
            <span className={styles.groupTitle}>📝 {t('session.title')}</span>
            <span className={styles.groupCount}>{manualSessions.length}</span>
            <span
              className={styles.addBtn} role="button" tabIndex={0}
              title={t('session.newSession')}
              onClick={(e) => { e.stopPropagation(); navigate('/new') }}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.stopPropagation(); navigate('/new') } }}
            >+</span>
            <span className={styles.groupArrow}>{collapsed.manual ? '▶' : '▼'}</span>
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
                        >✓</button>
                      </div>
                    ) : (
                      <>
                        <Link to={sessionUrl(session.id, session.workspace_id)} className={styles.itemLink}>
                          <div className={styles.itemRow}>
                            <span className={styles.itemTitle}>{session.title || session.agent_type}</span>
                            <span className={styles.itemTime}>{formatTimeAgo(session.created_at, t)}</span>
                          </div>
                        </Link>
                        <div className={styles.itemActions}>
                          <button type="button" className={favorites.includes(session.id) ? styles.favBtnActive : styles.favBtn}
                            title={favorites.includes(session.id) ? t('session.favorited') : t('session.unfavorited')}
                            onClick={(e) => toggleFavorite(session.id, e)}
                          >{favorites.includes(session.id) ? '★' : '☆'}</button>
                          {onRename && (
                            <button type="button" className={styles.renameBtn}
                              title={t('common.rename')} aria-label={t('common.rename')}
                              onClick={(e) => { e.preventDefault(); e.stopPropagation(); setEditTitle(session.title || session.agent_type); setEditingId(session.id) }}
                            >✎</button>
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
                            >×</button>
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
            <span className={styles.groupTitle}>📅 {t('nav.scheduledTasks')}</span>
            <span className={styles.groupCount}>{tasks.length}</span>
            <span
              className={styles.addBtn} role="button" tabIndex={0}
              title={t('scheduledTask.newTask')}
              onClick={handleNewScheduledTask}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') handleNewScheduledTask(e) }}
            >+</span>
            <span className={styles.groupArrow}>{collapsed.scheduled ? '▶' : '▼'}</span>
          </button>
          {!collapsed.scheduled && (
            <div className={styles.groupList}>
              {recentTask && recentTask.db_session_id ? (
                <button type="button" className={styles.recentEntry}
                  onClick={() => navigate(sessionUrl(recentTask.db_session_id, recentTask.workspace_id))}
                  title={`${t('nav.recentRun')}: ${recentTask.name}`}
                >
                  <span className={styles.recentIcon}>⚡</span>
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
                        <span className={styles.itemTitle}>{task.name}</span>
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
        <Link to="/" className={`${styles.navItem} ${isSessionList ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}>📋</span>
          <span>{t('nav.sessionList')}</span>
        </Link>
        <Link to="/scheduled-tasks" className={`${styles.navItem} ${location.pathname === '/scheduled-tasks' ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}>⏰</span>
          <span>{t('nav.scheduledTasks')}</span>
        </Link>
        <Link to="/notes" className={`${styles.navItem} ${location.pathname === '/notes' ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}>📝</span>
          <span>{t('nav.notes')}</span>
        </Link>
        {classifySession && (
          <Link
            to={sessionUrl(classifySession.id, classifySession.workspace_id)}
            className={`${styles.navItem} ${location.pathname === sessionUrl(classifySession.id, classifySession.workspace_id) ? styles.navItemActive : ''}`}
          >
            <span className={styles.navIcon}>🏷️</span>
            <span>{t('notes.classifyTask')}</span>
          </Link>
        )}
        <div className={styles.langRow} ref={langRef}>
          <button type="button" className={`${styles.navItem} ${styles.langBtn}`}
            onClick={() => setShowLangMenu((v) => !v)}
          >
            <span className={styles.navIcon}>🌐</span>
            <span>{i18n.language === 'zh' ? '中文' : 'English'}</span>
          </button>
          {showLangMenu && (
            <div className={styles.langMenu}>
              <button type="button" className={`${styles.langMenuItem} ${i18n.language === 'zh' ? styles.langMenuItemActive : ''}`}
                onClick={() => { i18n.changeLanguage('zh'); localStorage.setItem('nexus-lang', 'zh'); window.location.reload() }}
              >中文</button>
              <button type="button" className={`${styles.langMenuItem} ${i18n.language === 'en' ? styles.langMenuItemActive : ''}`}
                onClick={() => { i18n.changeLanguage('en'); localStorage.setItem('nexus-lang', 'en'); window.location.reload() }}
              >English</button>
            </div>
          )}
        </div>
        <Link to="/settings" className={`${styles.navItem} ${location.pathname === '/settings' ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}>⚙️</span>
          <span>{t('common.settings')}</span>
        </Link>
      </div>
    </div>
  )
}
