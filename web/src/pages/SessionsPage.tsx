import { useState, useEffect, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listAgents } from '../api/agents'
import { listSessions, createSession } from '../api/sessions'
import type { Agent, Session } from '../types'
import AgentSelector from '../components/AgentSelector'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import SessionSidebar from '../components/SessionSidebar'
import styles from './SessionsPage.module.css'

export default function SessionsPage() {
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()

  const [agents, setAgents] = useState<Agent[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // 新建会话表单状态
  const [showForm, setShowForm] = useState(false)
  const [selectedAgent, setSelectedAgent] = useState('')
  const [cwd, setCwd] = useState('')
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    if (!user) return
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
        setSelectedAgent(agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载数据失败')
    } finally {
      setLoading(false)
    }
  }

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    if (!selectedAgent) return
    setCreating(true)
    setError('')
    try {
      const resp = await createSession(selectedAgent, cwd)
      navigate(`/sessions/${resp.data.id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建会话失败')
    } finally {
      setCreating(false)
    }
  }

  if (authLoading) return <LoadingSpinner text="验证登录状态..." />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <SessionSidebar sessions={sessions} />

      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>会话列表</h1>
          <div className={styles.userInfo}>
            <span>{user.username}</span>
          </div>
        </div>

        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        {loading ? (
          <LoadingSpinner />
        ) : (
          <div className={styles.content}>
            <button
              className={styles.createBtn}
              onClick={() => setShowForm(!showForm)}
              type="button"
            >
              {showForm ? '取消' : '+ 新建会话'}
            </button>

            {showForm && (
              <form className={styles.createForm} onSubmit={handleCreate}>
                <AgentSelector
                  agents={agents}
                  value={selectedAgent}
                  onChange={setSelectedAgent}
                />
                <div className={styles.field}>
                  <label className={styles.label}>工作目录（可选）</label>
                  <input
                    className={styles.input}
                    type="text"
                    value={cwd}
                    onChange={(e) => setCwd(e.target.value)}
                    placeholder="/path/to/project"
                  />
                </div>
                <button className={styles.submitBtn} type="submit" disabled={creating || !selectedAgent}>
                  {creating ? '创建中...' : '创建会话'}
                </button>
              </form>
            )}

            <div className={styles.sessionList}>
              {sessions.length === 0 ? (
                <p className={styles.empty}>暂无会话，点击「新建会话」开始</p>
              ) : (
                sessions.map((session) => (
                  <div
                    key={session.id}
                    className={styles.sessionCard}
                    onClick={() => navigate(`/sessions/${session.id}`)}
                  >
                    <div className={styles.sessionHeader}>
                      <span className={styles.sessionAgent}>{session.agent_type}</span>
                      <span className={`${styles.sessionStatus} ${styles[`status_${session.status}`] || ''}`}>
                        {session.status === 'active' ? '活跃' : session.status === 'closed' ? '已关闭' : '错误'}
                      </span>
                    </div>
                    {session.last_prompt && (
                      <p className={styles.sessionPrompt}>{session.last_prompt}</p>
                    )}
                    <span className={styles.sessionTime}>
                      {new Date(session.created_at).toLocaleString('zh-CN')}
                    </span>
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
