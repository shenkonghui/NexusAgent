import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgents, probeAgentConfigs } from '../api/agents'
import { listSessions, createSession, deleteSession, updateSessionTitle } from '../api/sessions'
import type { Agent, Session, ConfigOption } from '../types'
import AgentSelector from '../components/AgentSelector'
import DirectoryPicker from '../components/DirectoryPicker'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import PromptInput from '../components/PromptInput'
import SessionSidebar from '../components/SessionSidebar'
import UserMenu from '../components/UserMenu'
import styles from './SessionsPage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

export default function SessionsPage() {
  const { t, i18n } = useTranslation()
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()

  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [showDirPicker, setShowDirPicker] = useState(false)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [selectedAgent, setSelectedAgent] = useState('')
  const [cwd, setCwd] = useState('')
  const [creating, setCreating] = useState(false)
  const [defaultAgent, setDefaultAgent] = useState('')
  const [modelOptions, setModelOptions] = useState<ConfigOption | null>(null)
  const [selectedModel, setSelectedModel] = useState('')
  const [probing, setProbing] = useState(false)

  useEffect(() => {
    if (!user) return
    setDefaultAgent(localStorage.getItem(DEFAULT_AGENT_KEY) || '')
    loadData()
  }, [user])

  useEffect(() => {
    if (!selectedAgent) {
      setModelOptions(null)
      setSelectedModel('')
      return
    }
    let alive = true
    setProbing(true)
    probeAgentConfigs(selectedAgent)
      .then((r) => {
        if (!alive) return
        const opts = r.data.config_options || []
        const modelOpt = opts.find((o) => o.category === 'model' && o.type === 'select' && o.options.length > 0)
        if (modelOpt) {
          setModelOptions(modelOpt)
          setSelectedModel(modelOpt.current_value || modelOpt.options[0]?.value || '')
        } else { setModelOptions(null); setSelectedModel('') }
      })
      .catch(() => { if (!alive) return; setModelOptions(null); setSelectedModel('') })
      .finally(() => { if (alive) setProbing(false) })
    return () => { alive = false }
  }, [selectedAgent])

  async function loadData() {
    setLoading(true); setError('')
    try {
      const [agentsResp, sessionsResp] = await Promise.all([listAgents(), listSessions()])
      setAgents(agentsResp.data.agents || [])
      setSessions(sessionsResp.data.sessions || [])
      if (agentsResp.data.agents?.length > 0) {
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a) => a.type)
        if (saved && types.includes(saved)) setSelectedAgent(saved)
        else setSelectedAgent(agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setLoading(false) }
  }

  async function handleQuickSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, 0, selectedModel || undefined)
      navigate(`/workspaces/${resp.data.workspace_id}/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      setCreating(false)
    }
  }

  async function handleDelete(session: Session) {
    if (!window.confirm(t('session.deleteConfirm'))) return
    setError('')
    try {
      await deleteSession(session.id)
      setSessions((prev) => prev.filter((s) => s.id !== session.id))
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    }
  }

  async function handleRename(id: number, title: string) {
    setError('')
    try {
      const resp = await updateSessionTitle(id, title)
      setSessions((prev) => prev.map((s) => (s.id === id ? resp.data : s)))
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    }
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null

  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'

  return (
    <div className={styles.layout}>
      <div className={styles.sidebarWrap}>
        <SessionSidebar sessions={sessions} onRename={handleRename} />
      </div>

      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>{t('nav.sessionList')}</h1>
          <UserMenu />
        </div>

        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        {loading ? <LoadingSpinner /> : (
          <div className={styles.content}>
            <div className={styles.hero}>
              <h2 className={styles.heroTitle}>{t('session.newSession')}</h2>
              <p className={styles.heroSubtitle}>{t('session.autoCreateHint')}</p>
              <div className={styles.heroAgent}>
                <AgentSelector agents={agents} value={selectedAgent} onChange={setSelectedAgent} />
                {modelOptions && (
                  <div className={styles.modelRow}>
                    <label className={styles.modelLabel}>{t('chat.model')}</label>
                    <select className={styles.modelSelect} value={selectedModel}
                      disabled={probing || creating}
                      onChange={(e) => setSelectedModel(e.target.value)}
                    >
                      {modelOptions.options.map((v) => (
                        <option key={v.value} value={v.value}>{v.name}{v.description ? ` — ${v.description}` : ''}</option>
                      ))}
                    </select>
                  </div>
                )}
                {defaultAgent && (
                  <p className={styles.defaultHint}>{t('session.defaultAgent')}: {defaultAgent}</p>
                )}
              </div>
              <div className={styles.heroPrompt}>
                <PromptInput
                  onSend={handleQuickSend} sending={creating}
                  disabled={!selectedAgent || creating}
                  placeholder={t('session.quickSendPlaceholder')}
                />
              </div>
              <div className={styles.advanced}>
                <button type="button" className={styles.advancedToggle}
                  onClick={() => setShowAdvanced(!showAdvanced)}
                >
                  {showAdvanced ? '▾' : '▸'} {t('session.advancedOptions')}{cwd ? ` (${cwd})` : ''}
                </button>
                {showAdvanced && (
                  <div className={styles.cwdRow}>
                    <input className={styles.input} type="text" value={cwd}
                      onChange={(e) => setCwd(e.target.value)}
                      placeholder={t('scheduledTask.cwdPlaceholder')}
                    />
                    <button type="button" className={styles.browseBtn}
                      onClick={() => setShowDirPicker(true)}
                    >{t('common.search')}</button>
                  </div>
                )}
              </div>
            </div>

            {showDirPicker && (
              <DirectoryPicker initialPath={cwd}
                onSelect={(path) => { setCwd(path); setShowDirPicker(false) }}
                onClose={() => setShowDirPicker(false)}
              />
            )}

            <div className={styles.sessionList}>
              <h3 className={styles.listTitle}>{t('session.history')}</h3>
              {sessions.length === 0 ? (
                <p className={styles.empty}>{t('session.noSessions')}</p>
              ) : (
                sessions.map((session) => (
                  <div key={session.id} className={styles.sessionCard}
                    onClick={() => navigate(`/sessions/${session.id}`)}
                  >
                    <div className={styles.sessionHeader}>
                      <span className={styles.sessionAgent}>{session.title || session.agent_type}</span>
                      <span className={`${styles.sessionStatus} ${styles[`status_${session.status}`] || ''}`}>
                        {session.status === 'active' ? t('session.active') : session.status === 'closed' ? t('session.closed') : t('status.error')}
                      </span>
                    </div>
                    {session.last_prompt && <p className={styles.sessionPrompt}>{session.last_prompt}</p>}
                    <div className={styles.sessionFooter}>
                      <span className={styles.sessionTime}>{new Date(session.created_at).toLocaleString(locale)}</span>
                      <button type="button" className={styles.deleteBtn}
                        onClick={(e) => { e.stopPropagation(); handleDelete(session) }}
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
