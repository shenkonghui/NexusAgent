import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listWorkspaces, createWorkspace, deleteWorkspace, updateWorkspace, saveWorkspace } from '../api/workspaces'
import { createSession } from '../api/sessions'
import { listAgents, probeAgentConfigs } from '../api/agents'
import type { Workspace, Agent, ConfigOption } from '../types'
import WorkspaceSidebar from '../components/WorkspaceSidebar'
import PromptInput from '../components/PromptInput'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import CreateWorkspaceDialog from '../components/CreateWorkspaceDialog'
import UserMenu from '../components/UserMenu'
import styles from './HomePage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

export default function HomePage() {
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()

  const [workspaces, setWorkspaces] = useState<(Workspace & { session_count?: number })[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgent, setSelectedAgent] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [probeConfigs, setProbeConfigs] = useState<ConfigOption[]>([])
  const [probing, setProbing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [showCreateDialog, setShowCreateDialog] = useState(false)

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [wsResp, agentsResp] = await Promise.all([listWorkspaces(), listAgents()])
      setWorkspaces(wsResp.data.workspaces || [])
      setAgents(agentsResp.data.agents || [])
      if (agentsResp.data.agents?.length > 0) {
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a: Agent) => a.type)
        setSelectedAgent(saved && types.includes(saved) ? saved : agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally { setLoading(false) }
  }, [])

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

  async function handleFirstSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, 0, selectedModel || undefined)
      localStorage.setItem(DEFAULT_AGENT_KEY, selectedAgent)
      navigate(`/workspaces/${resp.data.workspace_id}/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败')
      setCreating(false)
    }
  }

  async function handleCreateWorkspace(name: string, cwd: string) {
    try {
      await createWorkspace(name, cwd)
      setShowCreateDialog(false)
      loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败')
    }
  }

  async function handleDeleteWorkspace(id: number) {
    try { await deleteWorkspace(id); loadData() }
    catch (err) { setError(err instanceof Error ? err.message : '删除失败') }
  }

  async function handleRenameWorkspace(id: number, name: string) {
    try { await updateWorkspace(id, name); loadData() }
    catch (err) { setError(err instanceof Error ? err.message : '重命名失败') }
  }

  async function handleSaveWorkspace(id: number) {
    const ws = workspaces.find(w => w.id === id)
    if (ws) {
      try { await saveWorkspace(id, ws.name, ws.cwd); loadData() }
      catch (err) { setError(err instanceof Error ? err.message : '保存失败') }
    }
  }

  if (authLoading) return <LoadingSpinner />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <WorkspaceSidebar
        workspaces={workspaces}
        onDelete={handleDeleteWorkspace}
        onRename={handleRenameWorkspace}
        onSave={handleSaveWorkspace}
        onCreateClick={() => setShowCreateDialog(true)}
      />
      <div className={styles.main}>
        <div className={styles.header}>
          <span className={styles.agentType}>新对话</span>
          <UserMenu />
        </div>
        <div className={styles.configBar}>
          <select className={styles.configSelect} value={selectedAgent}
            onChange={e => setSelectedAgent(e.target.value)} disabled={creating}>
            {agents.map(a => <option key={a.type} value={a.type}>{a.display_name}</option>)}
          </select>
          {probeConfigs.filter(o => o.type === 'select' && o.options.length > 0 && o.category === 'model').map(opt => (
            <input key={opt.id} className={styles.configInput}
              value={selectedModel}
              onChange={e => setSelectedModel(e.target.value)}
              disabled={probing || creating}
              placeholder="模型"
              list={`list-${opt.id}`}
            />
          ))}
          {probing && <span>探测配置中...</span>}
        </div>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}
        {loading ? <LoadingSpinner /> : (
          <div className={styles.hero}>
            <h2>新对话</h2>
            <p>选择 Agent 后直接输入 prompt 开始对话（使用默认工作区）</p>
          </div>
        )}
        <PromptInput onSend={handleFirstSend}
          sending={creating}
          disabled={!selectedAgent || creating}
          placeholder="输入 prompt..."
        />
      </div>
      {showCreateDialog && (
        <CreateWorkspaceDialog onSubmit={handleCreateWorkspace} onClose={() => setShowCreateDialog(false)} />
      )}
    </div>
  )
}
