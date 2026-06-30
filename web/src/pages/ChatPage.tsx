import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getSession, listMessages, closeSession, cancelSession, resumeSession, listSessions, listCommands, listModes, listSkills, listConfigOptions, setConfigOption, deleteSession, updateSessionTitle } from '../api/sessions'
import { listScheduledTasks, listExecutions } from '../api/scheduledTasks'
import { streamPrompt, isTimeoutError } from '../api/sse'
import { parseDiffsFromMessage } from '../utils/diff'
import type { Session, Message, AgentCommand, ConfigOption, SessionMode, AgentSkill, Execution, ScheduledTask } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ModelSelector from '../components/ModelSelector'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import WorkspacePanel from '../components/WorkspacePanel'
import ContextStats from '../components/ContextStats'
import UserMenu from '../components/UserMenu'
import styles from './ChatPage.module.css'

export default function ChatPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const sessionId = Number(id)
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const initialPromptRef = useRef<string>(
    (location.state as { initialPrompt?: string } | null)?.initialPrompt || '',
  )

  const [session, setSession] = useState<Session | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [allSessions, setAllSessions] = useState<Session[]>([])
  const [commands, setCommands] = useState<AgentCommand[]>([])
  const [modes, setModes] = useState<SessionMode[]>([])
  const [skills, setSkills] = useState<AgentSkill[]>([])
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [configOptions, setConfigOptions] = useState<ConfigOption[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sending, setSending] = useState(false)
  const [showWorkspace, setShowWorkspace] = useState(true)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [lastFailedPrompt, setLastFailedPrompt] = useState('')
  const [retryable, setRetryable] = useState(false)
  const [executions, setExecutions] = useState<Execution[]>([])

  const changesCount = useMemo(() => {
    const paths = new Set<string>()
    for (const msg of messages) {
      for (const d of parseDiffsFromMessage(msg)) paths.add(d.path)
    }
    return paths.size
  }, [messages])

  const loadData = useCallback(async () => {
    if (!sessionId) return
    setLoading(true); setError('')
    try {
      const [sessionResp, msgResp, sessionsResp] = await Promise.all([
        getSession(sessionId), listMessages(sessionId), listSessions(),
      ])
      setSession(sessionResp.data)
      setMessages(msgResp.data.messages || [])
      setAllSessions(sessionsResp.data.sessions || [])
      if (sessionResp.data.source === 'scheduled') {
        loadExecutions(sessionId)
      } else { setExecutions([]) }
      listCommands(sessionId).then((r) => setCommands(r.data.commands || [])).catch(() => setCommands([]))
      listModes(sessionId).then((r) => setModes(r.data.modes || [])).catch(() => setModes([]))
      listSkills(sessionId).then((r) => setSkills(r.data.skills || [])).catch(() => setSkills([]))
      listConfigOptions(sessionId).then((r) => setConfigOptions(r.data.config_options || [])).catch(() => setConfigOptions([]))
      listScheduledTasks().then((r) => setTasks(r.data.tasks || [])).catch(() => setTasks([]))
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setLoading(false) }
  }, [sessionId])

  useEffect(() => { if (user) loadData() }, [user, loadData])

  useEffect(() => {
    if (!session || session.status !== 'active') return
    const pending = initialPromptRef.current
    if (!pending) return
    initialPromptRef.current = ''
    if (location.state) navigate(location.pathname, { replace: true, state: null })
    handleSend(pending)
  }, [session])

  async function loadExecutions(dbSessionId: number) {
    try {
      const tasksResp = await listScheduledTasks()
      const task = (tasksResp.data.tasks || []).find((t) => t.db_session_id === dbSessionId)
      if (task) { const execResp = await listExecutions(task.id); setExecutions(execResp.data.executions || []) }
    } catch { /* silent */ }
  }

  async function handleSend(prompt: string) {
    if (!session || session.status !== 'active') return
    setSending(true); setError(''); setRetryable(false); setLastFailedPrompt('')
    await streamPrompt(
      sessionId, prompt,
      (msg) => setMessages((prev) => [...prev, msg]),
      () => { setSending(false); setLastFailedPrompt(''); setRetryable(false); loadData() },
      (err) => { setSending(false); setError(err.message); if (isTimeoutError(err)) { setLastFailedPrompt(prompt); setRetryable(true) } },
    )
  }

  async function handleRetry() { if (!lastFailedPrompt) return; setError(''); setRetryable(false); await handleSend(lastFailedPrompt) }

  async function handleCancel() { try { await cancelSession(sessionId); setSending(false) } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) } }

  async function handleResume() {
    if (!session) return; setError('')
    let cwd: string | undefined
    if (session.status === 'closed') {
      cwd = window.prompt(t('session.resumePrompt'), session.cwd || '') ?? undefined
      if (cwd === undefined) return
    }
    try {
      const resp = await resumeSession(sessionId, cwd)
      setSession(resp.data)
      listCommands(sessionId).then((r) => setCommands(r.data.commands || [])).catch(() => setCommands([]))
      listModes(sessionId).then((r) => setModes(r.data.modes || [])).catch(() => setModes([]))
      listSkills(sessionId).then((r) => setSkills(r.data.skills || [])).catch(() => setSkills([]))
      listConfigOptions(sessionId).then((r) => setConfigOptions(r.data.config_options || [])).catch(() => setConfigOptions([]))
      listScheduledTasks().then((r) => setTasks(r.data.tasks || [])).catch(() => setTasks([]))
    } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  async function handleSetConfigOption(configId: string, value: string) {
    setError('')
    setConfigOptions((prev) => prev.map((o) => (o.id === configId ? { ...o, current_value: value } : o)))
    try { await setConfigOption(sessionId, configId, value) }
    catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      listConfigOptions(sessionId).then((r) => setConfigOptions(r.data.config_options || [])).catch(() => {})
    }
  }

  async function handleClose() { try { await closeSession(sessionId); navigate('/') } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) } }

  async function handleDeleteSession(id: number) {
    setError('')
    try { await deleteSession(id); if (id === sessionId) navigate('/'); else setAllSessions((prev) => prev.filter((s) => s.id !== id)) }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  async function handleRenameSession(id: number, title: string) {
    setError('')
    try { const resp = await updateSessionTitle(id, title); setAllSessions((prev) => prev.map((s) => (s.id === id ? resp.data : s))); if (id === sessionId) setSession(resp.data) }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null
  if (loading) return <LoadingSpinner text={t('common.loading')} />

  return (
    <div className={styles.layout}>
      {!sidebarCollapsed && (
        <div className={styles.sidebarWrap}>
          <SessionSidebar sessions={allSessions} currentId={sessionId}
            onDelete={handleDeleteSession} onRename={handleRenameSession}
            onCollapse={() => setSidebarCollapsed(true)}
          />
        </div>
      )}

      <div className={styles.main}>
        <div className={styles.header}>
          <div className={styles.sessionInfo}>
            {sidebarCollapsed && (
              <button className={styles.iconBtn} onClick={() => setSidebarCollapsed(false)} type="button" title={t('common.open')}>
                ☰
              </button>
            )}
            <span className={styles.agentType}>{session?.agent_type || ''}</span>
            <span className={`${styles.statusBadge} ${styles[`status_${session?.status}`] || ''}`}>
              {session?.status === 'active' ? t('session.active') : session?.status === 'closed' ? t('session.closed') : t('status.error')}
            </span>
            {session?.cwd && <span className={styles.cwd}>{session.cwd}</span>}
          </div>
          <div className={styles.actions}>
            <button className={`${styles.fileBtn} ${showWorkspace ? styles.fileBtnActive : ''}`}
              onClick={() => setShowWorkspace(!showWorkspace)} type="button" title={t('chat.workspace')}
            >🗂</button>
            {(session?.status === 'error' || session?.status === 'closed') && (
              <button className={styles.resumeBtn} onClick={handleResume} type="button">
                {session?.status === 'closed' ? t('session.resume') : t('session.reconnect')}
              </button>
            )}
            {session?.status === 'active' && (
              <button className={styles.closeBtn} onClick={handleClose} type="button" title={t('common.close')}>✕</button>
            )}
            <UserMenu />
          </div>
        </div>

        {session?.status === 'active' && (
          <div className={styles.configBar}>
            <ModelSelector options={configOptions} onApply={handleSetConfigOption} disabled={sending} />
            <div className={styles.statsArea}><ContextStats messages={messages} /></div>
          </div>
        )}

        {error && (
          <ErrorBanner
            message={retryable ? `${error} (${t('common.retry')})` : error}
            onClose={() => { setError(''); setRetryable(false) }}
            onRetry={retryable ? handleRetry : undefined}
          />
        )}

        <MessageList messages={messages} loading={sending}
          scheduled={session?.source === 'scheduled'} executions={executions}
          sessionId={sessionId} cwd={session?.cwd}
        />

        <PromptInput onSend={handleSend} onCancel={handleCancel}
          sending={sending} disabled={session?.status !== 'active'}
          commands={commands} modes={modes} skills={skills} tasks={tasks} cwd={session?.cwd}
          placeholder={session?.status === 'closed'
            ? t('session.closedHint')
            : session?.status === 'error'
              ? t('session.errorHint')
              : t('session.promptPlaceholder')
          }
        />
      </div>

      {showWorkspace && session && (
        <div className={styles.workspaceWrap}>
          <WorkspacePanel sessionId={sessionId} cwd={session.cwd}
            messages={messages} changesCount={changesCount}
            onClose={() => setShowWorkspace(false)}
          />
        </div>
      )}
    </div>
  )
}
