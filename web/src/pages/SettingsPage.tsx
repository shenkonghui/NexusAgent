import { useState, useEffect, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgentConfigs, updateAgentConfig, deleteAgentConfig } from '../api/agentConfigs'
import { listAgents } from '../api/agents'
import { listSessions } from '../api/sessions'
import type { AgentConfig, Session, Agent } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import UserMenu from '../components/UserMenu'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import i18n from '../i18n'
import styles from './SettingsPage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

const EMPTY_FORM = {
  type: '', display_name: '', description: '', command: '', args: '',
  api_key_env: '', timeout: '', enabled: true,
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()

  const [configs, setConfigs] = useState<AgentConfig[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [defaultAgent, setDefaultAgent] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [form, setForm] = useState({ ...EMPTY_FORM })
  const [editingId, setEditingId] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => { if (user) loadData() }, [user])

  async function loadData() {
    setLoading(true); setError('')
    try {
      const [cfgResp, agentsResp, sessResp] = await Promise.all([listAgentConfigs(), listAgents(), listSessions()])
      setConfigs(cfgResp.data.agent_configs || [])
      setAgents(agentsResp.data.agents || [])
      setSessions(sessResp.data.sessions || [])
      setDefaultAgent(localStorage.getItem(DEFAULT_AGENT_KEY) || '')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.loadFailed'))
    } finally { setLoading(false) }
  }

  function handleSetDefault(agentType: string) {
    localStorage.setItem(DEFAULT_AGENT_KEY, agentType); setDefaultAgent(agentType)
  }

  function resetForm() { setForm({ ...EMPTY_FORM }); setEditingId(null) }

  function startEdit(cfg: AgentConfig) {
    setEditingId(cfg.id)
    setForm({
      type: cfg.type, display_name: cfg.display_name, description: cfg.description,
      command: cfg.command, args: (cfg.args || []).join('\n'), api_key_env: cfg.api_key_env,
      timeout: cfg.timeout, enabled: cfg.enabled,
    })
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form.type || !form.display_name || !form.command) {
      setError(t('settings.validationError')); return
    }
    setSaving(true); setError('')
    const payload = {
      type: form.type.trim(), display_name: form.display_name.trim(),
      description: form.description.trim(), command: form.command.trim(),
      args: form.args.split('\n').map((s) => s.trim()).filter(Boolean),
      api_key_env: form.api_key_env.trim(), timeout: form.timeout.trim(), enabled: form.enabled,
    }
    try {
      if (editingId != null) {
        await updateAgentConfig(editingId, payload)
        resetForm()
        await loadData()
      }
    } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
    finally { setSaving(false) }
  }

  async function handleDelete(id: number) {
    if (!window.confirm(t('settings.deleteConfirm'))) return
    setError('')
    try { await deleteAgentConfig(id); if (editingId === id) resetForm(); await loadData() }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
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
          <div className={styles.content}>
            <p className={styles.hint}>{t('settings.hint')}</p>

            {/* 语言切换 */}
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

            {/* 默认 Agent */}
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
                      <span className={styles.enabledTag}>{t('settings.enabled')}</span>
                    ) : (
                      <button type="button" className={styles.enableBtn}
                        onClick={async () => {
                          try { await updateAgentConfig(cfg.id, { ...cfg, enabled: true }); await loadData() }
                          catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
                        }}
                      >Enable</button>
                    )}
                    <button type="button" className={styles.editIconBtn} title={t('common.edit')}
                      onClick={() => startEdit(cfg)}
                    >⋯</button>
                  </div>
                ))
              )}
            </div>

            {editingId != null && (
              <form className={styles.form} onSubmit={handleSubmit}>
                <h2 className={styles.formTitle}>{t('settings.editTitle')}</h2>
                <div className={styles.grid}>
                  <div className={styles.field}>
                    <label className={styles.label}>{t('settings.type')} *</label>
                    <input className={styles.input} value={form.type} disabled />
                  </div>
                  <div className={styles.field}>
                    <label className={styles.label}>{t('settings.displayName')} *</label>
                    <input className={styles.input} value={form.display_name}
                      onChange={(e) => setForm({ ...form, display_name: e.target.value })} placeholder="Claude Code"
                    />
                  </div>
                  <div className={styles.field}>
                    <label className={styles.label}>{t('settings.command')} *</label>
                    <input className={styles.input} value={form.command}
                      onChange={(e) => setForm({ ...form, command: e.target.value })} placeholder="npx / codebuddy"
                    />
                  </div>
                  <div className={styles.field}>
                    <label className={styles.label}>{t('settings.apiKeyEnv')}</label>
                    <input className={styles.input} value={form.api_key_env}
                      onChange={(e) => setForm({ ...form, api_key_env: e.target.value })} placeholder="ANTHROPIC_API_KEY"
                    />
                  </div>
                  <div className={styles.field}>
                    <label className={styles.label}>{t('settings.timeout')}</label>
                    <input className={styles.input} value={form.timeout}
                      onChange={(e) => setForm({ ...form, timeout: e.target.value })} placeholder="300s"
                    />
                  </div>
                  <div className={styles.field}>
                    <label className={styles.label}>{t('settings.description')}</label>
                    <input className={styles.input} value={form.description}
                      onChange={(e) => setForm({ ...form, description: e.target.value })}
                      placeholder="Anthropic Claude Code agent"
                    />
                  </div>
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>{t('settings.args')}</label>
                  <textarea className={styles.textarea} rows={3} value={form.args}
                    onChange={(e) => setForm({ ...form, args: e.target.value })}
                    placeholder={'-y\n@agentclientprotocol/claude-agent-acp@latest'}
                  />
                </div>
                <label className={styles.checkbox}>
                  <input type="checkbox" checked={form.enabled}
                    onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                  /> {t('settings.enabled')}
                </label>
                <div className={styles.formActions}>
                  <button className={styles.submitBtn} type="submit" disabled={saving}>
                    {saving ? t('common.saving') : t('common.save')}
                  </button>
                  <button type="button" className={styles.cancelBtn} onClick={resetForm}>{t('common.cancel')}</button>
                  <button type="button" className={styles.deleteBtn} onClick={() => { handleDelete(editingId) }}>{t('common.delete')}</button>
                </div>
              </form>
            )}
            <button className={styles.backBtn} type="button" onClick={() => navigate('/')}>{t('common.back')}</button>
          </div>
        )}
      </div>
    </div>
  )
}
