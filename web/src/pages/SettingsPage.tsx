import { useState, useEffect, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgentConfigs, createAgentConfig, updateAgentConfig, deleteAgentConfig } from '../api/agentConfigs'
import { listAgents } from '../api/agents'
import { listSessions } from '../api/sessions'
import type { AgentConfig, Session, Agent } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import UserMenu from '../components/UserMenu'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import styles from './SettingsPage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

const EMPTY_FORM = {
  type: '',
  display_name: '',
  description: '',
  command: '',
  args: '',
  api_key_env: '',
  timeout: '',
  enabled: true,
}

export default function SettingsPage() {
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

  useEffect(() => {
    if (user) loadData()
  }, [user])

  async function loadData() {
    setLoading(true)
    setError('')
    try {
      const [cfgResp, agentsResp, sessResp] = await Promise.all([
        listAgentConfigs(),
        listAgents(),
        listSessions(),
      ])
      setConfigs(cfgResp.data.agent_configs || [])
      setAgents(agentsResp.data.agents || [])
      setSessions(sessResp.data.sessions || [])
      setDefaultAgent(localStorage.getItem(DEFAULT_AGENT_KEY) || '')
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载设置失败')
    } finally {
      setLoading(false)
    }
  }

  function handleSetDefault(agentType: string) {
    localStorage.setItem(DEFAULT_AGENT_KEY, agentType)
    setDefaultAgent(agentType)
  }

  function resetForm() {
    setForm({ ...EMPTY_FORM })
    setEditingId(null)
  }

  function startEdit(cfg: AgentConfig) {
    setEditingId(cfg.id)
    setForm({
      type: cfg.type,
      display_name: cfg.display_name,
      description: cfg.description,
      command: cfg.command,
      args: (cfg.args || []).join('\n'),
      api_key_env: cfg.api_key_env,
      timeout: cfg.timeout,
      enabled: cfg.enabled,
    })
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!form.type || !form.display_name || !form.command) {
      setError('类型、显示名称、命令为必填项')
      return
    }
    setSaving(true)
    setError('')
    const payload = {
      type: form.type.trim(),
      display_name: form.display_name.trim(),
      description: form.description.trim(),
      command: form.command.trim(),
      args: form.args
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean),
      api_key_env: form.api_key_env.trim(),
      timeout: form.timeout.trim(),
      enabled: form.enabled,
    }
    try {
      if (editingId != null) {
        await updateAgentConfig(editingId, payload)
      } else {
        await createAgentConfig(payload)
      }
      resetForm()
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    if (!window.confirm('确定删除该 agent 配置？')) return
    setError('')
    try {
      await deleteAgentConfig(id)
      if (editingId === id) resetForm()
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  if (authLoading) return <LoadingSpinner text="验证登录状态..." />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <SessionSidebar sessions={sessions} />

      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>Agent 设置</h1>
          <UserMenu />
        </div>

        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        {loading ? (
          <LoadingSpinner />
        ) : (
          <div className={styles.content}>
            <p className={styles.hint}>
              在此添加本地支持 ACP 的 agent（如 Claude Code、CodeBuddy、Devin 等）。配置全局共享，所有用户可用。内置 agent 由 config.yaml 管理。
            </p>

            {/* 默认 Agent 选择 */}
            <div className={styles.defaultSection}>
              <label className={styles.label}>默认 Agent（新建会话时自动选中）</label>
              <div className={styles.defaultRow}>
                <select
                  className={styles.input}
                  value={defaultAgent}
                  onChange={(e) => handleSetDefault(e.target.value)}
                >
                  <option value="">未设置</option>
                  {agents.map((a) => (
                    <option key={a.type} value={a.type}>
                      {a.display_name}（{a.type}）
                    </option>
                  ))}
                </select>
                {defaultAgent && (
                  <button
                    type="button"
                    className={styles.clearDefaultBtn}
                    onClick={() => {
                      localStorage.removeItem(DEFAULT_AGENT_KEY)
                      setDefaultAgent('')
                    }}
                  >
                    清除
                  </button>
                )}
              </div>
            </div>

            <form className={styles.form} onSubmit={handleSubmit}>
              <h2 className={styles.formTitle}>{editingId != null ? '编辑 Agent' : '添加 Agent'}</h2>
              <div className={styles.grid}>
                <div className={styles.field}>
                  <label className={styles.label}>类型（唯一标识）*</label>
                  <input
                    className={styles.input}
                    value={form.type}
                    onChange={(e) => setForm({ ...form, type: e.target.value })}
                    placeholder="claude-code / codebuddy / devin"
                  />
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>显示名称 *</label>
                  <input
                    className={styles.input}
                    value={form.display_name}
                    onChange={(e) => setForm({ ...form, display_name: e.target.value })}
                    placeholder="Claude Code"
                  />
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>命令 *</label>
                  <input
                    className={styles.input}
                    value={form.command}
                    onChange={(e) => setForm({ ...form, command: e.target.value })}
                    placeholder="npx / codebuddy / devin"
                  />
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>API Key 环境变量</label>
                  <input
                    className={styles.input}
                    value={form.api_key_env}
                    onChange={(e) => setForm({ ...form, api_key_env: e.target.value })}
                    placeholder="ANTHROPIC_API_KEY"
                  />
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>超时（如 300s / 5m）</label>
                  <input
                    className={styles.input}
                    value={form.timeout}
                    onChange={(e) => setForm({ ...form, timeout: e.target.value })}
                    placeholder="300s"
                  />
                </div>
                <div className={styles.field}>
                  <label className={styles.label}>描述</label>
                  <input
                    className={styles.input}
                    value={form.description}
                    onChange={(e) => setForm({ ...form, description: e.target.value })}
                    placeholder="Anthropic Claude Code 编码 agent"
                  />
                </div>
              </div>
              <div className={styles.field}>
                <label className={styles.label}>参数（每行一个）</label>
                <textarea
                  className={styles.textarea}
                  rows={3}
                  value={form.args}
                  onChange={(e) => setForm({ ...form, args: e.target.value })}
                  placeholder={'-y\n@zed-industries/claude-code-acp@latest'}
                />
              </div>
              <label className={styles.checkbox}>
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                />
                启用
              </label>
              <div className={styles.formActions}>
                <button className={styles.submitBtn} type="submit" disabled={saving}>
                  {saving ? '保存中...' : editingId != null ? '更新' : '添加'}
                </button>
                {editingId != null && (
                  <button type="button" className={styles.cancelBtn} onClick={resetForm}>
                    取消编辑
                  </button>
                )}
              </div>
            </form>

            <div className={styles.configList}>
              <h2 className={styles.sectionTitle}>已配置 Agent（{configs.length}）</h2>
              {configs.length === 0 ? (
                <p className={styles.empty}>暂无自定义 agent，请在上方添加</p>
              ) : (
                configs.map((cfg) => (
                  <div key={cfg.id} className={styles.configCard}>
                    <div className={styles.configHeader}>
                      <span className={styles.configType}>{cfg.display_name}</span>
                      <span
                        className={`${styles.statusBadge} ${cfg.enabled ? styles.enabled : styles.disabled}`}
                      >
                        {cfg.enabled ? '启用' : '禁用'}
                      </span>
                    </div>
                    <div className={styles.configMeta}>
                      <span className={styles.metaItem}>类型: {cfg.type}</span>
                      <span className={styles.metaItem}>命令: {cfg.command}</span>
                      {cfg.api_key_env && <span className={styles.metaItem}>KeyEnv: {cfg.api_key_env}</span>}
                      {cfg.timeout && <span className={styles.metaItem}>超时: {cfg.timeout}</span>}
                    </div>
                    {cfg.description && <p className={styles.configDesc}>{cfg.description}</p>}
                    {cfg.args && cfg.args.length > 0 && (
                      <code className={styles.argsBlock}>{cfg.args.join(' ')}</code>
                    )}
                    <div className={styles.cardActions}>
                      <button type="button" className={styles.editBtn} onClick={() => startEdit(cfg)}>
                        编辑
                      </button>
                      <button type="button" className={styles.deleteBtn} onClick={() => handleDelete(cfg.id)}>
                        删除
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>

            <button className={styles.backBtn} type="button" onClick={() => navigate('/')}>
              返回会话列表
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
