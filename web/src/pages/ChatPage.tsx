import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getSession, listMessages, cancelSession, listCommands, listModes, listSkills, listConfigOptions, setConfigOption, setSessionMode, respondPermission, deleteSession, updateSessionTitle, createSession, resumeSession, listSessionExecutions, getInterruptedTasks } from '../api/sessions'
import { getWorkspace } from '../api/workspaces'
import { listScheduledTasks, listExecutions } from '../api/scheduledTasks'
import { listAgents, probeAgentConfigs, preconnectAgent, listAgentCommands, listAgentModes } from '../api/agents'
import { listSkillsByPath } from '../api/filesystem'
import { getAgentPrefs, patchAgentPrefs } from '../api/agentPrefs'
import { WORKSPACE_STORAGE_KEY, useCurrentWorkspace } from '../hooks/useCurrentWorkspace'
import { applyPrefsToConfigs, configsFromProbe, takeLegacyLocalAgentPrefs } from '../utils/agentPrefs'
import { streamPrompt, subscribeStream, streamResumeTask, isTimeoutError } from '../api/sse'
import { tasksUrl, newTaskUrl, sessionUrl, isNewTaskPath } from '../utils/routes'
import type { Session, Message, AgentCommand, ConfigOption, SessionMode, AgentSkill, Execution, Agent, PermissionRequestPayload, RunningTask, AgentPrefs } from '../types'
import { parsePermissionRequest } from '../utils/permission'
import { formatOptionLabel, fullOptionLabel } from '../utils/selectLabel'
import AppLayout, { SidebarToggleButton } from '../components/AppLayout'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ModelSelector from '../components/ModelSelector'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import WorkspacePanel from '../components/WorkspacePanel'
import ContextStats from '../components/ContextStats'
import UserMenu from '../components/UserMenu'
import WorkspaceSelector from '../components/WorkspaceSelector'
import ModelPicker from '../components/ModelPicker'
import PermissionDialog from '../components/PermissionDialog'
import SessionModeSelector from '../components/SessionModeSelector'
import ConvStatusBar, { type ConvState as ConvStatusState } from '../components/ConvStatusBar'
import { FolderOpen, Plus } from 'lucide-react'
import styles from './ChatPage.module.css'

type NavigateState = { initialPrompt?: string; createdSession?: Session }

type ConvState = ConvStatusState

export default function ChatPage() {
  const { t, i18n } = useTranslation()
  const { wid, sid } = useParams<{ wid?: string; sid?: string }>()
  const urlWorkspaceId = wid ? Number(wid) : NaN
  const sessionId = sid ? Number(sid) : NaN
  const hasSession = !isNaN(sessionId)
  const { user, loading: authLoading } = useRequireAuth()
  const { workspaceId: storedWorkspaceId, sessions, selectWorkspace, reload: reloadWorkspace } = useCurrentWorkspace(!!user)
  const workspaceId = !isNaN(urlWorkspaceId) ? urlWorkspaceId : storedWorkspaceId
  const navigate = useNavigate()
  const location = useLocation()
  const initialPromptRef = useRef<string>('')
  const bootstrappedSessionIdRef = useRef<number | null>(null)
  // location.state 变化时同步到 ref（navigate 跳转不会重新挂载组件，useRef 不会自动更新）
  const navState = location.state as NavigateState | null
  if (navState?.initialPrompt) {
    initialPromptRef.current = navState.initialPrompt
  }

  // 会话相关状态
  const [session, setSession] = useState<Session | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [restoreRefreshKey, setRestoreRefreshKey] = useState(0)
  const [restoreInput, setRestoreInput] = useState<string | undefined>(undefined)
  const [commands, setCommands] = useState<AgentCommand[]>([])
  const [modes, setModes] = useState<SessionMode[]>([])
  const [skills, setSkills] = useState<AgentSkill[]>([])
  const [configOptions, setConfigOptions] = useState<ConfigOption[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [convState, setConvState] = useState<ConvState>('idle')
  const abortRef = useRef<AbortController | null>(null)
  const mountedRef = useRef(true)
  // lastSeqRef 记录最后接收到的消息 sequence，用于断点续传重连时携带 Last-Event-ID
  const lastSeqRef = useRef(0)
  // interruptedTasks 存储因服务重启而中断的任务，用于显示重发横幅
  const [interruptedTasks, setInterruptedTasks] = useState<RunningTask[]>([])
  const [showWorkspace, setShowWorkspace] = useState(false)
  const [lastFailedPrompt, setLastFailedPrompt] = useState('')
  const [retryable, setRetryable] = useState(false)
  const [executions, setExecutions] = useState<Execution[]>([])
  const [currentModeId, setCurrentModeId] = useState('')
  const [pendingPermission, setPendingPermission] = useState<PermissionRequestPayload | null>(null)
  const [permissionResponding, setPermissionResponding] = useState(false)
  const permissionQueueRef = useRef<PermissionRequestPayload[]>([])
  const waitingPermissionRef = useRef(false)

  function enqueuePermission(req: PermissionRequestPayload) {
    waitingPermissionRef.current = true
    setPendingPermission((prev) => {
      if (prev?.request_id === req.request_id) return prev
      if (!prev) return req
      if (!permissionQueueRef.current.some((p) => p.request_id === req.request_id)) {
        permissionQueueRef.current = [...permissionQueueRef.current, req]
      }
      return prev
    })
    setConvState('waiting_permission')
  }

  function clearPermissions() {
    permissionQueueRef.current = []
    waitingPermissionRef.current = false
    setPendingPermission(null)
  }

  function advancePermissionQueue() {
    const next = permissionQueueRef.current.shift() || null
    waitingPermissionRef.current = !!next
    setPendingPermission(next)
    if (next) setConvState('waiting_permission')
    else setConvState((s) => (s === 'waiting_permission' ? 'streaming' : s))
  }

  // 无会话模式下的 agent / 模型选择状态
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgent, setSelectedAgent] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [probeConfigs, setProbeConfigs] = useState<ConfigOption[]>([])
  const [probing, setProbing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [homeCommands, setHomeCommands] = useState<AgentCommand[]>([])
  const [homeModes, setHomeModes] = useState<SessionMode[]>([])
  const [homeSkills, setHomeSkills] = useState<AgentSkill[]>([])
  const [workspaceCwd, setWorkspaceCwd] = useState('')
  const [agentPrefs, setAgentPrefs] = useState<AgentPrefs>({ last_agent_type: '', prefs: {} })
  const prefsSaveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const agentPrefsRef = useRef(agentPrefs)
  agentPrefsRef.current = agentPrefs

  const bootstrapSession = navState?.createdSession?.id === sessionId ? navState.createdSession : null
  const isCreateMode = !hasSession && isNewTaskPath(location.pathname, workspaceId)
  const activeSession = session ?? bootstrapSession

  // 从消息流中提取当前 session mode
  useEffect(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      const msg = messages[i]
      if (msg.kind !== 'current_mode_update') continue
      try {
        const data = JSON.parse(msg.raw_json)
        const modeId = data?.CurrentModeUpdate?.currentModeId || data?.current_mode_update?.currentModeId
        if (modeId) {
          setCurrentModeId(String(modeId))
          return
        }
      } catch { /* ignore */ }
    }
  }, [messages])

  // 加载会话数据（有会话时）；quiet 模式下不阻塞 UI（用于新建会话后的后台刷新）
  const loadData = useCallback(async (opts?: { quiet?: boolean }) => {
    if (!hasSession) return
    if (!opts?.quiet) { setLoading(true); setError('') }
    try {
      const [sessionResp, msgResp] = await Promise.all([
        getSession(sessionId), listMessages(sessionId),
      ])
      setSession(sessionResp.data)
      const msgs = msgResp.data.messages || []
      setMessages(msgs)
      // 同步 lastSeqRef 为当前最大 sequence（用于断点续传重连）
      if (msgs.length > 0) {
        lastSeqRef.current = msgs[msgs.length - 1].sequence
      }
      // 查询中断任务（服务重启后未完成的 prompt）
      getInterruptedTasks(sessionId).then((r) => setInterruptedTasks(r.data.tasks || [])).catch(() => setInterruptedTasks([]))
      // 会话列表由 useCurrentWorkspace 统一加载，会话详情模式下通过 URL workspace 同步 effect 保持一致
      if (sessionResp.data.source === 'scheduled' || sessionResp.data.source === 'classify') {
        loadExecutions(sessionId, sessionResp.data.source)
      } else { setExecutions([]) }
      listCommands(sessionId).then((r) => setCommands(r.data.commands || [])).catch(() => setCommands([]))
      listModes(sessionId).then((r) => {
        const modeList = r.data.modes || []
        setModes(modeList)
        if (modeList.length > 0) {
          setCurrentModeId((prev) => prev || modeList[0].id)
        }
      }).catch(() => setModes([]))
      listSkills(sessionId).then((r) => setSkills(r.data.skills || [])).catch(() => setSkills([]))
      listConfigOptions(sessionId).then((r) => {
        const opts = r.data.config_options || []
        setConfigOptions(opts)
      }).catch(() => setConfigOptions([]))
    } catch (err) {
      if (!opts?.quiet) setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { if (!opts?.quiet) setLoading(false) }
  }, [sessionId, hasSession, workspaceId])

  // 加载 agent 列表和会话列表（无会话时）
  const loadHomeData = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const [agentsResp, wsResp, prefsResp] = await Promise.all([
        listAgents(),
        workspaceId ? getWorkspace(workspaceId) : Promise.resolve(null),
        getAgentPrefs().catch(() => ({ data: { last_agent_type: '', prefs: {} } as AgentPrefs })),
      ])
      setAgents(agentsResp.data.agents || [])
      setWorkspaceCwd(wsResp?.data.workspace?.cwd || '')

      let prefs = prefsResp.data
      if (!prefs.last_agent_type && Object.keys(prefs.prefs || {}).length === 0) {
        const legacy = takeLegacyLocalAgentPrefs()
        if (legacy) {
          try {
            for (const [agentType, configs] of Object.entries(legacy.prefs)) {
              prefs = (await patchAgentPrefs({
                last_agent_type: legacy.last_agent_type || undefined,
                agent_type: agentType,
                configs,
              })).data
            }
            if (legacy.last_agent_type && Object.keys(legacy.prefs).length === 0) {
              prefs = (await patchAgentPrefs({ last_agent_type: legacy.last_agent_type })).data
            }
          } catch { /* 迁移失败不影响主流程 */ }
        }
      }
      setAgentPrefs(prefs)

      if (agentsResp.data.agents?.length > 0) {
        const types = agentsResp.data.agents.map((a: Agent) => a.type)
        if (prefs.last_agent_type && types.includes(prefs.last_agent_type)) {
          setSelectedAgent(prefs.last_agent_type)
        } else {
          setSelectedAgent(agentsResp.data.agents[0].type)
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setLoading(false) }
  }, [workspaceId])

  function schedulePrefsPatch(payload: { last_agent_type?: string; agent_type?: string; configs?: Record<string, string> }) {
    setAgentPrefs((prev) => {
      const next: AgentPrefs = {
        last_agent_type: payload.last_agent_type ?? prev.last_agent_type,
        prefs: { ...prev.prefs },
      }
      if (payload.agent_type && payload.configs) {
        const cur = { ...(next.prefs[payload.agent_type] || {}) }
        for (const [k, v] of Object.entries(payload.configs)) {
          if (!v) delete cur[k]
          else cur[k] = v
        }
        if (Object.keys(cur).length === 0) delete next.prefs[payload.agent_type]
        else next.prefs[payload.agent_type] = cur
      }
      return next
    })
    if (prefsSaveTimer.current) clearTimeout(prefsSaveTimer.current)
    prefsSaveTimer.current = setTimeout(() => {
      patchAgentPrefs(payload)
        .then((r) => setAgentPrefs(r.data))
        .catch(() => { /* 静默 */ })
    }, 300)
  }

  function handleWorkspaceChange(id: number) {
    if (hasSession) {
      localStorage.setItem(WORKSPACE_STORAGE_KEY, String(id))
      navigate(tasksUrl(id))
      return
    }
    // 无会话模式：同步更新 URL，使刷新/书签/后退按钮都能定位到正确工作区
    navigate(isCreateMode ? newTaskUrl(id) : tasksUrl(id))
  }

  function handleWorkspaceRefresh() {
    reloadWorkspace()
      .then(() => (hasSession ? loadData({ quiet: true }) : loadHomeData()))
      .catch((err) => setError(err instanceof Error ? err.message : t('common.failed')))
  }

  // 监听 agent 变化，探测 config options
  useEffect(() => {
    if (hasSession || !isCreateMode || !selectedAgent) {
      setProbeConfigs([])
      setSelectedModel('')
      return
    }
    let alive = true
    setProbing(true)
    probeAgentConfigs(selectedAgent)
      .then((r) => {
        if (!alive) return
        const opts = r.data.config_options || []
        const applied = applyPrefsToConfigs(opts, agentPrefsRef.current.prefs[selectedAgent])
        setProbeConfigs(applied)
        const modelOpt = applied.find((o) => o.category === 'model')
        setSelectedModel(modelOpt?.current_value || '')
      })
      .catch((err) => {
        if (!alive) return
        setProbeConfigs([])
        const savedModel = agentPrefsRef.current.prefs[selectedAgent]?.model || ''
        setSelectedModel(savedModel)
        setError(err instanceof Error ? err.message : '探测配置失败')
      })
      .finally(() => { if (alive) setProbing(false) })
    return () => { alive = false }
  }, [selectedAgent, hasSession, isCreateMode])

  // 新建页：提前预连接 agent，减少首次发送时的冷启动等待。
  // agent 选中后立即用 probe cwd 预热；workspaceCwd 就绪后若不同则再次预热。
  useEffect(() => {
    if (hasSession || !isCreateMode || !selectedAgent) return
    preconnectAgent(selectedAgent, workspaceCwd)
  }, [selectedAgent, workspaceCwd, hasSession, isCreateMode])

  // 新建任务页：加载 agent 级 slash command / mode（探测完成后刷新）
  useEffect(() => {
    if (hasSession || !isCreateMode || !selectedAgent || probing) {
      if (hasSession || !isCreateMode) {
        setHomeCommands([])
        setHomeModes([])
      }
      return
    }
    listAgentCommands(selectedAgent, workspaceCwd || undefined).then((r) => setHomeCommands(r.data.commands || [])).catch(() => setHomeCommands([]))
    listAgentModes(selectedAgent).then((r) => setHomeModes(r.data.modes || [])).catch(() => setHomeModes([]))
  }, [selectedAgent, hasSession, isCreateMode, probing, workspaceCwd])

  // 新建任务页：加载 skills（与 agent 无关；cwd 为空时仍扫用户目录）
  useEffect(() => {
    if (hasSession || !isCreateMode) {
      setHomeSkills([])
      return
    }
    listSkillsByPath(workspaceCwd || undefined)
      .then((r) => setHomeSkills(r.data.skills || []))
      .catch(() => setHomeSkills([]))
  }, [workspaceCwd, hasSession, isCreateMode])

  // 将 URL 中的 workspace 同步到 hook，使 sidebar 展示该 workspace 的会话列表。
  // 任务列表页（无会话）与 会话详情页 都需要同步，否则切换工作区时侧边栏会显示其它工作区的会话。
  useEffect(() => {
    if (!user || isNaN(urlWorkspaceId)) return
    if (urlWorkspaceId !== storedWorkspaceId) {
      selectWorkspace(urlWorkspaceId).catch(() => {})
    }
  }, [user, urlWorkspaceId, storedWorkspaceId, selectWorkspace])

  useEffect(() => {
    if (!user) return
    if (hasSession) {
      const created = navState?.createdSession
      if (created?.id === sessionId && bootstrappedSessionIdRef.current !== sessionId) {
        bootstrappedSessionIdRef.current = sessionId
        setSession(created)
        setMessages([])
        setLoading(false)
        setCreating(false)
        // 刷新会话列表，使 sidebar 显示新建的会话
        reloadWorkspace()
        loadData({ quiet: true })
      } else if (bootstrappedSessionIdRef.current !== sessionId) {
        loadData()
      }
    } else {
      bootstrappedSessionIdRef.current = null
      loadHomeData()
    }
  }, [user, hasSession, sessionId, loadData, loadHomeData, reloadWorkspace, navState?.createdSession])

  // 当组件卸载或切换到不同会话时，中断旧的 SSE 流，防止内存泄漏和 React 警告
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      if (abortRef.current) {
        abortRef.current.abort()
        abortRef.current = null
      }
    }
  }, [sessionId])

  // 定时轮询执行状态：当会话活跃或为定时/分类任务时，每 5 秒刷新消息和执行列表。
  // 即使前端重启，也能通过 loadData 从数据库获取最新状态。
  // 同时清理「UI 仍显示生成中但已无 SSE」的陈旧状态（ACP 恢复后常见）。
  useEffect(() => {
    if (!hasSession || !session) return
    const needPoll = session.status === 'active' || session.status === 'pending' ||
      session.source === 'scheduled' || session.source === 'classify'
    if (!needPoll) return

    const interval = setInterval(() => {
      if (!mountedRef.current) return
      if (abortRef.current == null) {
        setConvState((s) => (
          s === 'streaming' || s === 'reconnecting' || s === 'connecting' ? 'idle' : s
        ))
      }
      loadData({ quiet: true })
    }, 5000)

    return () => clearInterval(interval)
  }, [hasSession, session?.id, session?.status, session?.source, loadData])

  useEffect(() => {
    if (loading && !bootstrapSession) return // 新建会话有 bootstrap 数据时不等待 loadData
    if (!activeSession) return
    const pending = initialPromptRef.current
    if (!pending) return
    initialPromptRef.current = ''
    if (location.state) navigate(location.pathname, { replace: true, state: null })
    handleSend(pending)
  }, [activeSession, loading, bootstrapSession])

  // 页面可见性恢复时，若会话有进行中的 prompt 但前端未在流式接收，尝试断点续传重连。
  // 使用 subscribeStream 而非发起新 prompt，避免重复执行。
  useEffect(() => {
    if (!hasSession) return
    const handleVisible = () => {
      if (document.visibilityState !== 'visible') return
      if (!mountedRef.current) return
      // 已在流式接收则无需重连；等待权限时也不抢占
      if (abortRef.current) return
      if (convState === 'waiting_permission' || waitingPermissionRef.current) return
      // 尝试订阅会话当前进行中的 prompt 流（若服务端无活跃 prompt 会立即返回）
      setConvState('reconnecting')
      const ac = new AbortController()
      abortRef.current = ac
      subscribeStream(
        sessionId,
        lastSeqRef.current,
        (msg) => {
          if (!mountedRef.current) return
          if (msg.kind === 'permission_request') {
            const req = parsePermissionRequest(msg.raw_json)
            if (req) enqueuePermission(req)
          }
          setMessages((prev) => {
            if (prev.some((m) => m.sequence === msg.sequence)) return prev
            return [...prev, msg]
          })
          setConvState((s) => (s === 'waiting_permission' ? s : 'streaming'))
        },
        () => {
          if (!mountedRef.current) return
          abortRef.current = null
          setConvState(waitingPermissionRef.current ? 'waiting_permission' : 'idle')
          loadData({ quiet: true })
        },
        () => {
          if (!mountedRef.current) return
          abortRef.current = null
          // 静默处理重连失败（服务端可能无活跃 prompt）
          setConvState(waitingPermissionRef.current ? 'waiting_permission' : 'idle')
          loadData({ quiet: true })
        },
        {
          signal: ac.signal,
          onSeq: (seq) => { if (seq > lastSeqRef.current) lastSeqRef.current = seq },
          // 仅真正收到消息才进入 streaming；连接建立本身不改状态，避免误显「生成中」
          onActivity: () => {
            if (mountedRef.current) setConvState((s) => (s === 'idle' ? 'streaming' : s))
          },
          shouldPauseIdleTimeout: () => waitingPermissionRef.current,
        },
      )
    }
    document.addEventListener('visibilitychange', handleVisible)
    return () => document.removeEventListener('visibilitychange', handleVisible)
  }, [hasSession, sessionId, convState, loadData])

  // 组件挂载时也尝试一次订阅（处理页面刷新后服务端仍在输出但前端未连接的情况）
  useEffect(() => {
    if (!hasSession || !session || convState !== 'idle') return
    if (session.status !== 'active') return
    if (abortRef.current) return
    const ac = new AbortController()
    abortRef.current = ac
    setConvState('reconnecting')
    subscribeStream(
      sessionId,
      lastSeqRef.current,
      (msg) => {
        if (!mountedRef.current) return
        if (msg.kind === 'permission_request') {
          const req = parsePermissionRequest(msg.raw_json)
          if (req) enqueuePermission(req)
        }
        setMessages((prev) => {
          if (prev.some((m) => m.sequence === msg.sequence)) return prev
          return [...prev, msg]
        })
        setConvState((s) => (s === 'waiting_permission' ? s : 'streaming'))
      },
      () => {
        if (!mountedRef.current) return
        abortRef.current = null
        setConvState(waitingPermissionRef.current ? 'waiting_permission' : 'idle')
      },
      () => {
        if (!mountedRef.current) return
        abortRef.current = null
        setConvState(waitingPermissionRef.current ? 'waiting_permission' : 'idle')
      },
      {
        signal: ac.signal,
        onSeq: (seq) => { if (seq > lastSeqRef.current) lastSeqRef.current = seq },
        onActivity: () => { if (mountedRef.current) setConvState((s) => (s === 'idle' ? 'streaming' : s)) },
        shouldPauseIdleTimeout: () => waitingPermissionRef.current,
      },
    )
    return () => { ac.abort() }
    // 仅在 session.id 变化时触发
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasSession, session?.id])

  async function loadExecutions(dbSessionId: number, source?: Session['source']) {
    try {
      if (source === 'classify') {
        const execResp = await listSessionExecutions(dbSessionId)
        setExecutions(execResp.data.executions || [])
        return
      }
      const tasksResp = await listScheduledTasks(workspaceId || undefined)
      const task = (tasksResp.data.tasks || []).find((t) => t.db_session_id === dbSessionId)
      if (task) { const execResp = await listExecutions(task.id); setExecutions(execResp.data.executions || []) }
    } catch { /* silent */ }
  }

  // 无会话时：创建会话并发起首次对话
  async function handleFirstSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, workspaceId || 0, selectedModel || undefined)
      const extras = probeConfigs.filter((o) => o.type === 'select' && o.category !== 'model' && o.current_value)
      for (const o of extras) {
        try { await setConfigOption(resp.data.id, o.id, o.current_value) } catch { /* 部分失败可接受 */ }
      }
      const configs = configsFromProbe(
        selectedModel
          ? probeConfigs.map((o) => (o.category === 'model' ? { ...o, current_value: selectedModel } : o))
          : probeConfigs,
      )
      try {
        const r = await patchAgentPrefs({
          last_agent_type: selectedAgent,
          agent_type: selectedAgent,
          configs: Object.keys(configs).length > 0 ? configs : undefined,
        })
        setAgentPrefs(r.data)
      } catch { /* 静默 */ }
      navigate(`/workspaces/${resp.data.workspace_id}/sessions/${resp.data.id}`, {
        state: { initialPrompt: prompt, createdSession: resp.data },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      setCreating(false)
    }
  }

  async function handleSend(prompt: string) {
    if (!activeSession) return
    setConvState('connecting')
    setError('')
    setRetryable(false)
    setLastFailedPrompt('')
    setRestoreInput(undefined)

    // 乐观展示用户消息，避免发送后界面无反馈
    const optimisticId = -Date.now()
    const optimisticMsg: Message = {
      id: optimisticId,
      session_id: activeSession.session_id,
      role: 'user',
      kind: 'user_message_chunk',
      content: prompt,
      raw_json: '',
      sequence: 0,
      execution_id: null,
      created_at: new Date().toISOString(),
    }
    setMessages((prev) => [...prev, optimisticMsg])

    const ac = new AbortController()
    abortRef.current = ac
    let gotAgentReply = false

    await streamPrompt(
      sessionId,
      prompt,
      (msg) => {
        if (!mountedRef.current) return
        if (msg.role !== 'user') gotAgentReply = true
        if (msg.kind === 'permission_request') {
          const req = parsePermissionRequest(msg.raw_json)
          if (req) enqueuePermission(req)
        }
        // 按 sequence 去重：避免轮询补齐与实时流重复
        setMessages((prev) => {
          const rest = prev.filter((m) => m.id === optimisticId || m.sequence < msg.sequence)
          const noOptimistic = rest.filter((m) => m.id !== optimisticId)
          return [...noOptimistic, msg]
        })
        setConvState((s) => (s === 'waiting_permission' ? s : 'streaming'))
      },
      () => {
        if (!mountedRef.current) return
        abortRef.current = null
        clearPermissions()
        setConvState('idle')
        setLastFailedPrompt('')
        setRetryable(false)
        loadData({ quiet: true })
      },
      async (err) => {
        if (!mountedRef.current) return
        abortRef.current = null
        setMessages((prev) => prev.filter((m) => m.id !== optimisticId))
        if (isTimeoutError(err)) {
          await recoverFromTimeout(prompt, gotAgentReply)
          return
        }
        clearPermissions()
        setConvState('idle')
        setError(err.message)
      },
      {
        signal: ac.signal,
        onSeq: (seq) => { if (seq > lastSeqRef.current) lastSeqRef.current = seq },
        onActivity: () => { if (mountedRef.current) setConvState((s) => (s === 'waiting_permission' ? s : 'streaming')) },
        shouldPauseIdleTimeout: () => waitingPermissionRef.current,
      },
    )
  }

  const displayConvState: ConvState = pendingPermission ? 'waiting_permission' : convState

  async function recoverFromTimeout(prompt: string, gotAgentReply: boolean) {
    setConvState('reconnecting')
    clearPermissions()
    setError('')
    try {
      try { await cancelSession(sessionId) } catch { /* 尽力取消挂起的 prompt */ }
      const resp = await resumeSession(sessionId)
      setSession(resp.data)
      await loadData({ quiet: true })
      setConvState('idle')
      if (!gotAgentReply) {
        setLastFailedPrompt(prompt)
        setRetryable(true)
        setError(t('session.timeoutReconnected'))
      } else {
        setError(t('session.timeoutReconnected'))
      }
    } catch (err) {
      setConvState('idle')
      setError(err instanceof Error ? err.message : t('common.failed'))
      setLastFailedPrompt(prompt)
      setRetryable(true)
    }
  }

  const sending = convState !== 'idle'

  async function handleRetry() { if (!lastFailedPrompt) return; setError(''); setRetryable(false); await handleSend(lastFailedPrompt) }

  // 恢复中断的任务（服务重启后用户手动重发）
  async function handleResumeInterruptedTask(taskId: number) {
    setConvState('connecting')
    setError('')
    const ac = new AbortController()
    abortRef.current = ac
    await streamResumeTask(
      taskId,
      (msg) => {
        if (!mountedRef.current) return
        if (msg.kind === 'permission_request') {
          const req = parsePermissionRequest(msg.raw_json)
          if (req) enqueuePermission(req)
        }
        setMessages((prev) => {
          if (prev.some((m) => m.sequence === msg.sequence)) return prev
          return [...prev, msg]
        })
        setConvState((s) => (s === 'waiting_permission' ? s : 'streaming'))
      },
      () => {
        if (!mountedRef.current) return
        abortRef.current = null
        clearPermissions()
        setConvState('idle')
        loadData({ quiet: true })
      },
      (err) => {
        if (!mountedRef.current) return
        abortRef.current = null
        clearPermissions()
        setConvState('idle')
        setError(err.message)
      },
      {
        signal: ac.signal,
        onSeq: (seq) => { if (seq > lastSeqRef.current) lastSeqRef.current = seq },
        onActivity: () => { if (mountedRef.current) setConvState((s) => (s === 'waiting_permission' ? s : 'streaming')) },
        shouldPauseIdleTimeout: () => waitingPermissionRef.current,
      },
    )
  }

  async function handleCancel() {
    abortRef.current?.abort()
    abortRef.current = null
    try { await cancelSession(sessionId) } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
    clearPermissions()
    setConvState('idle')
  }

  async function handleSetMode(modeId: string) {
    if (!modeId || modeId === currentModeId) return
    setError('')
    setCurrentModeId(modeId)
    try {
      await setSessionMode(sessionId, modeId)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      loadData({ quiet: true })
    }
  }

  async function handlePermissionRespond(optionId: string) {
    if (!pendingPermission) return
    setPermissionResponding(true)
    setError('')
    try {
      const current = pendingPermission
      const queued = optionId === 'allow-always' ? [...permissionQueueRef.current] : []
      if (optionId === 'allow-always') permissionQueueRef.current = []
      await respondPermission(sessionId, current.request_id, optionId)
      // allow-always 已由后端批量放行；前端同步清队列，避免弹窗连环弹出
      for (const req of queued) {
        try { await respondPermission(sessionId, req.request_id, optionId) } catch { /* 后端可能已批量处理 */ }
      }
      if (optionId === 'allow-always') {
        waitingPermissionRef.current = false
        setPendingPermission(null)
        setConvState('streaming')
      } else {
        advancePermissionQueue()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setPermissionResponding(false)
    }
  }

  async function handlePermissionCancel() {
    if (!pendingPermission) return
    setPermissionResponding(true)
    setError('')
    try {
      await respondPermission(sessionId, pendingPermission.request_id, '', true)
      advancePermissionQueue()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally {
      setPermissionResponding(false)
    }
  }

  async function handleSetConfigOption(configId: string, value: string) {
    setError('')
    const opt = configOptions.find((o) => o.id === configId)
    setConfigOptions((prev) => prev.map((o) => (o.id === configId ? { ...o, current_value: value } : o)))
    try {
      await setConfigOption(sessionId, configId, value)
      if (opt?.category && activeSession?.agent_type) {
        schedulePrefsPatch({
          last_agent_type: activeSession.agent_type,
          agent_type: activeSession.agent_type,
          configs: { [opt.category]: value },
        })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      listConfigOptions(sessionId).then((r) => setConfigOptions(r.data.config_options || [])).catch(() => {})
    }
  }

  async function handleDeleteSession(id: number) {
    setError('')
    try {
      await deleteSession(id)
      if (id === sessionId) {
        localStorage.setItem(WORKSPACE_STORAGE_KEY, String(workspaceId))
        navigate(tasksUrl(workspaceId))
      }
      // 刷新会话列表，使 sidebar 同步删除
      reloadWorkspace()
    } catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  async function handleRenameSession(id: number, title: string) {
    setError('')
    try {
      const resp = await updateSessionTitle(id, title)
      if (id === sessionId) setSession(resp.data)
      // 刷新会话列表，使 sidebar 同步新标题
      reloadWorkspace()
    }
    catch (err) { setError(err instanceof Error ? err.message : t('common.failed')) }
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null
  if (hasSession && loading && !activeSession) return <LoadingSpinner text={t('common.loading')} />

  // ============ 无会话模式：任务列表 / 新建任务 ============
  if (!hasSession) {
    const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US'
    const manualSessions = sessions.filter((s) => !s.source || s.source === 'manual')

    if (!isCreateMode) {
      return (
        <AppLayout sidebarProps={{ sessions, workspaceId, onDelete: handleDeleteSession, onRename: handleRenameSession }}>
          <div className={styles.main}>
            <div className={`${styles.header} ${styles.headerSingle}`}>
              <div className={styles.sessionInfo}>
                <SidebarToggleButton />
                <span className={styles.agentType}>{t('nav.sessionList')}</span>
              </div>
              <div className={styles.actions}>
                <WorkspaceSelector value={workspaceId} onChange={handleWorkspaceChange} onRefresh={handleWorkspaceRefresh} onError={setError} />
                <button type="button" className={styles.newTaskBtn} onClick={() => navigate(newTaskUrl(workspaceId))}><Plus size={14} style={{ verticalAlign: '-2px' }} /> {t('session.newSession')}</button>
                <UserMenu />
              </div>
            </div>

            {error && <ErrorBanner message={error} onClose={() => setError('')} />}

            {loading ? <LoadingSpinner /> : (
              <div className={styles.listContent}>
                <h3 className={styles.listTitle}>{t('session.history')}</h3>
                <div className={styles.sessionList}>
                  {manualSessions.length === 0 ? (
                    <p className={styles.listEmpty}>{t('session.noSessions')}</p>
                  ) : (
                    manualSessions.map((item) => (
                      <div key={item.id} className={styles.sessionCard}
                        onClick={() => navigate(sessionUrl(item.id, item.workspace_id))}
                      >
                        <div className={styles.sessionHeader}>
                          <span className={styles.sessionAgent}>{item.title || item.agent_type}</span>
                          {(item.status === 'active' || item.status === 'error') && (
                            <span className={`${styles.sessionStatus} ${styles[`status_${item.status}`] || ''}`}>
                              {item.status === 'active' ? t('session.active') : t('status.error')}
                            </span>
                          )}
                        </div>
                        {item.last_prompt && <p className={styles.sessionPrompt}>{item.last_prompt}</p>}
                        <div className={styles.sessionFooter}>
                          <span className={styles.sessionTime}>{new Date(item.created_at).toLocaleString(locale)}</span>
                          <button type="button" className={styles.listDeleteBtn}
                            onClick={(e) => { e.stopPropagation(); handleDeleteSession(item.id) }}
                          >{t('common.delete')}</button>
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </div>
            )}
          </div>
        </AppLayout>
      )
    }

    return (
      <AppLayout sidebarProps={{ sessions, workspaceId, onDelete: handleDeleteSession, onRename: handleRenameSession }}>
        <div className={styles.main}>
          <div className={`${styles.header} ${styles.headerSingle}`}>
            <div className={styles.sessionInfo}>
              <SidebarToggleButton />
              <span className={styles.agentType}>{t('session.newSession')}</span>
            </div>
            <div className={styles.actions}>
              <WorkspaceSelector value={workspaceId} onChange={handleWorkspaceChange} onRefresh={handleWorkspaceRefresh} onError={setError} />
              <button type="button" className={styles.newTaskBtn} onClick={() => navigate(newTaskUrl(workspaceId))}><Plus size={14} style={{ verticalAlign: '-2px' }} /> {t('session.newSession')}</button>
              <UserMenu />
            </div>
          </div>

          <div className={styles.configBar}>
            <div className={styles.homeConfigRow}>
              <div className={styles.homeConfigItem}>
                <label className={styles.homeConfigLabel}>Agent</label>
                <select className={styles.homeConfigSelect}
                  value={selectedAgent}
                  onChange={(e) => {
                    const val = e.target.value
                    setSelectedAgent(val)
                    if (val) schedulePrefsPatch({ last_agent_type: val })
                  }}
                  disabled={creating}
                >
                  {agents.length === 0 && <option value="">无可用 Agent</option>}
                  {agents.map((agent) => (
                    <option key={agent.type} value={agent.type}>{agent.display_name}</option>
                  ))}
                </select>
              </div>
              {/* 渲染可选择的配置项（模型用可过滤输入，模式等用下拉框） */}
              {probeConfigs.filter((o) => o.type === 'select' && o.options.length > 0).map((opt) => {
                const label = opt.category === 'model' ? '模型'
                  : opt.category === 'mode' ? '模式'
                  : opt.category === 'thought_level' ? '思考级别'
                  : opt.name
                const isModel = opt.category === 'model'
                return (
                  <div key={opt.id} className={styles.homeConfigItem}>
                    <label className={styles.homeConfigLabel}>{label}</label>
                    {isModel ? (
                      <ModelPicker
                        value={selectedModel}
                        options={opt.options}
                        onChange={(val) => {
                          setSelectedModel(val)
                          setProbeConfigs((prev) => prev.map((o) => (o.id === opt.id ? { ...o, current_value: val } : o)))
                          if (selectedAgent) {
                            schedulePrefsPatch({
                              last_agent_type: selectedAgent,
                              agent_type: selectedAgent,
                              configs: { [opt.category || 'model']: val },
                            })
                          }
                        }}
                        disabled={probing || creating}
                        placeholder={t('session.selectModel')}
                      />
                    ) : (
                      <select className={styles.homeConfigSelect}
                        value={opt.current_value || ''}
                        disabled={probing || creating}
                        onChange={(ev) => {
                          const val = ev.target.value
                          setProbeConfigs((prev) => prev.map((o) => (o.id === opt.id ? { ...o, current_value: val } : o)))
                          if (selectedAgent && opt.category) {
                            schedulePrefsPatch({
                              last_agent_type: selectedAgent,
                              agent_type: selectedAgent,
                              configs: { [opt.category]: val },
                            })
                          }
                        }}
                      >
                        {opt.options.map((v) => (
                          <option key={v.value} value={v.value} title={fullOptionLabel(v.name, v.description)}>
                            {formatOptionLabel(v.name, v.description, 10)}
                          </option>
                        ))}
                      </select>
                    )}
                  </div>
                )
              })}
              {/* 探测无配置时提供手动输入 */}
              {!probing && !loading && probeConfigs.filter((o) => o.type === 'select' && o.options.length > 0).length === 0 && (
                <div className={styles.homeConfigItem}>
                  <label className={styles.homeConfigLabel}>模型</label>
                  <input className={styles.homeConfigInput}
                    type="text" value={selectedModel}
                    onChange={(e) => {
                      const val = e.target.value
                      setSelectedModel(val)
                      if (val && selectedAgent) {
                        schedulePrefsPatch({
                          last_agent_type: selectedAgent,
                          agent_type: selectedAgent,
                          configs: { model: val },
                        })
                      }
                    }}
                    placeholder="手动输入模型 ID"
                    disabled={creating}
                  />
                </div>
              )}
              {probing && <span className={styles.homeConfigHint}>探测配置中...</span>}
              {/* 工作目录：后续由 workspace 管理 */}
            </div>
          </div>

          {error && <ErrorBanner message={error} onClose={() => setError('')} />}

          {loading ? <LoadingSpinner /> : (
            <div className={styles.homeContent}>
              <div className={styles.homeHero}>
                <h2 className={styles.homeTitle}>{t('session.newSession')}</h2>
                <p className={styles.homeSubtitle}>{t('session.autoCreateHint')}</p>
              </div>
            </div>
          )}

          <PromptInput onSend={handleFirstSend}
            sending={creating}
            disabled={!selectedAgent || creating}
            commands={homeCommands}
            modes={homeModes}
            skills={homeSkills}
            cwd={workspaceCwd}
            workspaceId={workspaceId}
            placeholder={t('session.quickSendPlaceholder')}
          />
        </div>

      </AppLayout>
    )
  }

  // ============ 有会话模式 ============
  return (
    <AppLayout sidebarProps={{ sessions, workspaceId, currentId: sessionId, onDelete: handleDeleteSession, onRename: handleRenameSession }}>
      <div className={styles.main}>
        <div className={styles.header}>
          <div className={styles.sysBar}>
            <SidebarToggleButton />
            <div className={styles.actions}>
              <WorkspaceSelector value={workspaceId} onChange={handleWorkspaceChange} onRefresh={handleWorkspaceRefresh} onError={setError} />
              <button type="button" className={styles.newTaskBtn} onClick={() => navigate(newTaskUrl(workspaceId))}><Plus size={14} style={{ verticalAlign: '-2px' }} /> {t('session.newSession')}</button>
              <button className={`${styles.fileBtn} ${showWorkspace ? styles.fileBtnActive : ''}`}
                onClick={() => setShowWorkspace(!showWorkspace)} type="button" title={t('chat.workspace')}
              ><FolderOpen size={16} /></button>
            </div>
            <UserMenu />
          </div>
          <div className={styles.sessionInfo}>
            <span className={styles.agentType}>{activeSession?.agent_type || ''}</span>
            {displayConvState !== 'idle' && (
              <span className={`${styles.convStatus} ${styles[`conv_${displayConvState}`]}`}>
                {t(`session.conv_${displayConvState}`)}
              </span>
            )}
            {activeSession?.workspace?.cwd && (
              <span className={styles.cwd} title={activeSession.workspace.cwd}>
                {activeSession.workspace.cwd.split('/').filter(Boolean).pop()}
              </span>
            )}
          </div>

          <div className={styles.configBar}>
            <div className={styles.configOptions}>
              <SessionModeSelector
                modes={modes}
                currentModeId={currentModeId}
                onChange={handleSetMode}
                disabled={sending}
              />
              <ModelSelector options={configOptions} onApply={handleSetConfigOption} disabled={sending} />
            </div>
            <div className={styles.statsArea}><ContextStats messages={messages} /></div>
          </div>
        </div>

        {error && (
          <ErrorBanner
            message={retryable ? `${error} (${t('common.retry')})` : error}
            onClose={() => { setError(''); setRetryable(false) }}
            onRetry={retryable ? handleRetry : undefined}
          />
        )}

        {interruptedTasks.length > 0 && !sending && (
          <div className={styles.interruptedBanner}>
            <span>
              {t('session.interruptedPrompt', { count: interruptedTasks.length, defaultValue: `上次任务因服务重启中断（共 ${interruptedTasks.length} 个）` })}
            </span>
            {interruptedTasks.map((task) => (
              <button
                key={task.id}
                className={styles.resumeBtn}
                onClick={() => handleResumeInterruptedTask(task.id)}
                title={task.prompt}
              >
                {t('session.resume', { defaultValue: '重发' })}: {task.prompt.slice(0, 40)}{task.prompt.length > 40 ? '...' : ''}
              </button>
            ))}
          </div>
        )}

        <MessageList messages={messages} loading={sending}
          scheduled={activeSession?.source === 'scheduled' || activeSession?.source === 'classify'} executions={executions}
          sessionId={sessionId} cwd={activeSession?.workspace?.cwd || ''}
          onRestored={(promptText) => { loadData(); setRestoreRefreshKey((k) => k + 1); if (promptText) setRestoreInput(promptText) }}
        />

        {activeSession?.source === 'classify' ? (
          <p className={styles.classifyViewHint}>{t('notes.classifyTaskHint')}</p>
        ) : (
          <div className={styles.bottomArea}>
            <ConvStatusBar state={displayConvState}>
              {pendingPermission && (
                <PermissionDialog
                  request={pendingPermission}
                  responding={permissionResponding}
                  onRespond={handlePermissionRespond}
                  onCancel={handlePermissionCancel}
                />
              )}
            </ConvStatusBar>
            <PromptInput onSend={handleSend} onCancel={handleCancel}
              sending={sending} disabled={false}
              value={restoreInput} onValueChange={setRestoreInput}
              commands={commands} modes={modes} skills={skills} cwd={activeSession?.workspace?.cwd || ''}
              workspaceId={workspaceId}
              placeholder={sending ? t(`session.conv_${displayConvState === 'idle' ? 'connecting' : displayConvState}`) : t('session.promptPlaceholder')}
            />
          </div>
        )}
      </div>

      {showWorkspace && activeSession && (
        <div className={styles.workspaceWrap}>
          <WorkspacePanel sessionId={sessionId} cwd={activeSession.workspace?.cwd || ''}
            refreshKey={restoreRefreshKey}
            onClose={() => setShowWorkspace(false)}
          />
        </div>
      )}


    </AppLayout>
  )
}
