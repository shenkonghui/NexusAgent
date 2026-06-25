import { useState, useEffect, type FormEvent } from 'react'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgents } from '../api/agents'
import {
  listScheduledTasks,
  createScheduledTask,
  updateScheduledTask,
  deleteScheduledTask,
  runScheduledTask,
} from '../api/scheduledTasks'
import type { Agent, ScheduledTask } from '../types'
import AgentSelector from '../components/AgentSelector'
import DirectoryPicker from '../components/DirectoryPicker'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import SessionSidebar from '../components/SessionSidebar'
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
}

const emptyForm: FormState = {
  name: '',
  agent_type: '',
  cwd: '',
  prompt: '',
  cron_expr: '*/5 * * * *',
  enabled: true,
  preset: '每 5 分钟',
}

export default function ScheduledTasksPage() {
  const { user, loading: authLoading } = useRequireAuth()

  const [agents, setAgents] = useState<Agent[]>([])
  const [tasks, setTasks] = useState<ScheduledTask[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
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

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form.agent_type || !form.cwd || !form.prompt || !form.cron_expr) {
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
        })
      } else {
        await createScheduledTask({
          name: form.name,
          agent_type: form.agent_type,
          cwd: form.cwd,
          prompt: form.prompt,
          cron_expr: form.cron_expr,
          enabled: form.enabled,
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
          <div className={styles.userInfo}>
            <span>{user.username}</span>
          </div>
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
                  <label className={styles.label}>工作目录（必填）</label>
                  <div className={styles.cwdRow}>
                    <input
                      className={styles.input}
                      type="text"
                      value={form.cwd}
                      onChange={(e) => setForm({ ...form, cwd: e.target.value })}
                      placeholder="/path/to/project"
                      required
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
