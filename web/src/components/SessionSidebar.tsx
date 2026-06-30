import { useState, useEffect, useRef } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import i18n from '../i18n'
import type { Session, ScheduledTask, AgentStatus } from '../types'
import { listScheduledTasks } from '../api/scheduledTasks'
import { listAgentStatus } from '../api/agents'
import styles from './SessionSidebar.module.css'

interface SessionSidebarProps {
  sessions: Session[]
  currentId?: number
  onDelete?: (id: number) => void
  onRename?: (id: number, title: string) => void
  onCollapse?: () => void
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

export default function SessionSidebar({ sessions, currentId, onDelete, onRename, onCollapse }: SessionSidebarProps) {
  const { t } = useTranslation()
  const [editingId, setEditingId] = useState<number | null>(null)
  const [showLangMenu, setShowLangMenu] = useState(false)
  const langRef = useRef<HTMLDivElement>(null)
  const [editTitle, setEditTitle] = useState('')
  const location = useLocation()
  const navigate = useNavigate()
  const isHome = location.pathname === '/' || location.pathname.startsWith('/sessions')

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
    listScheduledTasks()
      .then((r) => { if (alive) setTasks(r.data.tasks || []) })
      .catch(() => { if (alive) setTasks([]) })
    return () => { alive = false }
  }, [location.pathname])

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

  const manualSessions = sessions.filter((s) => !s.source || s.source === 'manual')
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

  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'

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

      {agentStatuses.length > 0 && (
        <div className={styles.agentStatus}>
          {agentStatuses.map((s) => {
            const statusLabel = s.status === 'connected' ? t('status.connected') : s.status === 'connecting' ? t('status.connecting') : t('status.disconnected')
            const dotClass = s.status === 'connected' ? styles.agentDotOn : s.status === 'connecting' ? styles.agentDotConnecting : styles.agentDotOff
            return (
              <div key={s.agent_type} className={styles.agentStatusItem} title={`${s.agent_type}: ${statusLabel}`}>
                <span className={`${styles.agentDot} ${dotClass}`} />
                <span className={styles.agentName}>{s.agent_type}</span>
                {s.status === 'connecting' && <span className={styles.agentStatusText}>{statusLabel}</span>}
                <span className={styles.agentCount}>{s.active_count}</span>
              </div>
            )
          })}
        </div>
      )}

      <div className={styles.groups}>
        {/* 收藏任务 */}
        <div className={styles.group}>
          <button type="button" className={styles.groupHeader} onClick={() => toggleGroup('favorites')}>
            <span className={styles.groupArrow}>{collapsed.favorites ? '▶' : '▼'}</span>
            <span className={styles.groupTitle}>⭐ {t('session.favGroup')}</span>
            <span className={styles.groupCount}>{favorites.length}</span>
          </button>
          {!collapsed.favorites && (
            <div className={styles.groupList}>
              {favorites.length === 0 ? (
                <p className={styles.empty}>{t('session.noFavorites')}</p>
              ) : (
                favorites.map((fid) => {
                  const session = sessions.find((s) => s.id === fid)
                  if (!session) return null
                  return (
                    <div key={session.id} className={`${styles.item} ${currentId === session.id ? styles.itemActive : ''}`}>
                      <Link to={`/sessions/${session.id}`} className={styles.itemLink}>
                        <div className={styles.itemHeader}>
                          <span className={styles.agentType}>{session.title || session.agent_type}</span>
                        </div>
                        {session.last_prompt && <p className={styles.lastPrompt}>{session.last_prompt}</p>}
                        <span className={styles.time}>{new Date(session.created_at).toLocaleString(locale)}</span>
                      </Link>
                      <div className={styles.itemActions}>
                        <button type="button" className={styles.favBtnActive}
                          title={t('session.favorited')}
                          onClick={(e) => toggleFavorite(session.id, e)}
                        >★</button>
                      </div>
                    </div>
                  )
                })
              )}
            </div>
          )}
        </div>

        <div className={styles.group}>
          <button type="button" className={styles.groupHeader} onClick={() => toggleGroup('manual')}>
            <span className={styles.groupArrow}>{collapsed.manual ? '▶' : '▼'}</span>
            <span className={styles.groupTitle}>📝 {t('session.title')}</span>
            <span className={styles.groupCount}>{manualSessions.length}</span>
            <span
              className={styles.addBtn} role="button" tabIndex={0}
              title={t('session.newSession')}
              onClick={(e) => { e.stopPropagation(); navigate('/') }}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.stopPropagation(); navigate('/') } }}
            >+</span>
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
                        <Link to={`/sessions/${session.id}`} className={styles.itemLink}>
                          <div className={styles.itemHeader}>
                            <span className={styles.agentType}>{session.title || session.agent_type}</span>
                          </div>
                          {session.last_prompt && <p className={styles.lastPrompt}>{session.last_prompt}</p>}
                          <span className={styles.time}>{new Date(session.created_at).toLocaleString(locale)}</span>
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
                                if (window.confirm(t('session.deleteConfirm'))) onDelete(session.id)
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
            <span className={styles.groupArrow}>{collapsed.scheduled ? '▶' : '▼'}</span>
            <span className={styles.groupTitle}>📅 {t('nav.scheduledTasks')}</span>
            <span className={styles.groupCount}>{tasks.length}</span>
          </button>
          {!collapsed.scheduled && (
            <div className={styles.groupList}>
              {recentTask && recentTask.db_session_id ? (
                <button type="button" className={styles.recentEntry}
                  onClick={() => navigate(`/sessions/${recentTask.db_session_id}`)}
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
                      onClick={() => task.db_session_id ? navigate(`/sessions/${task.db_session_id}`) : undefined}
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

      <div className={styles.footer}>
        <Link to="/" className={`${styles.navItem} ${isHome ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}>📋</span>
          <span>{t('nav.sessionList')}</span>
        </Link>
        <Link to="/scheduled-tasks" className={`${styles.navItem} ${location.pathname === '/scheduled-tasks' ? styles.navItemActive : ''}`}>
          <span className={styles.navIcon}>⏰</span>
          <span>{t('nav.scheduledTasks')}</span>
        </Link>
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
