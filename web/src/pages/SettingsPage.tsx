import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgentConfigs, updateAgentConfig, deleteAgentConfig } from '../api/agentConfigs'
import { listAgents, getAgentModels, probeAgentConfigs, clearAgentProbeCache } from '../api/agents'
import { listSessions } from '../api/sessions'
import { getNoteSettings, updateNoteSettings } from '../api/notes'
import type { AgentConfig, Session, Agent, ModelOption, ConfigOption } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import EditAgentDialog, { type AgentFormPayload } from '../components/EditAgentDialog'
import ConfigEditor from '../components/ConfigEditor'
import UserMenu from '../components/UserMenu'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import i18n from '../i18n'
import styles from './SettingsPage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'
type SettingsTab = 'language' | 'agent' | 'classify' | 'config'

function parseSettingsTab(raw: string | null): SettingsTab {
  if (raw === 'agent' || raw === 'classify' || raw === 'config') return raw
  return 'language'
}

function modelOptFromConfig(modelOpt: ConfigOption): ModelOption {
  return {
    id: modelOpt.id,
    name: modelOpt.name,
    current_value: modelOpt.current_value,
    options: modelOpt.options,
  }
}

function findModelConfigOption(opts: ConfigOption[]) {
  return opts.find((o) => o.category === 'model' && o.type === 'select')
    || opts.find((o) => o.category === 'model')
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const tab = parseSettingsTab(searchParams.get('tab'))

  function setTab(next: SettingsTab) {
    if (next === 'language') {
      setSearchParams({})
    } else {
      setSearchParams({ tab: next })
    }
  }

  const [configs, setConfigs] = useState<AgentConfig[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [defaultAgent, setDefaultAgent] = useState('')
  const [noteAgent, setNoteAgent] = useState('')
  const [noteModel, setNoteModel] = useState('')
  const [noteInterval, setNoteInterval] = useState(5)
  const [notePrompt, setNotePrompt] = useState('')
  const [noteClassifySessionId, setNoteClassifySessionId] = useState(0)
  const [noteModelOptions, setNoteModelOptions] = useState<ModelOption[]>([])
  const [noteModelProbing, setNoteModelProbing] = useState(false)
  const [noteSettingsSaving, setNoteSettingsSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editingConfig, setEditingConfig] = useState<AgentConfig | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => { if (user) loadData() }, [user])

  useEffect(() => {
    if (tab !== 'classify' || !noteAgent) {
      return
    }
    let alive = true
    setNoteModelProbing(true)

    async function loadNoteModels() {
      try {
        const cached = await getAgentModels(noteAgent)
        if (!alive) return
        const fromSession = cached.data.model_options || []
        if (fromSession.length > 0 && fromSession[0].options.length > 0) {
          setNoteModelOptions(fromSession)
          return
        }

        const probed = await probeAgentConfigs(noteAgent)
        if (!alive) return
        const modelOpt = findModelConfigOption(probed.data.config_options || [])
        if (modelOpt && modelOpt.options.length > 0) {
          setNoteModelOptions([modelOptFromConfig(modelOpt)])
        } else {
          setNoteModelOptions([])
        }
      } catch (err) {
        if (!alive) return
        setNoteModelOptions([])
        setError(err instanceof Error ? err.message : t('common.failed'))
      } finally {
        if (alive) setNoteModelProbing(false)
      }
    }

    loadNoteModels()
    return () => { alive = false }
  }, [tab, noteAgent, t])

  async function loadData() {
    setLoading(true); setError('')
    try {
      const [cfgResp, agentsResp, sessResp, noteSettingsResp] = await Promise.all([
        listAgentConfigs(), listAgents(), listSessions(), getNoteSettings(),
      ])
      setConfigs(cfgResp.data.agent_configs || [])
      setAgents(agentsResp.data.agents || [])
      setSessions(sessResp.data.sessions || [])
      setDefaultAgent(localStorage.getItem(DEFAULT_AGENT_KEY) || '')
      setNoteAgent(noteSettingsResp.data.agent_type || '')
      setNoteModel(noteSettingsResp.data.model_value || '')
      setNoteInterval(noteSettingsResp.data.classify_interval_minutes || 5)
      setNotePrompt(noteSettingsResp.data.classify_prompt || '')
      setNoteClassifySessionId(noteSettingsResp.data.classify_db_session_id || 0)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.loadFailed'))
    } finally { setLoading(false) }
  }

  function handleSetDefault(agentType: string) {
    localStorage.setItem(DEFAULT_AGENT_KEY, agentType); setDefaultAgent(agentType)
  }

  async function handleProbeNoteModel() {
    if (!noteAgent) return
    setNoteModelProbing(true); setError('')
    try {
      clearAgentProbeCache(noteAgent)
      const r = await probeAgentConfigs(noteAgent, { force: true })
      const modelOpt = findModelConfigOption(r.data.config_options || [])
      if (modelOpt && modelOpt.options.length > 0) {
        setNoteModelOptions([modelOptFromConfig(modelOpt)])
      } else {
        setNoteModelOptions([])
        setError(t('scheduledTask.probeHint'))
      }
    } catch (err) {
      setNoteModelOptions([])
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setNoteModelProbing(false)
    }
  }

  async function handleSaveNoteSettings() {
    setNoteSettingsSaving(true); setError('')
    try {
      const resp = await updateNoteSettings({
        agent_type: noteAgent,
        model_value: noteModel,
        classify_prompt: notePrompt,
        classify_interval_minutes: noteInterval,
      })
      setNoteAgent(resp.data.agent_type || '')
      setNoteModel(resp.data.model_value || '')
      setNoteInterval(resp.data.classify_interval_minutes || 5)
      setNotePrompt(resp.data.classify_prompt || '')
      setNoteClassifySessionId(resp.data.classify_db_session_id || 0)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setNoteSettingsSaving(false)
    }
  }

  function closeEditDialog() {
    if (saving) return
    setEditingConfig(null)
  }

  async function handleSaveEdit(payload: AgentFormPayload) {
    if (!editingConfig) return
    if (!payload.display_name || !payload.command) {
      setError(t('settings.validationError'))
      return
    }
    setSaving(true); setError('')
    try {
      await updateAgentConfig(editingConfig.id, payload)
      setEditingConfig(null)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setSaving(false)
    }
  }

  async function handleDeleteEditing() {
    if (!editingConfig || !window.confirm(t('settings.deleteConfirm'))) return
    setSaving(true); setError('')
    try {
      await deleteAgentConfig(editingConfig.id)
      setEditingConfig(null)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setSaving(false)
    }
  }

  function switchLang(lang: string) {
    i18n.changeLanguage(lang)
    localStorage.setItem('nexus-lang', lang)
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <div className={styles.sidebarWrap}><SessionSidebar sessions={sessions} /></div>
      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>{t('settings.title')}</h1>
          <UserMenu />
        </div>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}
        {loading ? <LoadingSpinner /> : (
          <div className={styles.body}>
            <nav className={styles.settingsNav}>
              <button
                type="button"
                className={`${styles.navItem} ${tab === 'language' ? styles.navItemActive : ''}`}
                onClick={() => setTab('language')}
              >
                {t('settings.tabLanguage')}
              </button>
              <button
                type="button"
                className={`${styles.navItem} ${tab === 'agent' ? styles.navItemActive : ''}`}
                onClick={() => setTab('agent')}
              >
                {t('settings.tabAgent')}
              </button>
              <button
                type="button"
                className={`${styles.navItem} ${tab === 'classify' ? styles.navItemActive : ''}`}
                onClick={() => setTab('classify')}
              >
                {t('settings.tabClassify')}
              </button>
              <button
                type="button"
                className={`${styles.navItem} ${tab === 'config' ? styles.navItemActive : ''}`}
                onClick={() => setTab('config')}
              >
                {t('settings.tabConfig')}
              </button>
            </nav>
            <div className={styles.content}>
              {tab === 'language' && (
                <>
                  <p className={styles.hint}>{t('settings.languageHint')}</p>
                  <div className={styles.defaultSection}>
                    <label className={styles.label}>{t('settings.language')}</label>
                    <div className={styles.langRow}>
                      <button type="button"
                        className={`${styles.langBtn} ${i18n.language === 'zh' ? styles.langBtnActive : ''}`}
                        onClick={() => switchLang('zh')}
                      >{t('settings.chinese')}</button>
                      <button type="button"
                        className={`${styles.langBtn} ${i18n.language === 'en' ? styles.langBtnActive : ''}`}
                        onClick={() => switchLang('en')}
                      >{t('settings.english')}</button>
                    </div>
                  </div>
                </>
              )}

              {tab === 'agent' && (
                <>
                  <p className={styles.hint}>{t('settings.hint')}</p>
                  <div className={styles.defaultSection}>
                    <label className={styles.label}>{t('settings.defaultAgent')}</label>
                    <div className={styles.defaultRow}>
                      <select className={styles.input} value={defaultAgent}
                        onChange={(e) => handleSetDefault(e.target.value)}
                      >
                        <option value="">{t('common.no')}</option>
                        {agents.map((a) => (
                          <option key={a.type} value={a.type}>{a.display_name}（{a.type}）</option>
                        ))}
                      </select>
                      {defaultAgent && (
                        <button type="button" className={styles.clearDefaultBtn}
                          onClick={() => { localStorage.removeItem(DEFAULT_AGENT_KEY); setDefaultAgent('') }}
                        >{t('common.cancel')}</button>
                      )}
                    </div>
                  </div>
                  <div className={styles.configList}>
                    <h2 className={styles.sectionTitle}>{t('settings.agentList')}（{configs.length}）</h2>
                    {configs.length === 0 ? (
                      <p className={styles.empty}>{t('settings.noAgents')}</p>
                    ) : (
                      configs.map((cfg) => (
                        <div key={cfg.id} className={styles.configRow}>
                          <div className={styles.configIcon}>{cfg.display_name.slice(0, 2).toUpperCase()}</div>
                          <div className={styles.configInfo}>
                            <div className={styles.configName}>{cfg.display_name}</div>
                            {cfg.description && <div className={styles.configDesc}>{cfg.description}</div>}
                          </div>
                          {cfg.enabled ? (
                            <button type="button" className={styles.disableBtn}
                              onClick={async () => {
                                try { await updateAgentConfig(cfg.id, { ...cfg, enabled: false }); await loadData() }
                                catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
                              }}
                            >{t('settings.disable')}</button>
                          ) : (
                            <button type="button" className={styles.enableBtn}
                              onClick={async () => {
                                try { await updateAgentConfig(cfg.id, { ...cfg, enabled: true }); await loadData() }
                                catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
                              }}
                            >{t('settings.enable')}</button>
                          )}
                          <button type="button" className={styles.editIconBtn} title={t('common.edit')}
                            onClick={() => setEditingConfig(cfg)}
                          >⋯</button>
                        </div>
                      ))
                    )}
                  </div>
                </>
              )}

              {tab === 'classify' && (
                <>
                  <p className={styles.hint}>{t('settings.classifyHint')}</p>

                  <div className={styles.defaultSection}>
                    <label className={styles.label}>{t('settings.noteClassifyAgent')}</label>
                    <p className={styles.sectionHint}>{t('settings.noteClassifyAgentHint')}</p>
                    <select className={styles.input} value={noteAgent}
                      onChange={(e) => {
                        setNoteAgent(e.target.value)
                        setNoteModel('')
                        setNoteModelOptions([])
                      }}
                    >
                      <option value="">{t('common.no')}</option>
                      {agents.map((a) => (
                        <option key={a.type} value={a.type}>{a.display_name}（{a.type}）</option>
                      ))}
                    </select>
                    {noteAgent && (
                      <>
                        <label className={styles.label}>{t('settings.noteClassifyModel')}</label>
                        <div className={styles.inlineRow}>
                          {noteModelOptions.length > 0 && noteModelOptions[0].options.length > 0 ? (
                            <select className={styles.input} value={noteModel}
                              onChange={(e) => setNoteModel(e.target.value)}
                            >
                              <option value="">{t('scheduledTask.defaultModel')}</option>
                              {noteModelOptions[0].options.map((o) => (
                                <option key={o.value} value={o.value}>
                                  {o.name !== o.value ? `${o.name} (${o.value})` : o.value}
                                </option>
                              ))}
                            </select>
                          ) : (
                            <input className={styles.input} type="text" value={noteModel}
                              onChange={(e) => setNoteModel(e.target.value)}
                              placeholder={t('scheduledTask.modelValuePlaceholder')}
                            />
                          )}
                          <button type="button" className={styles.secondaryBtn}
                            onClick={handleProbeNoteModel}
                            disabled={noteModelProbing}
                            title={t('scheduledTask.probeTitle')}
                          >{noteModelProbing ? t('common.loading') : t('scheduledTask.probeConfig')}</button>
                        </div>
                        <p className={styles.sectionHint}>
                          {noteModelProbing
                            ? t('common.loading')
                            : noteModelOptions.length === 0
                              ? t('scheduledTask.probeHint')
                              : t('scheduledTask.probeDone')}
                        </p>
                      </>
                    )}
                    <label className={styles.label}>{t('settings.noteClassifyInterval')}</label>
                    <p className={styles.sectionHint}>{t('settings.noteClassifyIntervalHint')}</p>
                    <input
                      className={styles.input}
                      type="number"
                      min={1}
                      max={1440}
                      value={noteInterval}
                      onChange={(e) => setNoteInterval(Math.min(1440, Math.max(1, Number(e.target.value) || 5)))}
                    />
                    <label className={styles.label}>{t('settings.noteClassifyPrompt')}</label>
                    <p className={styles.sectionHint}>{t('settings.noteClassifyPromptHint')}</p>
                    <textarea
                      className={styles.textarea}
                      rows={8}
                      value={notePrompt}
                      onChange={(e) => setNotePrompt(e.target.value)}
                    />
                    <button
                      type="button"
                      className={styles.saveNoteBtn}
                      disabled={noteSettingsSaving}
                      onClick={handleSaveNoteSettings}
                    >
                      {noteSettingsSaving ? t('notes.saving') : t('common.save')}
                    </button>
                    {noteClassifySessionId > 0 && (
                      <Link className={styles.classifyTaskLink} to={`/sessions/${noteClassifySessionId}`}>
                        {t('settings.viewClassifyTask')}
                      </Link>
                    )}
                  </div>
                </>
              )}

              {tab === 'config' && (
                <>
                  <p className={styles.hint}>{t('settings.configHint')}</p>
                  <ConfigEditor />
                </>
              )}

              <button className={styles.backBtn} type="button" onClick={() => navigate('/')}>{t('common.back')}</button>
            </div>
          </div>
        )}
      </div>

      {editingConfig && (
        <EditAgentDialog
          config={editingConfig}
          saving={saving}
          onSave={handleSaveEdit}
          onDelete={handleDeleteEditing}
          onClose={closeEditDialog}
        />
      )}
    </div>
  )
}
