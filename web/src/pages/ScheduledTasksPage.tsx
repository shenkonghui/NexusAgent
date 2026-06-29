import { useState, useEffect, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgents, getAgentModels, probeAgentConfigs } from '../api/agents'
import {
  listScheduledTasks, createScheduledTask, updateScheduledTask, deleteScheduledTask, runScheduledTask,
} from '../api/scheduledTasks'
import type { Agent, ScheduledTask, ModelOption } from '../types'
import AgentSelector from '../components/AgentSelector'
import DirectoryPicker from '../components/DirectoryPicker'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import SessionSidebar from '../components/SessionSidebar'
import UserMenu from '../components/UserMenu'
import { listSessions } from '../api/sessions'
import type { Session } from '../types'
import styles from './ScheduledTasksPage.module.css'

interface FormState {
  name: string; agent_type: string; cwd: string; prompt: string
  cron_expr: string; enabled: boolean; preset: string; model_value: string; timeout_minutes: number
}

export default function ScheduledTasksPage() {
  const { t, i18n } = useTranslation()
  const { user, loading: authLoading } = useRequireAuth()

  const [agents, setAgents] = useState<Agent[]>([])
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [modelOptions, setModelOptions] = useState<ModelOption[]>([])
  const [probing, setProbing] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormState>({
    name: '', agent_type: '', cwd: '', prompt: '', cron_expr: '*/5 * * * *',
    enabled: true, preset: '每 5 分钟', model_value: '', timeout_minutes: 5,
  })
  const [showDirPicker, setShowDirPicker] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => { if (!user) return; loadData() }, [user])

  useEffect(() => {
    if (!form.agent_type) { setModelOptions([]); return }
    let alive = true
    getAgentModels(form.agent_type).then((r) => { if (alive) setModelOptions(r.data.model_options || []) }).catch(() => { if (alive) setModelOptions([]) })
    return () => { alive = false }
  }, [form.agent_type])

  useEffect(() => {
    if (!showForm || !form.agent_type || modelOptions.length > 0 || probing) return
    let alive = true; setProbing(true)
    probeAgentConfigs(form.agent_type)
      .then((r) => {
        if (!alive) return; const opts = r.data.config_options || []
        const modelOpt = opts.find((o) => o.category === 'model' && o.type === 'select')
        if (modelOpt) setModelOptions([{ id: modelOpt.id, name: modelOpt.name, current_value: modelOpt.current_value, options: modelOpt.options }])
      })
      .catch(() => {})
      .finally(() => { if (alive) setProbing(false) })
    return () => { alive = false }
  }, [showForm, form.agent_type, modelOptions.length])

  async function loadData() {
    setLoading(true); setError('')
    try {
      const [agentsResp, tasksResp, sessionsResp] = await Promise.all([listAgents(), listScheduledTasks(), listSessions()])
      setAgents(agentsResp.data.agents || [])
      setTasks(tasksResp.data.tasks || [])
      setSessions(sessionsResp.data.sessions || [])
      if (agentsResp.data.agents?.length > 0 && !form.agent_type) setForm((prev) => ({ ...prev, agent_type: agentsResp.data.agents[0].type }))
    } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
    finally { setLoading(false) }
  }

  function openCreate() { setEditingId(null); setForm({ ...form, name: '', agent_type: agents[0]?.type || '', cwd: '', prompt: '', cron_expr: '*/5 * * * *', enabled: true, preset: '每 5 分钟', model_value: '', timeout_minutes: 5 }); setShowForm(true) }

  function openEdit(task: ScheduledTask) {
    const customLabel = t('scheduledTask.custom')
    const presets = [
      { label: t('scheduledTask.every5min'), value: '*/5 * * * *' },
      { label: t('scheduledTask.everyHour'), value: '0 * * * *' },
      { label: t('scheduledTask.daily0'), value: '0 0 * * *' },
      { label: t('scheduledTask.daily9'), value: '0 9 * * *' },
      { label: t('scheduledTask.weeklyMon9'), value: '0 9 * * 1' },
      { label: customLabel, value: '' },
    ]
    const presetMatch = presets.find((p) => p.value === task.cron_expr)
    setEditingId(task.id)
    setForm({ name: task.name, agent_type: task.agent_type, cwd: task.cwd, prompt: task.prompt, cron_expr: task.cron_expr, enabled: task.enabled, preset: presetMatch ? presetMatch.label : customLabel, model_value: task.model_value || '', timeout_minutes: task.timeout_minutes || 5 })
    setShowForm(true)
  }

  function handlePresetChange(label: string) {
    const customLabel = t('scheduledTask.custom')
    const presets = [
      { label: t('scheduledTask.every5min'), value: '*/5 * * * *' },
      { label: t('scheduledTask.everyHour'), value: '0 * * * *' },
      { label: t('scheduledTask.daily0'), value: '0 0 * * *' },
      { label: t('scheduledTask.daily9'), value: '0 9 * * *' },
      { label: t('scheduledTask.weeklyMon9'), value: '0 9 * * 1' },
      { label: customLabel, value: '' },
    ]
    const preset = presets.find((p) => p.label === label)
    if (preset && label !== customLabel) setForm((prev) => ({ ...prev, preset: label, cron_expr: preset.value }))
    else setForm((prev) => ({ ...prev, preset: customLabel }))
  }

  async function handleProbe() {
    if (!form.agent_type || probing) return; setProbing(true); setError('')
    try {
      const resp = await probeAgentConfigs(form.agent_type); const opts = resp.data.config_options || []
      const modelOpt = opts.find((o) => o.category === 'model' && o.type === 'select')
      if (modelOpt) setModelOptions([{ id: modelOpt.id, name: modelOpt.name, current_value: modelOpt.current_value, options: modelOpt.options }])
      else setModelOptions([])
    } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
    finally { setProbing(false) }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form.agent_type || !form.prompt || !form.cron_expr) { setError(t('scheduledTask.validationError')); return }
    setSaving(true); setError('')
    const payload = { name: form.name, agent_type: form.agent_type, cwd: form.cwd, prompt: form.prompt, cron_expr: form.cron_expr, enabled: form.enabled, model_value: form.model_value, timeout_minutes: form.timeout_minutes }
    try {
      if (editingId) await updateScheduledTask(editingId, payload)
      else await createScheduledTask(payload)
      setShowForm(false); setEditingId(null); await loadData()
    } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
    finally { setSaving(false) }
  }

  async function handleDelete(task: ScheduledTask) {
    if (!window.confirm(t('scheduledTask.deleteConfirm'))) return; setError('')
    try { await deleteScheduledTask(task.id); await loadData() }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  async function handleRun(task: ScheduledTask) {
    setError('')
    try { await runScheduledTask(task.id); setTimeout(loadData, 500) }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  async function handleToggleEnabled(task: ScheduledTask) {
    setError('')
    try { await updateScheduledTask(task.id, { enabled: !task.enabled }); await loadData() }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null

  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'

  const taskStatusLabels: Record<string, string> = {
    success: t('scheduledTask.statusSuccess'),
    running: t('scheduledTask.statusRunning'),
    failed: t('scheduledTask.statusFailed'),
    skipped: t('scheduledTask.statusCancelled'),
    '': t('scheduledTask.noTasks'),
  }

  return (
    <div className={styles.layout}>
      <div className={styles.sidebarWrap}><SessionSidebar sessions={sessions} /></div>
      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>{t('scheduledTask.title')}</h1>
          <UserMenu />
        </div>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}
        {loading ? <LoadingSpinner /> : (
          <div className={styles.content}>
            <button className={styles.createBtn} onClick={openCreate} type="button">+ {t('scheduledTask.newTask')}</button>

            {showForm && (
              <form className={styles.form} onSubmit={handleSubmit}>
                <div className={styles.field}>
                  <label className={styles.label}>{t('scheduledTask.name')}</label>
                  <input className={styles.input} type="text" value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder={t('scheduledTask.namePlaceholder')} required
                  />
                </div>
                <AgentSelector agents={agents} value={form.agent_type} onChange={(v) => setForm({ ...form, agent_type: v })} />
                <div className={styles.field}>
                  <label className={styles.label}>{t('scheduledTask.modelValue')}</label>
                  <div className={styles.cwdRow}>
                    {modelOptions.length > 0 && modelOptions[0].options.length > 0 ? (
                      <select className={styles.input} value={form.model_value}
                        onChange={(e) => setForm({ ...form, model_value: e.target.value })}
                      >
                        <option value="">{t('scheduledTask.defaultModel')}</option>
                        {modelOptions[0].options.map((o) => (
                          <option key={o.value} value={o.value}>{o.name !== o.value ? `${o.name} (${o.value})` : o.value}</option>
                        ))}
                      </select>
                    ) : (
                      <input className={styles.input} type="text" value={form.model_value}
                        onChange={(e) => setForm({ ...form, model_value: e.target.value })}
                        placeholder={t('scheduledTask.modelValuePlaceholder')}
                      />
                    )}
                    <button type="button" className={styles.browseBtn}
                      onClick={handleProbe} disabled={probing || !form.agent_type}
                      title={t('scheduledTask.probeTitle')}
                    >{probing ? t('common.loading') : t('scheduledTask.probeConfig')}</button>
                  </div>
                  <span className={styles.hint}>{modelOptions.length === 0 ? t('scheduledTask.probeHint') : t('scheduledTask.probeDone')}</span>
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>{t('scheduledTask.cwd')}</label>
                  <div className={styles.cwdRow}>
                    <input className={styles.input} type="text" value={form.cwd}
                      onChange={(e) => setForm({ ...form, cwd: e.target.value })} placeholder={t('scheduledTask.cwdPlaceholder')}
                    />
                    <button type="button" className={styles.browseBtn} onClick={() => setShowDirPicker(true)}>{t('common.search')}</button>
                  </div>
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>{t('scheduledTask.schedule')}</label>
                  <select className={styles.input} value={form.preset} onChange={(e) => handlePresetChange(e.target.value)}>
                    {[
                      { label: t('scheduledTask.every5min'), value: '*/5 * * * *' },
                      { label: t('scheduledTask.everyHour'), value: '0 * * * *' },
                      { label: t('scheduledTask.daily0'), value: '0 0 * * *' },
                      { label: t('scheduledTask.daily9'), value: '0 9 * * *' },
                      { label: t('scheduledTask.weeklyMon9'), value: '0 9 * * 1' },
                      { label: t('scheduledTask.custom'), value: '' },
                    ].map((p) => (<option key={p.label} value={p.label}>{p.label}</option>))}
                  </select>
                  <input className={styles.input} type="text" value={form.cron_expr}
                    onChange={(e) => setForm({ ...form, cron_expr: e.target.value })}
                    placeholder={t('scheduledTask.cronExprPlaceholder')} required
                  />
                  <span className={styles.hint}>{t('scheduledTask.cronHint')}</span>
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>{t('scheduledTask.timeout')}</label>
                  <input className={styles.input} type="number" min={1} max={1440}
                    value={form.timeout_minutes}
                    onChange={(e) => setForm({ ...form, timeout_minutes: Number(e.target.value) || 5 })} required
                  />
                  <span className={styles.hint}>{t('scheduledTask.timeoutHint')}</span>
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>{t('scheduledTask.prompt')}</label>
                  <textarea className={styles.textarea} value={form.prompt}
                    onChange={(e) => setForm({ ...form, prompt: e.target.value })}
                    placeholder={t('scheduledTask.promptPlaceholder')} rows={4} required
                  />
                </div>
                <label className={styles.checkboxRow}>
                  <input type="checkbox" checked={form.enabled}
                    onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                  /> <span>{t('scheduledTask.enabled')}</span>
                </label>
                <div className={styles.formActions}>
                  <button className={styles.submitBtn} type="submit" disabled={saving}>
                    {saving ? t('common.saving') : editingId ? t('common.save') : t('common.create')}
                  </button>
                  <button type="button" className={styles.cancelBtn} onClick={() => { setShowForm(false); setEditingId(null) }}>{t('common.cancel')}</button>
                </div>
              </form>
            )}

            {showDirPicker && (
              <DirectoryPicker initialPath={form.cwd}
                onSelect={(path) => { setForm((prev) => ({ ...prev, cwd: path })); setShowDirPicker(false) }}
                onClose={() => setShowDirPicker(false)}
              />
            )}

            <div className={styles.taskList}>
              {tasks.length === 0 ? (
                <p className={styles.empty}>{t('scheduledTask.noTasks')}</p>
              ) : (
                tasks.map((task) => (
                  <div key={task.id} className={styles.taskCard}>
                    <div className={styles.taskHeader}>
                      <span className={styles.taskName}>{task.name}</span>
                      <span className={`${styles.taskStatus} ${styles[`status_${task.last_status}`] || ''}`}>
                        {taskStatusLabels[task.last_status] || t('scheduledTask.noTasks')}
                      </span>
                    </div>
                    <div className={styles.taskMeta}>
                      <span>{t('scheduledTask.agentType')}: {task.agent_type}</span>
                      <span>Cron: {task.cron_expr}</span>
                      <span>{t('scheduledTask.enabled')}: {task.enabled ? t('common.yes') : t('common.no')}</span>
                    </div>
                    <p className={styles.taskPrompt}>{task.prompt}</p>
                    <div className={styles.taskFooter}>
                      <span className={styles.taskTime}>
                        {task.last_run_at
                          ? `${t('scheduledTask.lastRun')}: ${new Date(task.last_run_at).toLocaleString(locale)}`
                          : t('scheduledTask.noTasks')}
                      </span>
                      <div className={styles.taskActions}>
                        <button type="button" className={styles.runBtn} onClick={() => handleRun(task)}>{t('scheduledTask.runNow')}</button>
                        <button type="button" className={styles.toggleBtn} onClick={() => handleToggleEnabled(task)}>
                          {task.enabled ? t('scheduledTask.disabled') : t('scheduledTask.enabled')}
                        </button>
                        <button type="button" className={styles.editBtn} onClick={() => openEdit(task)}>{t('common.edit')}</button>
                        <button type="button" className={styles.deleteBtn} onClick={() => handleDelete(task)}>{t('common.delete')}</button>
                      </div>
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
