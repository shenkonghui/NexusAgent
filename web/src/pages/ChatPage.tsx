import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getSession, listMessages, closeSession, cancelSession, resumeSession, listSessions, listCommands, listModes, listSkills, listConfigOptions, setConfigOption, deleteSession, updateSessionTitle } from '../api/sessions'
import { listScheduledTasks, listExecutions } from '../api/scheduledTasks'
import { streamPrompt, isTimeoutError } from '../api/sse'
import { parseDiffsFromMessage } from '../utils/diff'
import type { Session, Message, AgentCommand, ConfigOption, SessionMode, AgentSkill, Execution } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ModelSelector from '../components/ModelSelector'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import WorkspacePanel from '../components/WorkspacePanel'
import ContextStats from '../components/ContextStats'
import UserMenu from '../components/UserMenu'
import styles from './ChatPage.module.css'

export default function ChatPage() {
  const { id } = useParams<{ id: string }>()
  const sessionId = Number(id)
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()
  const location = useLocation()
  // 首页快捷发起时携带的初始 prompt，发送后清除（避免刷新重复发送）
  const initialPromptRef = useRef<string>(
    (location.state as { initialPrompt?: string } | null)?.initialPrompt || '',
  )

  const [session, setSession] = useState<Session | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [allSessions, setAllSessions] = useState<Session[]>([])
  const [commands, setCommands] = useState<AgentCommand[]>([])
  const [modes, setModes] = useState<SessionMode[]>([])
  const [skills, setSkills] = useState<AgentSkill[]>([])
  const [configOptions, setConfigOptions] = useState<ConfigOption[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sending, setSending] = useState(false)
  // 右侧工作区面板（Tab 切换文件/终端/改动），默认开启
  const [showWorkspace, setShowWorkspace] = useState(true)
  // 左侧侧边栏折叠状态
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  // 最近一次失败的 prompt（用于重试）
  const [lastFailedPrompt, setLastFailedPrompt] = useState('')
  // 错误是否可重试（超时/网络错误）
  const [retryable, setRetryable] = useState(false)
  // 定时会话的执行状态列表
  const [executions, setExecutions] = useState<Execution[]>([])

  // 聚合所有消息中的文件改动数量（用于工作区 Tab 徽标）
  // 注意：useMemo 必须在所有 early return 之前调用，否则违反 Hooks 规则
  const changesCount = useMemo(() => {
    const paths = new Set<string>()
    for (const msg of messages) {
      for (const d of parseDiffsFromMessage(msg)) paths.add(d.path)
    }
    return paths.size
  }, [messages])

  // 加载会话和消息
  const loadData = useCallback(async () => {
    if (!sessionId) return
    setLoading(true)
    setError('')
    try {
      const [sessionResp, msgResp, sessionsResp] = await Promise.all([
        getSession(sessionId),
        listMessages(sessionId),
        listSessions(),
      ])
      setSession(sessionResp.data)
      setMessages(msgResp.data.messages || [])
      setAllSessions(sessionsResp.data.sessions || [])
      // 定时会话：获取执行状态列表
      if (sessionResp.data.source === 'scheduled') {
        loadExecutions(sessionId)
      } else {
        setExecutions([])
      }
      // 加载 slash 命令、modes、skills 和 config options（失败时静默，可能尚未有任何 prompt 触发）
      listCommands(sessionId)
        .then((r) => setCommands(r.data.commands || []))
        .catch(() => setCommands([]))
      listModes(sessionId)
        .then((r) => setModes(r.data.modes || []))
        .catch(() => setModes([]))
      listSkills(sessionId)
        .then((r) => setSkills(r.data.skills || []))
        .catch(() => setSkills([]))
      listConfigOptions(sessionId)
        .then((r) => setConfigOptions(r.data.config_options || []))
        .catch(() => setConfigOptions([]))
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载会话失败')
    } finally {
      setLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    if (user) loadData()
  }, [user, loadData])

  // 首页快捷发起：会话加载完成且活跃时，自动发送携带的初始 prompt
  useEffect(() => {
    if (!session || session.status !== 'active') return
    const pending = initialPromptRef.current
    if (!pending) return
    initialPromptRef.current = ''
    // 清除 location.state，避免刷新或后退时重复发送
    if (location.state) {
      navigate(location.pathname, { replace: true, state: null })
    }
    handleSend(pending)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session])

  // 定时会话：通过 db_session_id 找到关联 task，获取执行状态
  async function loadExecutions(dbSessionId: number) {
    try {
      const tasksResp = await listScheduledTasks()
      const task = (tasksResp.data.tasks || []).find((t) => t.db_session_id === dbSessionId)
      if (task) {
        const execResp = await listExecutions(task.id)
        setExecutions(execResp.data.executions || [])
      }
    } catch {
      // 静默失败
    }
  }

  // 发送 prompt（SSE 流）
  async function handleSend(prompt: string) {
    if (!session || session.status !== 'active') return
    setSending(true)
    setError('')
    setRetryable(false)
    setLastFailedPrompt('')

    await streamPrompt(
      sessionId,
      prompt,
      // onMessage: 追加消息，连续同 kind 的文本 chunk 由 MessageList 在渲染时合并显示
      (msg) => {
        setMessages((prev) => [...prev, msg])
      },
      // onDone: 流结束
      () => {
        setSending(false)
        setLastFailedPrompt('')
        setRetryable(false)
        // 刷新会话状态
        loadData()
      },
      // onError: 错误处理
      (err) => {
        setSending(false)
        setError(err.message)
        // 检测是否为超时/网络错误，支持重试
        if (isTimeoutError(err)) {
          setLastFailedPrompt(prompt)
          setRetryable(true)
        }
      },
    )
  }

  // 重试上次失败的 prompt
  async function handleRetry() {
    if (!lastFailedPrompt) return
    setError('')
    setRetryable(false)
    await handleSend(lastFailedPrompt)
  }

  // 取消当前 prompt
  async function handleCancel() {
    try {
      await cancelSession(sessionId)
      setSending(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : '取消失败')
    }
  }

  // 恢复/重开会话。closed 会话重开时若需要新工作目录会提示用户输入。
  async function handleResume() {
    if (!session) return
    setError('')
    let cwd: string | undefined
    // 已关闭会话的工作目录可能已被清理（temporary 模式），提示用户提供新目录
    if (session.status === 'closed') {
      cwd = window.prompt('重新打开会话，请输入工作目录（留空则复用原目录）', session.cwd || '') ?? undefined
      if (cwd === undefined) return // 用户取消
    }
    try {
      const resp = await resumeSession(sessionId, cwd)
      setSession(resp.data)
      // 重开后刷新命令列表
      listCommands(sessionId)
        .then((r) => setCommands(r.data.commands || []))
        .catch(() => setCommands([]))
      listModes(sessionId)
        .then((r) => setModes(r.data.modes || []))
        .catch(() => setModes([]))
      listSkills(sessionId)
        .then((r) => setSkills(r.data.skills || []))
        .catch(() => setSkills([]))
      listConfigOptions(sessionId)
        .then((r) => setConfigOptions(r.data.config_options || []))
        .catch(() => setConfigOptions([]))
    } catch (err) {
      setError(err instanceof Error ? err.message : '恢复失败')
    }
  }

  // 切换模型 / 设置 config option
  async function handleSetConfigOption(configId: string, value: string) {
    setError('')
    // 乐观更新本地状态
    setConfigOptions((prev) =>
      prev.map((o) => (o.id === configId ? { ...o, current_value: value } : o)),
    )
    try {
      await setConfigOption(sessionId, configId, value)
    } catch (err) {
      setError(err instanceof Error ? err.message : '设置失败')
      // 回滚：重新拉取
      listConfigOptions(sessionId)
        .then((r) => setConfigOptions(r.data.config_options || []))
        .catch(() => {})
    }
  }

  // 关闭会话
  async function handleClose() {
    try {
      await closeSession(sessionId)
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : '关闭失败')
    }
  }

  // 删除会话（左侧列表触发）
  async function handleDeleteSession(id: number) {
    setError('')
    try {
      await deleteSession(id)
      // 删除当前会话则跳回列表页，否则仅刷新侧边栏
      if (id === sessionId) {
        navigate('/')
      } else {
        setAllSessions((prev) => prev.filter((s) => s.id !== id))
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  // 重命名会话（左侧列表触发）
  async function handleRenameSession(id: number, title: string) {
    setError('')
    try {
      const resp = await updateSessionTitle(id, title)
      setAllSessions((prev) => prev.map((s) => (s.id === id ? resp.data : s)))
      if (id === sessionId) setSession(resp.data)
    } catch (err) {
      setError(err instanceof Error ? err.message : '修改标题失败')
    }
  }

  if (authLoading) return <LoadingSpinner text="验证登录状态..." />
  if (!user) return null
  if (loading) return <LoadingSpinner text="加载会话..." />

  return (
    <div className={styles.layout}>
      {/* 左侧：会话侧边栏（固定宽度，可隐藏） */}
      {!sidebarCollapsed && (
        <div className={styles.sidebarWrap}>
          <SessionSidebar
            sessions={allSessions}
            currentId={sessionId}
            onDelete={handleDeleteSession}
            onRename={handleRenameSession}
            onCollapse={() => setSidebarCollapsed(true)}
          />
        </div>
      )}

      {/* 中间：聊天区 */}
      <div className={styles.main}>
            {/* 顶部会话信息 */}
            <div className={styles.header}>
              <div className={styles.sessionInfo}>
                {sidebarCollapsed && (
                  <button
                    className={styles.iconBtn}
                    onClick={() => setSidebarCollapsed(false)}
                    type="button"
                    title="展开侧边栏"
                  >
                    ☰
                  </button>
                )}
                <span className={styles.agentType}>{session?.agent_type || '未知'}</span>
                <span className={`${styles.statusBadge} ${styles[`status_${session?.status}`] || ''}`}>
                  {session?.status === 'active' ? '活跃' : session?.status === 'closed' ? '已关闭' : '错误'}
                </span>
                {session?.cwd && <span className={styles.cwd}>{session.cwd}</span>}
              </div>
              <div className={styles.actions}>
                <button
                  className={`${styles.fileBtn} ${showWorkspace ? styles.fileBtnActive : ''}`}
                  onClick={() => setShowWorkspace(!showWorkspace)}
                  type="button"
                  title="工作区"
                >
                  🗂
                </button>
                {(session?.status === 'error' || session?.status === 'closed') && (
                  <button className={styles.resumeBtn} onClick={handleResume} type="button">
                    {session?.status === 'closed' ? '重新打开' : '恢复会话'}
                  </button>
                )}
                {session?.status === 'active' && (
                  <button className={styles.closeBtn} onClick={handleClose} type="button" title="关闭会话">
                    ✕
                  </button>
                )}
                <UserMenu />
              </div>
            </div>

            {/* 模型 / config option 选择条 + 上下文统计 */}
            {session?.status === 'active' && (
              <div className={styles.configBar}>
                <ModelSelector
                  options={configOptions}
                  onApply={handleSetConfigOption}
                  disabled={sending}
                />
                <div className={styles.statsArea}>
                  <ContextStats messages={messages} />
                </div>
              </div>
            )}

            {error && (
              <ErrorBanner
                message={retryable ? `${error}（可重试）` : error}
                onClose={() => { setError(''); setRetryable(false) }}
                onRetry={retryable ? handleRetry : undefined}
              />
            )}

            {/* 消息列表（定时会话按执行块分块渲染） */}
            <MessageList
              messages={messages}
              loading={sending}
              scheduled={session?.source === 'scheduled'}
              executions={executions}
              sessionId={sessionId}
              cwd={session?.cwd}
            />

            {/* 底部输入 */}
            <PromptInput
              onSend={handleSend}
              onCancel={handleCancel}
              sending={sending}
              disabled={session?.status !== 'active'}
              commands={commands}
              modes={modes}
              skills={skills}
              cwd={session?.cwd}
              placeholder={
                session?.status === 'closed'
                  ? '会话已关闭，点击「重新打开」继续'
                  : session?.status === 'error'
                    ? '会话状态异常，请先恢复'
                    : '输入 prompt，/ 命令/模式，@ 文件引用，Enter 发送，Shift+Enter 换行'
              }
            />
      </div>

      {/* 右侧：工作区面板（固定宽度，可隐藏） */}
      {showWorkspace && session && (
        <div className={styles.workspaceWrap}>
          <WorkspacePanel
            sessionId={sessionId}
            cwd={session.cwd}
            messages={messages}
            changesCount={changesCount}
            onClose={() => setShowWorkspace(false)}
          />
        </div>
      )}
    </div>
  )
}
