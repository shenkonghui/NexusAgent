import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { useCurrentWorkspace } from '../hooks/useCurrentWorkspace'
import { listAgentConfigs, updateAgentConfig, deleteAgentConfig } from '../api/agentConfigs'
import { listAgents, getAgentModels, probeAgentConfigs, clearAgentProbeCache } from '../api/agents'
import { getNoteSettings, updateNoteSettings, generateNoteMCPToken } from '../api/notes'
import { getTaskSettings, updateTaskSettings } from '../api/tasks'
import { getAgentPrefs, patchAgentPrefs } from '../api/agentPrefs'
import type { AgentConfig, Agent, ModelOption, ConfigOption, TaskSettings } from '../types'
import { tasksUrl } from '../utils/routes'
import AppLayout, { SidebarToggleButton } from '../components/AppLayout'
import EditAgentDialog, { type AgentFormPayload } from '../components/EditAgentDialog'
import ConfigEditor from '../components/ConfigEditor'
import UserMenu from '../components/UserMenu'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import i18n from '../i18n'
import styles from './SettingsPage.module.css'

type SettingsTab = 'language' | 'agent' | 'classify' | 'config' | 'task'

function parseSettingsTab(raw: string | null): SettingsTab {
  if (raw === 'agent' || raw === 'classify' || raw === 'config' || raw === 'task') return raw
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

function buildNoteMcpConfig(endpoint: string, token: string): string {
  return JSON.stringify({
    mcpServers: {
      'nexus-notes': {
        type: 'http',
        url: endpoint,
        headers: { Authorization: `Bearer ${token}` },
      },
    },
  }, null, 2)
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
  const [configSearch, setConfigSearch] = useState('')
  const [agents, setAgents] = useState<Agent[]>([])
  const { workspaceId, sessions } = useCurrentWorkspace(!!user)
  const [defaultAgent, setDefaultAgent] = useState('')
  const [noteAgent, setNoteAgent] = useState('')
  const [noteModel, setNoteModel] = useState('')
  const [noteInterval, setNoteInterval] = useState(5)
  const [notePrompt, setNotePrompt] = useState('')
  const [noteClassifySessionId, setNoteClassifySessionId] = useState(0)
  const [noteMcpToken, setNoteMcpToken] = useState('')
  const [noteMcpGenerating, setNoteMcpGenerating] = useState(false)
  const [noteModelOptions, setNoteModelOptions] = useState<ModelOption[]>([])
  const [noteModelProbing, setNoteModelProbing] = useState(false)
  const [noteSettingsSaving, setNoteSettingsSaving] = useState(false)
  // 任务设置状态
  const [taskAutoTag, setTaskAutoTag] = useState(true)
  const [taskAutoTitle, setTaskAutoTitle] = useState(true)
  const [taskAgent, setTaskAgent] = useState('')
  const [taskTags, setTaskTags] = useState<string[]>([])
  const [taskTagInput, setTaskTagInput] = useState('')
  const [taskTagPrompt, setTaskTagPrompt] = useState('')
  const [taskTitlePrompt, setTaskTitlePrompt] = useState('')
  const [taskSettingsSaving, setTaskSettingsSaving] = useState(false)
  const [taskSettingsSaved, setTaskSettingsSaved] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editingConfig, setEditingConfig] = useState<AgentConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const noteMcpEndpoint = `${window.location.origin}/mcp/notes`
  const noteMcpConfig = noteMcpToken ? buildNoteMcpConfig(noteMcpEndpoint, noteMcpToken) : ''

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
      const [cfgResp, agentsResp, noteSettingsResp, taskSettingsResp] = await Promise.all([
        listAgentConfigs(), listAgents(), getNoteSettings(), getTaskSettings(),
      ])
      setConfigs(cfgResp.data.agent_configs || [])
      setAgents(agentsResp.data.agents || [])
      const prefsResp = await getAgentPrefs().catch(() => ({ data: { last_agent_type: '' } }))
      setDefaultAgent(prefsResp.data.last_agent_type || '')
      setNoteAgent(noteSettingsResp.data.agent_type || '')
      setNoteModel(noteSettingsResp.data.model_value || '')
      setNoteInterval(noteSettingsResp.data.classify_interval_minutes || 5)
      setNotePrompt(noteSettingsResp.data.classify_prompt || '')
      setNoteClassifySessionId(noteSettingsResp.data.classify_db_session_id || 0)
      setNoteMcpToken(noteSettingsResp.data.mcp_token || '')
      // 任务设置
      const ts: TaskSettings = taskSettingsResp.data
      setTaskAutoTag(ts.auto_tag_enabled)
      setTaskAutoTitle(ts.auto_title_enabled)
      setTaskAgent(ts.agent_type || (agentsResp.data.agents || [])[0]?.type || '')
      setTaskTags(ts.tags || [])
      setTaskTagPrompt(ts.tag_prompt || '')
      setTaskTitlePrompt(ts.title_prompt || '')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.loadFailed'))
    } finally { setLoading(false) }
  }

  async function handleSetDefault(agentType: string) {
    setDefaultAgent(agentType)
    try {
      await patchAgentPrefs({ last_agent_type: agentType })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    }
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
      setNoteMcpToken(resp.data.mcp_token || '')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setNoteSettingsSaving(false)
    }
  }

  async function handleGenerateNoteMcpToken() {
    setNoteMcpGenerating(true); setError('')
    try {
      const resp = await generateNoteMCPToken()
      setNoteMcpToken(resp.data.mcp_token || '')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setNoteMcpGenerating(false)
    }
  }

  async function copyText(text: string) {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      setError(t('settings.noteMcpCopyFailed'))
    }
  }

  function handleAddTaskTag() {
    const v = taskTagInput.trim()
    if (!v) return
    if (!taskTags.includes(v)) {
      setTaskTags([...taskTags, v])
    }
    setTaskTagInput('')
  }

  function handleRemoveTaskTag(tag: string) {
    setTaskTags(taskTags.filter((t2) => t2 !== tag))
  }

  async function handleSaveTaskSettings() {
    setTaskSettingsSaving(true); setError(''); setTaskSettingsSaved(false)
    try {
      const resp = await updateTaskSettings({
        auto_tag_enabled: taskAutoTag,
        auto_title_enabled: taskAutoTitle,
        agent_type: taskAgent,
        model_value: '',
        tags: taskTags,
        tag_prompt: taskTagPrompt,
        title_prompt: taskTitlePrompt,
      })
      setTaskTags(resp.data.tags || [])
      setTaskTagPrompt(resp.data.tag_prompt || '')
      setTaskTitlePrompt(resp.data.title_prompt || '')
      setTaskSettingsSaved(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setTaskSettingsSaving(false)
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
    <AppLayout sidebarProps={{ sessions, workspaceId }}>
      <div className={styles.main}>
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <SidebarToggleButton />
            <h1 className={styles.title}>{t('settings.title')}</h1>
          </div>
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
              <button
                type="button"
                className={`${styles.navItem} ${tab === 'task' ? styles.navItemActive : ''}`}
                onClick={() => setTab('task')}
              >
                {t('settings.tabTask')}
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
                          onClick={async () => {
                            setDefaultAgent('')
                            try { await patchAgentPrefs({ last_agent_type: '' }) }
                            catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
                          }}
                        >{t('common.cancel')}</button>
                      )}
                    </div>
                  </div>
                  <div className={styles.configList}>
                    <h2 className={styles.sectionTitle}>{t('settings.agentList')}（{configs.length}）</h2>
                    {configs.length > 0 && (
                      <input
                        type="search"
                        className={styles.configSearch}
                        value={configSearch}
                        onChange={(e) => setConfigSearch(e.target.value)}
                        placeholder={t('settings.searchAgent')}
                      />
                    )}
                    {configs.length === 0 ? (
                      <p className={styles.empty}>{t('settings.noAgents')}</p>
                    ) : (
                      configs
                        .filter((cfg) => {
                          const q = configSearch.trim().toLowerCase()
                          if (!q) return true
                          return cfg.display_name.toLowerCase().includes(q)
                            || cfg.type.toLowerCase().includes(q)
                            || (cfg.description || '').toLowerCase().includes(q)
                        })
                        .map((cfg) => (
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

                    <label className={styles.label}>{t('settings.noteMcpTitle')}</label>
                    <p className={styles.sectionHint}>{t('settings.noteMcpHint')}</p>
                    {!noteMcpToken ? (
                      <button
                        type="button"
                        className={styles.secondaryBtn}
                        disabled={noteMcpGenerating}
                        onClick={handleGenerateNoteMcpToken}
                      >
                        {noteMcpGenerating ? t('common.loading') : t('settings.noteMcpGenerate')}
                      </button>
                    ) : (
                      <>
                        <label className={styles.label}>{t('settings.noteMcpEndpoint')}</label>
                        <div className={styles.inlineRow}>
                          <input
                            className={styles.input}
                            type="text"
                            readOnly
                            value={noteMcpEndpoint}
                          />
                          <button
                            type="button"
                            className={styles.secondaryBtn}
                            onClick={() => copyText(noteMcpEndpoint)}
                          >
                            {t('settings.noteMcpCopy')}
                          </button>
                        </div>
                        <label className={styles.label}>{t('settings.noteMcpToken')}</label>
                        <div className={styles.inlineRow}>
                          <input className={styles.input} type="text" readOnly value={noteMcpToken} />
                          <button
                            type="button"
                            className={styles.secondaryBtn}
                            onClick={() => copyText(noteMcpToken)}
                          >
                            {t('settings.noteMcpCopy')}
                          </button>
                        </div>
                        <label className={styles.label}>{t('settings.noteMcpConfig')}</label>
                        <textarea
                          className={styles.textarea}
                          rows={8}
                          readOnly
                          value={noteMcpConfig}
                        />
                        <button
                          type="button"
                          className={styles.secondaryBtn}
                          onClick={() => copyText(noteMcpConfig)}
                        >
                          {t('settings.noteMcpCopyConfig')}
                        </button>
                        <p className={styles.sectionHint}>{t('settings.noteMcpAuthHint')}</p>
                      </>
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

              {tab === 'task' && (
                <>
                  <p className={styles.hint}>{t('settings.taskHint')}</p>

                  <div className={styles.defaultSection}>
                    {/* 功能开关 */}
                    <label className={styles.label}>
                      <input
                        type="checkbox"
                        checked={taskAutoTag}
                        onChange={(e) => setTaskAutoTag(e.target.checked)}
                        style={{ marginRight: 8, verticalAlign: 'middle' }}
                      />
                      {t('settings.autoTag')}
                    </label>
                    <p className={styles.sectionHint}>{t('settings.autoTagHint')}</p>

                    <label className={styles.label}>
                      <input
                        type="checkbox"
                        checked={taskAutoTitle}
                        onChange={(e) => setTaskAutoTitle(e.target.checked)}
                        style={{ marginRight: 8, verticalAlign: 'middle' }}
                      />
                      {t('settings.autoTitle')}
                    </label>
                    <p className={styles.sectionHint}>{t('settings.autoTitleHint')}</p>

                    {/* 执行 Agent 选择 */}
                    <label className={styles.label}>{t('settings.taskAgent')}</label>
                    <select className={styles.input} value={taskAgent}
                      onChange={(e) => setTaskAgent(e.target.value)}
                    >
                      <option value="">{t('common.no')}</option>
                      {agents.map((a) => (
                        <option key={a.type} value={a.type}>{a.display_name}（{a.type}）</option>
                      ))}
                    </select>

                    {/* 预定义标签管理 */}
                    <label className={styles.label}>{t('settings.predefinedTags')}</label>
                    <p className={styles.sectionHint}>{t('settings.predefinedTagsHint')}</p>
                    <div className={styles.inlineRow}>
                      <input className={styles.input} type="text" value={taskTagInput}
                        onChange={(e) => setTaskTagInput(e.target.value)}
                        placeholder={t('settings.tagPlaceholder')}
                        onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); handleAddTaskTag() } }}
                      />
                      <button type="button" className={styles.secondaryBtn} onClick={handleAddTaskTag}>
                        {t('settings.addTag')}
                      </button>
                    </div>
                    <div className={styles.tagList}>
                      {taskTags.map((tag) => (
                        <span key={tag} className={styles.tagChip}>
                          {tag}
                          <button type="button" className={styles.tagRemove}
                            onClick={() => handleRemoveTaskTag(tag)}
                          >×</button>
                        </span>
                      ))}
                    </div>

                    {/* 高级：自定义提示词 */}
                    <details className={styles.advancedSection}>
                      <summary className={styles.advancedSummary}>{t('settings.taskAdvanced')}</summary>
                      <label className={styles.label}>{t('settings.tagPrompt')}</label>
                      <p className={styles.sectionHint}>{t('settings.tagPromptHint')}</p>
                      <textarea className={styles.textarea} rows={6}
                        value={taskTagPrompt}
                        onChange={(e) => setTaskTagPrompt(e.target.value)}
                      />
                      <label className={styles.label}>{t('settings.titlePrompt')}</label>
                      <p className={styles.sectionHint}>{t('settings.titlePromptHint')}</p>
                      <textarea className={styles.textarea} rows={6}
                        value={taskTitlePrompt}
                        onChange={(e) => setTaskTitlePrompt(e.target.value)}
                      />
                    </details>

                    <button type="button" className={styles.saveNoteBtn}
                      disabled={taskSettingsSaving}
                      onClick={handleSaveTaskSettings}
                    >
                      {taskSettingsSaving ? t('common.saving') : t('common.save')}
                    </button>
                    {taskSettingsSaved && (
                      <span className={styles.savedHint}>{t('settings.taskSettingsSaved')}</span>
                    )}
                  </div>
                </>
              )}

              <button className={styles.backBtn} type="button" onClick={() => navigate(tasksUrl(workspaceId))}>{t('common.back')}</button>
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
    </AppLayout>
  )
}
