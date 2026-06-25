import { useState, useEffect, type FormEvent } from 'react'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgents, getAgentModels, probeAgentConfigs } from '../api/agents'
import {
  listScheduledTasks,
  createScheduledTask,
  updateScheduledTask,
  deleteScheduledTask,
  runScheduledTask,
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

// cron 预设
const CRON_PRESETS: { label: string; value: string }[] = [
  { label: '每 5 分钟', value: '*/5 * * * *' },
  { label: '每小时', value: '0 * * * *' },
  { label: '每天 0 点', value: '0 0 * * *' },
  { label: '每天 9 点', value: '0 9 * * *' },
  { label: '每周一 9 点', value: '0 9 * * 1' },
  { label: '自定义', value: '' },
]

const CUSTOM = '自定义'

const taskStatusLabels: Record<string, string> = {
  success: '成功',
  running: '执行中',
  failed: '失败',
  skipped: '跳过',
  '': '未执行',
}

interface FormState {
  name: string
  agent_type: string
  cwd: string
  prompt: string
  cron_expr: string
  enabled: boolean
  preset: string
  model_value: string
  timeout_minutes: number
}

const emptyForm: FormState = {
  name: '',
  agent_type: '',
  cwd: '',
  prompt: '',
  cron_expr: '*/5 * * * *',
  enabled: true,
  preset: '每 5 分钟',
  model_value: '',
  timeout_minutes: 5,
}

export default function ScheduledTasksPage() {
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
  const [form, setForm] = useState<FormState>(emptyForm)
  const [showDirPicker, setShowDirPicker] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!user) return
    loadData()
  }, [user])

  // agent_type 变化时获取可用模型列表
  useEffect(() => {
    if (!form.agent_type) {
      setModelOptions([])
      return
    }
    let alive = true
    getAgentModels(form.agent_type)
      .then((r) => {
        if (alive) setModelOptions(r.data.model_options || [])
      })
      .catch(() => {
        if (alive) setModelOptions([])
      })
    return () => {
      alive = false
    }
  }, [form.agent_type])

  // 编辑任务时，若缓存无模型列表则自动 probe 获取
  useEffect(() => {
    if (!showForm || !form.agent_type || modelOptions.length > 0 || probing) return
    let alive = true
    setProbing(true)
    probeAgentConfigs(form.agent_type)
      .then((r) => {
        if (!alive) return
        const opts = r.data.config_options || []
        const modelOpt = opts.find((o) => o.category === 'model' && o.type === 'select')
        if (modelOpt) {
          setModelOptions([
            {
              id: modelOpt.id,
              name: modelOpt.name,
              current_value: modelOpt.current_value,
              options: modelOpt.options,
            },
          ])
        }
      })
      .catch(() => {})
      .finally(() => {
        if (alive) setProbing(false)
      })
    return () => {
      alive = false
    }
  }, [showForm, form.agent_type, modelOptions.length])

  async function loadData() {
    setLoading(true)
    setError('')
    try {
      const [agentsResp, tasksResp, sessionsResp] = await Promise.all([
        listAgents(),
        listScheduledTasks(),
        listSessions(),
      ])
      setAgents(agentsResp.data.agents || [])
      setTasks(tasksResp.data.tasks || [])
      setSessions(sessionsResp.data.sessions || [])
      if (agentsResp.data.agents?.length > 0 && !form.agent_type) {
        setForm((prev) => ({ ...prev, agent_type: agentsResp.data.agents[0].type }))
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载数据失败')
    } finally {
      setLoading(false)
    }
  }

  function openCreate() {
    setEditingId(null)
    setForm({ ...emptyForm, agent_type: agents[0]?.type || '' })
    setShowForm(true)
  }

  function openEdit(task: ScheduledTask) {
    const presetMatch = CRON_PRESETS.find((p) => p.value === task.cron_expr)
    setEditingId(task.id)
    setForm({
      name: task.name,
      agent_type: task.agent_type,
      cwd: task.cwd,
      prompt: task.prompt,
      cron_expr: task.cron_expr,
      enabled: task.enabled,
      preset: presetMatch ? presetMatch.label : CUSTOM,
      model_value: task.model_value || '',
      timeout_minutes: task.timeout_minutes || 5,
    })
    setShowForm(true)
  }

  function handlePresetChange(label: string) {
    const preset = CRON_PRESETS.find((p) => p.label === label)
    if (preset && label !== CUSTOM) {
      setForm((prev) => ({ ...prev, preset: label, cron_expr: preset.value }))
    } else {
      setForm((prev) => ({ ...prev, preset: CUSTOM }))
    }
  }

  // 获取配置：建立临时会话探测该 agent 的 config options（含模型），随后删除临时会话。
  async function handleProbe() {
    if (!form.agent_type || probing) return
    setProbing(true)
    setError('')
    try {
      const resp = await probeAgentConfigs(form.agent_type)
      const opts = resp.data.config_options || []
      // 提取 model 类别的 config option 填充模型选择
      const modelOpt = opts.find((o) => o.category === 'model' && o.type === 'select')
      if (modelOpt) {
        setModelOptions([
          {
            id: modelOpt.id,
            name: modelOpt.name,
            current_value: modelOpt.current_value,
            options: modelOpt.options,
          },
        ])
      } else {
        setModelOptions([])
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '获取配置失败')
    } finally {
      setProbing(false)
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form.agent_type || !form.prompt || !form.cron_expr) {
      setError('请填写所有必填字段')
      return
    }
    setSaving(true)
    setError('')
    try {
      if (editingId) {
        await updateScheduledTask(editingId, {
          name: form.name,
          agent_type: form.agent_type,
          cwd: form.cwd,
          prompt: form.prompt,
          cron_expr: form.cron_expr,
          enabled: form.enabled,
          model_value: form.model_value,
          timeout_minutes: form.timeout_minutes,
        })
      } else {
        await createScheduledTask({
          name: form.name,
          agent_type: form.agent_type,
          cwd: form.cwd,
          prompt: form.prompt,
          cron_expr: form.cron_expr,
          enabled: form.enabled,
          model_value: form.model_value,
          timeout_minutes: form.timeout_minutes,
        })
      }
      setShowForm(false)
      setEditingId(null)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(task: ScheduledTask) {
    if (!window.confirm(`确定删除定时任务「${task.name}」？将同时删除其关联会话及全部消息，不可恢复。`)) return
    setError('')
    try {
      await deleteScheduledTask(task.id)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  async function handleRun(task: ScheduledTask) {
    setError('')
    try {
      await runScheduledTask(task.id)
      // 触发后稍等再刷新以获取最新状态
      setTimeout(loadData, 500)
    } catch (err) {
      setError(err instanceof Error ? err.message : '触发失败')
    }
  }

  async function handleToggleEnabled(task: ScheduledTask) {
    setError('')
    try {
      await updateScheduledTask(task.id, { enabled: !task.enabled })
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    }
  }

  if (authLoading) return <LoadingSpinner text="验证登录状态..." />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <SessionSidebar sessions={sessions} />

      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>定时任务配置</h1>
          <UserMenu />
        </div>

        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        {loading ? (
          <LoadingSpinner />
        ) : (
          <div className={styles.content}>
            <button className={styles.createBtn} onClick={openCreate} type="button">
              + 新建定时任务
            </button>

            {showForm && (
              <form className={styles.form} onSubmit={handleSubmit}>
                <div className={styles.field}>
                  <label className={styles.label}>任务名称</label>
                  <input
                    className={styles.input}
                    type="text"
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                    placeholder="如：每日代码检查"
                    required
                  />
                </div>

                <AgentSelector
                  agents={agents}
                  value={form.agent_type}
                  onChange={(v) => setForm({ ...form, agent_type: v })}
                />

                <div className={styles.field}>
                  <label className={styles.label}>模型（可选，留空使用默认）</label>
                  <div className={styles.cwdRow}>
                    {modelOptions.length > 0 && modelOptions[0].options.length > 0 ? (
                      <select
                        className={styles.input}
                        value={form.model_value}
                        onChange={(e) => setForm({ ...form, model_value: e.target.value })}
                      >
                        <option value="">默认模型</option>
                        {modelOptions[0].options.map((o) => (
                          <option key={o.value} value={o.value}>
                            {o.name !== o.value ? `${o.name} (${o.value})` : o.value}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <input
                        className={styles.input}
                        type="text"
                        value={form.model_value}
                        onChange={(e) => setForm({ ...form, model_value: e.target.value })}
                        placeholder="模型 ID（可选，点击右侧按钮获取列表）"
                      />
                    )}
                    <button
                      type="button"
                      className={styles.browseBtn}
                      onClick={handleProbe}
                      disabled={probing || !form.agent_type}
                      title="建立临时会话获取该 agent 的模型及配置"
                    >
                      {probing ? '获取中...' : '获取配置'}
                    </button>
                  </div>
                  <span className={styles.hint}>
                    {modelOptions.length === 0
                      ? '尚未获取模型列表，可手动输入模型 ID 或点击「获取配置」'
                      : '已获取的可用模型（括号内为模型 ID，供 agent 识别）'}
                  </span>
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>工作目录（可选，留空使用临时目录）</label>
                  <div className={styles.cwdRow}>
                    <input
                      className={styles.input}
                      type="text"
                      value={form.cwd}
                      onChange={(e) => setForm({ ...form, cwd: e.target.value })}
                      placeholder="/path/to/project（留空则自动创建临时目录）"
                    />
                    <button
                      type="button"
                      className={styles.browseBtn}
                      onClick={() => setShowDirPicker(true)}
                    >
                      浏览
                    </button>
                  </div>
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>执行计划</label>
                  <select
                    className={styles.input}
                    value={form.preset}
                    onChange={(e) => handlePresetChange(e.target.value)}
                  >
                    {CRON_PRESETS.map((p) => (
                      <option key={p.label} value={p.label}>
                        {p.label}
                      </option>
                    ))}
                  </select>
                  <input
                    className={styles.input}
                    type="text"
                    value={form.cron_expr}
                    onChange={(e) => setForm({ ...form, cron_expr: e.target.value })}
                    placeholder="标准 5 字段 cron 表达式，如 0 9 * * 1-5"
                    required
                  />
                  <span className={styles.hint}>分 时 日 月 周（标准 cron）</span>
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>超时时间（分钟，默认 5）</label>
                  <input
                    className={styles.input}
                    type="number"
                    min={1}
                    max={1440}
                    value={form.timeout_minutes}
                    onChange={(e) => setForm({ ...form, timeout_minutes: Number(e.target.value) || 5 })}
                    required
                  />
                  <span className={styles.hint}>单次执行超过此时间则标记为失败</span>
                </div>

                <div className={styles.field}>
                  <label className={styles.label}>Prompt（每次执行的固定提示词）</label>
                  <textarea
                    className={styles.textarea}
                    value={form.prompt}
                    onChange={(e) => setForm({ ...form, prompt: e.target.value })}
                    placeholder="如：请检查当前仓库的代码风格并报告问题"
                    rows={4}
                    required
                  />
                </div>

                <label className={styles.checkboxRow}>
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                  />
                  <span>启用调度</span>
                </label>

                <div className={styles.formActions}>
                  <button className={styles.submitBtn} type="submit" disabled={saving}>
                    {saving ? '保存中...' : editingId ? '保存修改' : '创建任务'}
                  </button>
                  <button
                    type="button"
                    className={styles.cancelBtn}
                    onClick={() => {
                      setShowForm(false)
                      setEditingId(null)
                    }}
                  >
                    取消
                  </button>
                </div>
              </form>
            )}

            {showDirPicker && (
              <DirectoryPicker
                initialPath={form.cwd}
                onSelect={(path) => {
                  setForm((prev) => ({ ...prev, cwd: path }))
                  setShowDirPicker(false)
                }}
                onClose={() => setShowDirPicker(false)}
              />
            )}

            <div className={styles.taskList}>
              {tasks.length === 0 ? (
                <p className={styles.empty}>暂无定时任务，点击「新建定时任务」开始</p>
              ) : (
                tasks.map((task) => (
                  <div key={task.id} className={styles.taskCard}>
                    <div className={styles.taskHeader}>
                      <span className={styles.taskName}>{task.name}</span>
                      <span
                        className={`${styles.taskStatus} ${styles[`status_${task.last_status}`] || ''}`}
                      >
                        {taskStatusLabels[task.last_status] || '未执行'}
                      </span>
                    </div>
                    <div className={styles.taskMeta}>
                      <span>Agent: {task.agent_type}</span>
                      <span>Cron: {task.cron_expr}</span>
                      <span>启用: {task.enabled ? '是' : '否'}</span>
                    </div>
                    <p className={styles.taskPrompt}>{task.prompt}</p>
                    <div className={styles.taskFooter}>
                      <span className={styles.taskTime}>
                        {task.last_run_at
                          ? `上次执行: ${new Date(task.last_run_at).toLocaleString('zh-CN')}`
                          : '尚未执行'}
                      </span>
                      <div className={styles.taskActions}>
                        <button type="button" className={styles.runBtn} onClick={() => handleRun(task)}>
                          立即执行
                        </button>
                        <button
                          type="button"
                          className={styles.toggleBtn}
                          onClick={() => handleToggleEnabled(task)}
                        >
                          {task.enabled ? '停用' : '启用'}
                        </button>
                        <button type="button" className={styles.editBtn} onClick={() => openEdit(task)}>
                          编辑
                        </button>
                        <button
                          type="button"
                          className={styles.deleteBtn}
                          onClick={() => handleDelete(task)}
                        >
                          删除
                        </button>
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
