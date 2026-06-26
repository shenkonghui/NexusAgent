import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgents } from '../api/agents'
import { listSessions, createSession, deleteSession, updateSessionTitle } from '../api/sessions'
import type { Agent, Session } from '../types'
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
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()

  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // 首页快捷发起会话状态
  const [showDirPicker, setShowDirPicker] = useState(false)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [selectedAgent, setSelectedAgent] = useState('')
  const [cwd, setCwd] = useState('')
  const [creating, setCreating] = useState(false)
  const [defaultAgent, setDefaultAgent] = useState('')

  useEffect(() => {
    if (!user) return
    setDefaultAgent(localStorage.getItem(DEFAULT_AGENT_KEY) || '')
    loadData()
  }, [user])

  async function loadData() {
    setLoading(true)
    setError('')
    try {
      const [agentsResp, sessionsResp] = await Promise.all([
        listAgents(),
        listSessions(),
      ])
      setAgents(agentsResp.data.agents || [])
      setSessions(sessionsResp.data.sessions || [])
      if (agentsResp.data.agents?.length > 0) {
        // 优先使用 localStorage 中的默认 agent，否则取第一个
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a) => a.type)
        if (saved && types.includes(saved)) {
          setSelectedAgent(saved)
        } else {
          setSelectedAgent(agentsResp.data.agents[0].type)
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载数据失败')
    } finally {
      setLoading(false)
    }
  }

  // 首页输入 prompt 后：用默认 agent 创建会话，并携带初始 prompt 跳转到聊天页自动发送
  async function handleQuickSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true)
    setError('')
    try {
      const resp = await createSession(selectedAgent, cwd)
      navigate(`/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建会话失败')
      setCreating(false)
    }
  }

  // 删除会话：彻底移除会话及其消息。active 会话会先释放连接。
  async function handleDelete(session: Session) {
    if (!window.confirm(`确定删除会话「${session.title || session.agent_type}」？此操作不可恢复，将同时删除其全部消息。`)) {
      return
    }
    setError('')
    try {
      await deleteSession(session.id)
      setSessions((prev) => prev.filter((s) => s.id !== session.id))
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除会话失败')
    }
  }

  // 重命名会话
  async function handleRename(id: number, title: string) {
    setError('')
    try {
      const resp = await updateSessionTitle(id, title)
      setSessions((prev) => prev.map((s) => (s.id === id ? resp.data : s)))
    } catch (err) {
      setError(err instanceof Error ? err.message : '修改标题失败')
    }
  }

  if (authLoading) return <LoadingSpinner text="验证登录状态..." />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <div className={styles.sidebarWrap}>
        <SessionSidebar sessions={sessions} onRename={handleRename} />
      </div>

      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>首页</h1>
          <UserMenu />
        </div>

        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        {loading ? (
          <LoadingSpinner />
        ) : (
          <div className={styles.content}>
            {/* 欢迎输入区：使用默认 agent，首次发送时创建会话 */}
            <div className={styles.hero}>
              <h2 className={styles.heroTitle}>开始新对话</h2>
              <p className={styles.heroSubtitle}>
                使用默认 Agent 开始，发送第一条消息后将自动创建会话
              </p>
              <div className={styles.heroAgent}>
                <AgentSelector
                  agents={agents}
                  value={selectedAgent}
                  onChange={setSelectedAgent}
                />
                {defaultAgent && (
                  <p className={styles.defaultHint}>
                    默认 Agent：{defaultAgent}（可在 Agent 设置中修改）
                  </p>
                )}
              </div>
              <div className={styles.heroPrompt}>
                <PromptInput
                  onSend={handleQuickSend}
                  sending={creating}
                  disabled={!selectedAgent || creating}
                  placeholder="输入你想做的事，Enter 发送，Shift+Enter 换行"
                />
              </div>
              {/* 高级选项：工作目录 */}
              <div className={styles.advanced}>
                <button
                  type="button"
                  className={styles.advancedToggle}
                  onClick={() => setShowAdvanced(!showAdvanced)}
                >
                  {showAdvanced ? '▾' : '▸'} 高级选项{cwd ? `（工作目录：${cwd}）` : ''}
                </button>
                {showAdvanced && (
                  <div className={styles.cwdRow}>
                    <input
                      className={styles.input}
                      type="text"
                      value={cwd}
                      onChange={(e) => setCwd(e.target.value)}
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
                )}
              </div>
            </div>

            {showDirPicker && (
              <DirectoryPicker
                initialPath={cwd}
                onSelect={(path) => {
                  setCwd(path)
                  setShowDirPicker(false)
                }}
                onClose={() => setShowDirPicker(false)}
              />
            )}

            {/* 历史会话列表 */}
            <div className={styles.sessionList}>
              <h3 className={styles.listTitle}>历史会话</h3>
              {sessions.length === 0 ? (
                <p className={styles.empty}>暂无会话，在上方输入框开始第一次对话</p>
              ) : (
                sessions.map((session) => (
                  <div
                    key={session.id}
                    className={styles.sessionCard}
                    onClick={() => navigate(`/sessions/${session.id}`)}
                  >
                    <div className={styles.sessionHeader}>
                      <span className={styles.sessionAgent}>{session.title || session.agent_type}</span>
                      <span className={`${styles.sessionStatus} ${styles[`status_${session.status}`] || ''}`}>
                        {session.status === 'active' ? '活跃' : session.status === 'closed' ? '已关闭' : '错误'}
                      </span>
                    </div>
                    {session.last_prompt && (
                      <p className={styles.sessionPrompt}>{session.last_prompt}</p>
                    )}
                    <div className={styles.sessionFooter}>
                      <span className={styles.sessionTime}>
                        {new Date(session.created_at).toLocaleString('zh-CN')}
                      </span>
                      <button
                        type="button"
                        className={styles.deleteBtn}
                        onClick={(e) => {
                          e.stopPropagation()
                          handleDelete(session)
                        }}
                      >
                        删除
                      </button>
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
