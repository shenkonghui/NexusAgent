import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getSession, listMessages, cancelSession, listSessions, listCommands, listModes, listSkills, listConfigOptions, setConfigOption, deleteSession, updateSessionTitle, createSession, resumeSession } from '../api/sessions'
import { listScheduledTasks, listExecutions } from '../api/scheduledTasks'
import { listAgents, probeAgentConfigs } from '../api/agents'
import { streamPrompt, isTimeoutError } from '../api/sse'
import { parseDiffsFromMessage } from '../utils/diff'
import type { Session, Message, AgentCommand, ConfigOption, SessionMode, AgentSkill, Execution, Agent } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ModelSelector from '../components/ModelSelector'
import DirectoryPicker from '../components/DirectoryPicker'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import WorkspacePanel from '../components/WorkspacePanel'
import ContextStats from '../components/ContextStats'
import UserMenu from '../components/UserMenu'
import styles from './ChatPage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

export default function ChatPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const sessionId = Number(id)
  const hasSession = !isNaN(sessionId)
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const initialPromptRef = useRef<string>(
    (location.state as { initialPrompt?: string } | null)?.initialPrompt || '',
  )

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
  const [agentCwd, setAgentCwd] = useState('')
  const [showDirPicker, setShowDirPicker] = useState(false)
  const [creating, setCreating] = useState(false)
  const [resuming, setResuming] = useState(false)
  const [showResumePicker, setShowResumePicker] = useState(false)
  const [resumeCwd, setResumeCwd] = useState('')

  const changesCount = useMemo(() => {
    const paths = new Set<string>()
    for (const msg of messages) {
      for (const d of parseDiffsFromMessage(msg)) paths.add(d.path)
    }
    return paths.size
  }, [messages])

  // 加载会话数据（有会话时）
  const loadData = useCallback(async () => {
    if (!hasSession) return
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
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setLoading(false) }
  }, [sessionId, hasSession])

  // 加载 agent 列表和会话列表（无会话时）
  const loadHomeData = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const [agentsResp, sessionsResp] = await Promise.all([listAgents(), listSessions()])
      setAgents(agentsResp.data.agents || [])
      setAllSessions(sessionsResp.data.sessions || [])
      if (agentsResp.data.agents?.length > 0) {
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a: Agent) => a.type)
        if (saved && types.includes(saved)) setSelectedAgent(saved)
        else setSelectedAgent(agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setLoading(false) }
  }, [])

  // 监听 agent 变化，探测 config options
  useEffect(() => {
    if (hasSession || !selectedAgent) {
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
  }, [selectedAgent, hasSession])

  useEffect(() => {
    if (!user) return
    if (hasSession) {
      loadData()
    } else {
      loadHomeData()
    }
  }, [user, hasSession, loadData, loadHomeData])

  useEffect(() => {
    if (loading) return // 等待 loadData 完成后再处理首次 prompt，避免竞态
    if (!session || session.status !== 'active') return
    const pending = initialPromptRef.current
    if (!pending) return
    initialPromptRef.current = ''
    if (location.state) navigate(location.pathname, { replace: true, state: null })
    handleSend(pending)
  }, [session, loading])

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
      const resp = await createSession(selectedAgent, agentCwd, selectedModel || undefined)
      // 记住默认 agent
      localStorage.setItem(DEFAULT_AGENT_KEY, selectedAgent)
      // 导航到新建的会话页面，携带初始 prompt
      navigate(`/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      setCreating(false)
    }
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

  // 恢复异常状态的会话（error 或 closed）
  async function handleResume(cwd?: string) {
    setResuming(true); setError('')
    try {
      await resumeSession(sessionId, cwd)
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
  if (hasSession && loading) return <LoadingSpinner text={t('common.loading')} />

  // ============ 无会话模式：选择 agent/模型，首次对话 ============
  if (!hasSession) {
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
              {/* 工作目录：未设置时显示按钮，设置后置灰显示路径 */}
              {!agentCwd ? (
                <button type="button" className={styles.homeBrowseBtn}
                  onClick={() => setShowDirPicker(true)}
                >设置工作目录</button>
              ) : (
                <span className={styles.homeCwdDisplay} title={agentCwd}>{agentCwd}</span>
              )}
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
            placeholder={t('session.quickSendPlaceholder')}
          />
        </div>

        {showDirPicker && (
          <DirectoryPicker initialPath={agentCwd}
            onSelect={(path) => { setAgentCwd(path); setShowDirPicker(false) }}
            onClose={() => setShowDirPicker(false)}
          />
        )}
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
            <span className={styles.agentType}>{session?.agent_type || ''}</span>
            {session?.cwd && <span className={styles.cwd}>{session.cwd}</span>}
          </div>
          <div className={styles.actions}>
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
          scheduled={session?.source === 'scheduled'} executions={executions}
          sessionId={sessionId} cwd={session?.cwd}
        />

        {session?.status === 'active' ? (
          <PromptInput onSend={handleSend} onCancel={handleCancel}
            sending={sending} disabled={false}
            commands={commands} modes={modes} skills={skills} cwd={session?.cwd}
            placeholder={t('session.promptPlaceholder')}
          />
        ) : (
          <div className={styles.recoverBar}>
            <span className={styles.recoverText}>
              {session?.status === 'error' ? t('session.errorHint') : t('session.closedHint')}
            </span>
            <button type="button" className={styles.recoverBtn}
              onClick={() => handleResume(resumeCwd || undefined)}
              disabled={resuming}
            >{resuming ? t('common.loading') : t('session.resume')}</button>
            <button type="button" className={styles.recoverDirBtn}
              onClick={() => setShowResumePicker(true)}
              disabled={resuming}
              title={t('session.resumePrompt')}
            >{resumeCwd ? resumeCwd : t('session.resumePrompt')}</button>
          </div>
        )}
      </div>

      {showWorkspace && session && (
        <div className={styles.workspaceWrap}>
          <WorkspacePanel sessionId={sessionId} cwd={session.cwd}
            messages={messages} changesCount={changesCount}
            onClose={() => setShowWorkspace(false)}
          />
        </div>
      )}

      {showResumePicker && (
        <DirectoryPicker initialPath={resumeCwd || session?.cwd || ''}
          onSelect={(path) => { setResumeCwd(path); setShowResumePicker(false) }}
          onClose={() => setShowResumePicker(false)}
        />
      )}
    </div>
  )
}
