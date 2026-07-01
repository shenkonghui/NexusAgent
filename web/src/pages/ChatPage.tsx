import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getSession, listMessages, cancelSession, listCommands, listModes, listSkills, listConfigOptions, setConfigOption, deleteSession, updateSessionTitle, createSession, resumeSession } from '../api/sessions'
import { getWorkspace } from '../api/workspaces'
import { listScheduledTasks, listExecutions } from '../api/scheduledTasks'
import { listAgents, probeAgentConfigs, listAgentCommands, listAgentModes } from '../api/agents'
import { listSkillsByPath } from '../api/filesystem'
import { WORKSPACE_STORAGE_KEY, useCurrentWorkspace } from '../hooks/useCurrentWorkspace'
import { streamPrompt, isTimeoutError } from '../api/sse'
import { parseDiffsFromMessage } from '../utils/diff'
import type { Session, Message, AgentCommand, ConfigOption, SessionMode, AgentSkill, Execution, Agent } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ModelSelector from '../components/ModelSelector'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import WorkspacePanel from '../components/WorkspacePanel'
import ContextStats from '../components/ContextStats'
import UserMenu from '../components/UserMenu'
import WorkspaceSelector from '../components/WorkspaceSelector'
import styles from './ChatPage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

type NavigateState = { initialPrompt?: string; createdSession?: Session }

export default function ChatPage() {
  const { t, i18n } = useTranslation()
  const { wid, sid } = useParams<{ wid?: string; sid?: string }>()
  const urlWorkspaceId = wid ? Number(wid) : NaN
  const sessionId = sid ? Number(sid) : NaN
  const hasSession = !isNaN(sessionId)
  const { user, loading: authLoading } = useRequireAuth()
  const { workspaceId: storedWorkspaceId, selectWorkspace, reload: reloadWorkspace } = useCurrentWorkspace(!!user && !hasSession)
  const workspaceId = !isNaN(urlWorkspaceId) ? urlWorkspaceId : storedWorkspaceId
  const navigate = useNavigate()
  const location = useLocation()
  const initialPromptRef = useRef<string>('')
  const bootstrappedSessionIdRef = useRef<number | null>(null)
  // location.state 变化时同步到 ref（navigate 跳转不会重新挂载组件，useRef 不会自动更新）
  const navState = location.state as NavigateState | null
  if (navState?.initialPrompt) {
    initialPromptRef.current = navState.initialPrompt
  }

  // 会话相关状态
  const [session, setSession] = useState<Session | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [allSessions, setAllSessions] = useState<Session[]>([])
  const [commands, setCommands] = useState<AgentCommand[]>([])
  const [modes, setModes] = useState<SessionMode[]>([])
  const [skills, setSkills] = useState<AgentSkill[]>([])
  const [configOptions, setConfigOptions] = useState<ConfigOption[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sending, setSending] = useState(false)
  const [showWorkspace, setShowWorkspace] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [lastFailedPrompt, setLastFailedPrompt] = useState('')
  const [retryable, setRetryable] = useState(false)
  const [executions, setExecutions] = useState<Execution[]>([])

  // 无会话模式下的 agent / 模型选择状态
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgent, setSelectedAgent] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [probeConfigs, setProbeConfigs] = useState<ConfigOption[]>([])
  const [probing, setProbing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [resuming, setResuming] = useState(false)
  const [homeCommands, setHomeCommands] = useState<AgentCommand[]>([])
  const [homeModes, setHomeModes] = useState<SessionMode[]>([])
  const [homeSkills, setHomeSkills] = useState<AgentSkill[]>([])
  const [workspaceCwd, setWorkspaceCwd] = useState('')

  const bootstrapSession = navState?.createdSession?.id === sessionId ? navState.createdSession : null
  const isCreateMode = !hasSession && location.pathname === '/new'
  const activeSession = session ?? bootstrapSession

  const changesCount = useMemo(() => {
    const paths = new Set<string>()
    for (const msg of messages) {
      for (const d of parseDiffsFromMessage(msg)) paths.add(d.path)
    }
    return paths.size
  }, [messages])

  // 加载会话数据（有会话时）；quiet 模式下不阻塞 UI（用于新建会话后的后台刷新）
  const loadData = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!hasSession) return
    if (!opts?.quiet) { setLoading(true); setError('') }
    try {
      const [sessionResp, msgResp] = await Promise.all([
        getSession(sessionId), listMessages(sessionId),
      ])
      setSession(sessionResp.data)
      setMessages(msgResp.data.messages || [])
      if (workspaceId) {
        const wsResp = await getWorkspace(workspaceId)
        setAllSessions(wsResp.data.sessions || [])
      } else {
        setAllSessions([])
      }
      if (sessionResp.data.source === 'scheduled') {
        loadExecutions(sessionId)
      } else { setExecutions([]) }
      listCommands(sessionId).then((r) => setCommands(r.data.commands || [])).catch(() => setCommands([]))
      listModes(sessionId).then((r) => setModes(r.data.modes || [])).catch(() => setModes([]))
      listSkills(sessionId).then((r) => setSkills(r.data.skills || [])).catch(() => setSkills([]))
      listConfigOptions(sessionId).then((r) => setConfigOptions(r.data.config_options || [])).catch(() => setConfigOptions([]))
    } catch (err) {
      if (!opts?.quiet) setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { if (!opts?.quiet) setLoading(false) }
  }, [sessionId, hasSession, workspaceId])

  // 加载 agent 列表和会话列表（无会话时）
  const loadHomeData = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const agentsResp = await listAgents()
      setAgents(agentsResp.data.agents || [])
      if (workspaceId) {
        const wsResp = await getWorkspace(workspaceId)
        setAllSessions(wsResp.data.sessions || [])
        setWorkspaceCwd(wsResp.data.workspace?.cwd || '')
      } else {
        setAllSessions([])
        setWorkspaceCwd('')
      }
      if (agentsResp.data.agents?.length > 0) {
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a: Agent) => a.type)
        if (saved && types.includes(saved)) setSelectedAgent(saved)
        else setSelectedAgent(agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setLoading(false) }
  }, [workspaceId])

  function handleWorkspaceChange(id: number) {
    if (hasSession) {
      localStorage.setItem(WORKSPACE_STORAGE_KEY, String(id))
      navigate('/')
      return
    }
    selectWorkspace(id).catch((err) => setError(err instanceof Error ? err.message : t('common.failed')))
  }

  function handleWorkspaceRefresh() {
    reloadWorkspace()
      .then(() => (hasSession ? loadData({ quiet: true }) : loadHomeData()))
      .catch((err) => setError(err instanceof Error ? err.message : t('common.failed')))
  }

  // 监听 agent 变化，探测 config options
  useEffect(() => {
    if (hasSession || !isCreateMode || !selectedAgent) {
      setProbeConfigs([])
      setSelectedModel('')
      return
    }
    let alive = true
    setProbing(true)
    probeAgentConfigs(selectedAgent)
      .then((r) => {
        if (!alive) return
        const opts = r.data.config_options || []
        setProbeConfigs(opts)
        // 从探测结果中提取模型默认值，用于创建会话时传递
        const modelOpt = opts.find((o) => o.category === 'model')
        if (modelOpt) {
          setSelectedModel(modelOpt.current_value || modelOpt.options[0]?.value || '')
        } else { setSelectedModel('') }
      })
      .catch((err) => {
        if (!alive) return
        setProbeConfigs([])
        setSelectedModel('')
        setError(err instanceof Error ? err.message : '探测配置失败')
      })
      .finally(() => { if (alive) setProbing(false) })
    return () => { alive = false }
  }, [selectedAgent, hasSession, isCreateMode])

  // 新建任务页：加载 agent 级 slash command / mode（探测完成后刷新）
  useEffect(() => {
    if (hasSession || !isCreateMode || !selectedAgent || probing) {
      if (hasSession || !isCreateMode) {
        setHomeCommands([])
        setHomeModes([])
      }
      return
    }
    listAgentCommands(selectedAgent, workspaceCwd || undefined).then((r) => setHomeCommands(r.data.commands || [])).catch(() => setHomeCommands([]))
    listAgentModes(selectedAgent).then((r) => setHomeModes(r.data.modes || [])).catch(() => setHomeModes([]))
  }, [selectedAgent, hasSession, isCreateMode, probing, workspaceCwd])

  // 新建任务页：加载 skills（与 agent 无关；cwd 为空时仍扫用户目录）
  useEffect(() => {
    if (hasSession || !isCreateMode) {
      setHomeSkills([])
      return
    }
    listSkillsByPath(workspaceCwd || undefined)
      .then((r) => setHomeSkills(r.data.skills || []))
      .catch(() => setHomeSkills([]))
  }, [workspaceCwd, hasSession, isCreateMode])

  useEffect(() => {
    if (!user) return
    if (hasSession) {
      const created = navState?.createdSession
      if (created?.id === sessionId && bootstrappedSessionIdRef.current !== sessionId) {
        bootstrappedSessionIdRef.current = sessionId
        setSession(created)
        setMessages([])
        setLoading(false)
        setCreating(false)
        setAllSessions((prev) => prev.some((s) => s.id === created.id) ? prev : [created, ...prev])
        loadData({ quiet: true })
      } else if (bootstrappedSessionIdRef.current !== sessionId) {
        loadData()
      }
    } else {
      bootstrappedSessionIdRef.current = null
      loadHomeData()
    }
  }, [user, hasSession, sessionId, loadData, loadHomeData, navState?.createdSession])

  useEffect(() => {
    if (loading && !bootstrapSession) return // 新建会话有 bootstrap 数据时不等待 loadData
    if (!activeSession || activeSession.status !== 'active') return
    const pending = initialPromptRef.current
    if (!pending) return
    initialPromptRef.current = ''
    if (location.state) navigate(location.pathname, { replace: true, state: null })
    handleSend(pending)
  }, [activeSession, loading, bootstrapSession])

  async function loadExecutions(dbSessionId: number) {
    try {
      const tasksResp = await listScheduledTasks()
      const task = (tasksResp.data.tasks || []).find((t) => t.db_session_id === dbSessionId)
      if (task) { const execResp = await listExecutions(task.id); setExecutions(execResp.data.executions || []) }
    } catch { /* silent */ }
  }

  // 无会话时：创建会话并发起首次对话
  async function handleFirstSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, workspaceId || 0, selectedModel || undefined)
      localStorage.setItem(DEFAULT_AGENT_KEY, selectedAgent)
      navigate(`/workspaces/${resp.data.workspace_id}/sessions/${resp.data.id}`, {
        state: { initialPrompt: prompt, createdSession: resp.data },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      setCreating(false)
    }
  }

  async function handleSend(prompt: string) {
    if (!activeSession || activeSession.status !== 'active') return
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

  // 恢复异常状态的会话（error 或 closed）
  async function handleResume() {
    setResuming(true); setError('')
    try {
      await resumeSession(sessionId)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setResuming(false) }
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

  async function handleDeleteSession(id: number) {
    setError('')
    try {
      await deleteSession(id)
      if (id === sessionId) {
        localStorage.setItem(WORKSPACE_STORAGE_KEY, String(workspaceId))
        navigate('/')
      } else {
        setAllSessions((prev) => prev.filter((s) => s.id !== id))
      }
    } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  async function handleRenameSession(id: number, title: string) {
    setError('')
    try { const resp = await updateSessionTitle(id, title); setAllSessions((prev) => prev.map((s) => (s.id === id ? resp.data : s))); if (id === sessionId) setSession(resp.data) }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null
  if (hasSession && loading && !activeSession) return <LoadingSpinner text={t('common.loading')} />

  // ============ 无会话模式：任务列表 / 新建任务 ============
  if (!hasSession) {
    const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'
    const manualSessions = allSessions.filter((s) => !s.source || s.source === 'manual')

    if (!isCreateMode) {
      return (
        <div className={styles.layout}>
          <div className={styles.sidebarWrap}>
            <SessionSidebar sessions={allSessions} onDelete={handleDeleteSession} onRename={handleRenameSession} />
          </div>

          <div className={styles.main}>
            <div className={styles.header}>
              <div className={styles.sessionInfo}>
                <span className={styles.agentType}>{t('nav.sessionList')}</span>
              </div>
              <div className={styles.actions}>
                <WorkspaceSelector value={workspaceId} onChange={handleWorkspaceChange} onRefresh={handleWorkspaceRefresh} onError={setError} />
                <button type="button" className={styles.newTaskBtn} onClick={() => navigate('/new')}>+ {t('session.newSession')}</button>
                <UserMenu />
              </div>
            </div>

            {error && <ErrorBanner message={error} onClose={() => setError('')} />}

            {loading ? <LoadingSpinner /> : (
              <div className={styles.listContent}>
                <h3 className={styles.listTitle}>{t('session.history')}</h3>
                <div className={styles.sessionList}>
                  {manualSessions.length === 0 ? (
                    <p className={styles.listEmpty}>{t('session.noSessions')}</p>
                  ) : (
                    manualSessions.map((item) => (
                      <div key={item.id} className={styles.sessionCard}
                        onClick={() => navigate(`/sessions/${item.id}`)}
                      >
                        <div className={styles.sessionHeader}>
                          <span className={styles.sessionAgent}>{item.title || item.agent_type}</span>
                          {(item.status === 'active' || item.status === 'error') && (
                            <span className={`${styles.sessionStatus} ${styles[`status_${item.status}`] || ''}`}>
                              {item.status === 'active' ? t('session.active') : t('status.error')}
                            </span>
                          )}
                        </div>
                        {item.last_prompt && <p className={styles.sessionPrompt}>{item.last_prompt}</p>}
                        <div className={styles.sessionFooter}>
                          <span className={styles.sessionTime}>{new Date(item.created_at).toLocaleString(locale)}</span>
                          <button type="button" className={styles.listDeleteBtn}
                            onClick={(e) => { e.stopPropagation(); handleDeleteSession(item.id) }}
                          >{t('common.delete')}</button>
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      )
    }

    return (
      <div className={styles.layout}>
        <div className={styles.sidebarWrap}>
          <SessionSidebar sessions={allSessions} onDelete={handleDeleteSession} onRename={handleRenameSession} />
        </div>

        <div className={styles.main}>
          <div className={styles.header}>
            <div className={styles.sessionInfo}>
              <span className={styles.agentType}>{t('session.newSession')}</span>
            </div>
            <div className={styles.actions}>
              <WorkspaceSelector value={workspaceId} onChange={handleWorkspaceChange} onRefresh={handleWorkspaceRefresh} onError={setError} />
              <UserMenu />
            </div>
          </div>

          <div className={styles.configBar}>
            <div className={styles.homeConfigRow}>
              <div className={styles.homeConfigItem}>
                <label className={styles.homeConfigLabel}>Agent</label>
                <select className={styles.homeConfigSelect}
                  value={selectedAgent}
                  onChange={(e) => setSelectedAgent(e.target.value)}
                  disabled={creating}
                >
                  {agents.length === 0 && <option value="">无可用 Agent</option>}
                  {agents.map((agent) => (
                    <option key={agent.type} value={agent.type}>{agent.display_name}</option>
                  ))}
                </select>
              </div>
              {/* 渲染可选择的配置项（模型用可过滤输入，模式等用下拉框） */}
              {probeConfigs.filter((o) => o.type === 'select' && o.options.length > 0).map((opt) => {
                const label = opt.category === 'model' ? '模型'
                  : opt.category === 'mode' ? '模式'
                  : opt.category === 'thought_level' ? '思考级别'
                  : opt.name
                const isModel = opt.category === 'model'
                const listId = `list-${opt.id}`
                return (
                  <div key={opt.id} className={styles.homeConfigItem}>
                    <label className={styles.homeConfigLabel}>{label}</label>
                    {isModel ? (
                      <>
                        <input className={styles.homeConfigInput}
                          list={listId}
                          value={opt.current_value || ''}
                          onChange={(e) => {
                            const val = e.target.value
                            setProbeConfigs((prev) => prev.map((o) => (o.id === opt.id ? { ...o, current_value: val } : o)))
                            setSelectedModel(val)
                          }}
                          disabled={probing || creating}
                          placeholder="输入过滤或选择"
                        />
                        <datalist id={listId}>
                          {opt.options.map((v) => (
                            <option key={v.value} value={v.value}>{v.name}{v.description ? ` — ${v.description}` : ''}</option>
                          ))}
                        </datalist>
                      </>
                    ) : (
                      <select className={styles.homeConfigSelect}
                        value={opt.current_value || ''}
                        disabled={probing || creating}
                        onChange={(ev) => setProbeConfigs((prev) => prev.map((o) => (o.id === opt.id ? { ...o, current_value: ev.target.value } : o)))}
                      >
                        {opt.options.map((v) => (
                          <option key={v.value} value={v.value}>
                            {v.name.length > 10 ? v.name.slice(0, 10) + '…' : v.name}
                          </option>
                        ))}
                      </select>
                    )}
                  </div>
                )
              })}
              {/* 探测无配置时提供手动输入 */}
              {!probing && !loading && probeConfigs.filter((o) => o.type === 'select' && o.options.length > 0).length === 0 && (
                <div className={styles.homeConfigItem}>
                  <label className={styles.homeConfigLabel}>模型</label>
                  <input className={styles.homeConfigInput}
                    type="text" value={selectedModel}
                    onChange={(e) => setSelectedModel(e.target.value)}
                    placeholder="手动输入模型 ID"
                    disabled={creating}
                  />
                </div>
              )}
              {probing && <span className={styles.homeConfigHint}>探测配置中...</span>}
              {/* 工作目录：后续由 workspace 管理 */}
            </div>
          </div>

          {error && <ErrorBanner message={error} onClose={() => setError('')} />}

          {loading ? <LoadingSpinner /> : (
            <div className={styles.homeContent}>
              <div className={styles.homeHero}>
                <h2 className={styles.homeTitle}>{t('session.newSession')}</h2>
                <p className={styles.homeSubtitle}>{t('session.autoCreateHint')}</p>
              </div>
            </div>
          )}

          <PromptInput onSend={handleFirstSend}
            sending={creating}
            disabled={!selectedAgent || creating}
            commands={homeCommands}
            modes={homeModes}
            skills={homeSkills}
            cwd={workspaceCwd}
            placeholder={t('session.quickSendPlaceholder')}
          />
        </div>

        
      </div>
    )
  }

  // ============ 有会话模式 ============

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
            <span className={styles.agentType}>{activeSession?.agent_type || ''}</span>
            {activeSession?.workspace?.cwd && <span className={styles.cwd}>{activeSession.workspace.cwd}</span>}
          </div>
          <div className={styles.actions}>
            <WorkspaceSelector value={workspaceId} onChange={handleWorkspaceChange} onRefresh={handleWorkspaceRefresh} onError={setError} />
            <button className={`${styles.fileBtn} ${showWorkspace ? styles.fileBtnActive : ''}`}
              onClick={() => setShowWorkspace(!showWorkspace)} type="button" title={t('chat.workspace')}
            >🗂</button>
            <UserMenu />
          </div>
        </div>

        <div className={styles.configBar}>
          <ModelSelector options={configOptions} onApply={handleSetConfigOption} disabled={sending} />
          <div className={styles.statsArea}><ContextStats messages={messages} /></div>
        </div>

        {error && (
          <ErrorBanner
            message={retryable ? `${error} (${t('common.retry')})` : error}
            onClose={() => { setError(''); setRetryable(false) }}
            onRetry={retryable ? handleRetry : undefined}
          />
        )}

        <MessageList messages={messages} loading={sending}
          scheduled={activeSession?.source === 'scheduled'} executions={executions}
          sessionId={sessionId} cwd={activeSession?.workspace?.cwd || ''}
        />

        {activeSession?.status === 'active' ? (
          <PromptInput onSend={handleSend} onCancel={handleCancel}
            sending={sending} disabled={false}
            commands={commands} modes={modes} skills={skills} cwd={activeSession?.workspace?.cwd || ''}
            placeholder={t('session.promptPlaceholder')}
          />
        ) : (
          <div className={styles.recoverBar}>
            <span className={styles.recoverText}>
              {activeSession?.status === 'error' ? t('session.errorHint') : t('session.closedHint')}
            </span>
            <button type="button" className={styles.recoverBtn}
              onClick={handleResume}
              disabled={resuming}
            >{resuming ? t('common.loading') : t('session.resume')}</button>
          </div>
        )}
      </div>

      {showWorkspace && activeSession && (
        <div className={styles.workspaceWrap}>
          <WorkspacePanel sessionId={sessionId} cwd={activeSession.workspace?.cwd || ''}
            messages={messages} changesCount={changesCount}
            onClose={() => setShowWorkspace(false)}
          />
        </div>
      )}

      
    </div>
  )
}
