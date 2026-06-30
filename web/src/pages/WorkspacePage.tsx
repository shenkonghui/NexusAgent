import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getWorkspace, listWorkspaces, deleteWorkspace, updateWorkspace, saveWorkspace } from '../api/workspaces'
import { createSession } from '../api/sessions'
import { listAgents, probeAgentConfigs } from '../api/agents'
import type { Workspace, Session, Agent, ConfigOption } from '../types'
import WorkspaceSidebar from '../components/WorkspaceSidebar'
import PromptInput from '../components/PromptInput'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import UserMenu from '../components/UserMenu'
import styles from './WorkspacePage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

export default function WorkspacePage() {
  const { wid } = useParams<{ wid: string }>()
  const workspaceId = Number(wid)
  const navigate = useNavigate()
  const { user, loading: authLoading } = useRequireAuth()

  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [sessions, setSessions] = useState<Session[]>([])
  const [allWorkspaces, setAllWorkspaces] = useState<(Workspace & { session_count?: number })[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgent, setSelectedAgent] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [probeConfigs, setProbeConfigs] = useState<ConfigOption[]>([])
  const [probing, setProbing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  const loadData = useCallback(async () => {
    if (!workspaceId) return
    setLoading(true)
    try {
      const [wsResp, allWsResp, agentsResp] = await Promise.all([
        getWorkspace(workspaceId), listWorkspaces(), listAgents(),
      ])
      setWorkspace(wsResp.data.workspace)
      setSessions(wsResp.data.sessions || [])
      setAllWorkspaces(allWsResp.data.workspaces || [])
      setAgents(agentsResp.data.agents || [])
      if (agentsResp.data.agents?.length > 0) {
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a: Agent) => a.type)
        setSelectedAgent(saved && types.includes(saved) ? saved : agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally { setLoading(false) }
  }, [workspaceId])

  useEffect(() => { if (user) loadData() }, [user, loadData])

  useEffect(() => {
    if (!selectedAgent) return
    let alive = true
    setProbing(true)
    probeAgentConfigs(selectedAgent)
      .then(r => {
        if (!alive) return
        const opts = r.data.config_options || []
        setProbeConfigs(opts)
        const modelOpt = opts.find(o => o.category === 'model')
        setSelectedModel(modelOpt?.current_value || modelOpt?.options[0]?.value || '')
      })
      .catch(() => { if (alive) { setProbeConfigs([]); setSelectedModel('') } })
      .finally(() => { if (alive) setProbing(false) })
    return () => { alive = false }
  }, [selectedAgent])

  async function handleSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, workspaceId, selectedModel || undefined)
      localStorage.setItem(DEFAULT_AGENT_KEY, selectedAgent)
      navigate(`/workspaces/${workspaceId}/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败')
      setCreating(false)
    }
  }

  async function handleDeleteWorkspace(id: number) {
    try { await deleteWorkspace(id); navigate('/') }
    catch (err) { setError(err instanceof Error ? err.message : '删除失败') }
  }

  async function handleRenameWorkspace(id: number, name: string) {
    try { await updateWorkspace(id, name); loadData() }
    catch (err) { setError(err instanceof Error ? err.message : '重命名失败') }
  }

  async function handleSaveWorkspace(id: number) {
    const ws = allWorkspaces.find(w => w.id === id)
    if (ws) {
      try { await saveWorkspace(id, ws.name, ws.cwd); loadData() }
      catch (err) { setError(err instanceof Error ? err.message : '保存失败') }
    }
  }

  if (authLoading) return <LoadingSpinner />
  if (!user) return null
  if (loading) return <LoadingSpinner />

  return (
    <div className={styles.layout}>
      <WorkspaceSidebar
        workspaces={allWorkspaces}
        currentId={workspaceId}
        onDelete={handleDeleteWorkspace}
        onRename={handleRenameWorkspace}
        onSave={handleSaveWorkspace}
        onCreateClick={() => navigate('/')}
      />
      <div className={styles.main}>
        <div className={styles.header}>
          <div className={styles.workspaceInfo}>
            <span className={styles.wsName}>{workspace?.name}</span>
            <span className={styles.wsCwd}>{workspace?.cwd}</span>
          </div>
          <UserMenu />
        </div>
        <div className={styles.configBar}>
          <select className={styles.configSelect} value={selectedAgent}
            onChange={e => setSelectedAgent(e.target.value)} disabled={creating}>
            {agents.map(a => <option key={a.type} value={a.type}>{a.display_name}</option>)}
          </select>
          {probeConfigs.filter(o => o.type === 'select' && o.category === 'model').map(opt => (
            <input key={opt.id} className={styles.configInput}
              value={selectedModel}
              onChange={e => setSelectedModel(e.target.value)}
              disabled={probing || creating}
              placeholder="模型"
            />
          ))}
        </div>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        <div className={styles.sessionsList}>
          <h3>会话历史</h3>
          {sessions.length === 0 && <p className={styles.empty}>暂无会话，输入 prompt 开始</p>}
          {sessions.map(s => (
            <div key={s.id} className={styles.sessionItem}
              onClick={() => navigate(`/workspaces/${workspaceId}/sessions/${s.id}`)}>
              <span className={styles.sessionTitle}>{s.title || '新会话'}</span>
              <span className={styles.sessionTime}>{new Date(s.created_at).toLocaleString()}</span>
            </div>
          ))}
        </div>

        <PromptInput onSend={handleSend}
          sending={creating}
          disabled={!selectedAgent || creating}
          placeholder="输入 prompt 开始新对话..."
        />
      </div>
    </div>
  )
}
