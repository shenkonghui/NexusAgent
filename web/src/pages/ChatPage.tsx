import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getSession, listMessages, closeSession, cancelSession, resumeSession, listSessions } from '../api/sessions'
import { streamPrompt } from '../api/sse'
import type { Session, Message } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import MessageList from '../components/MessageList'
import PromptInput from '../components/PromptInput'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import styles from './ChatPage.module.css'

export default function ChatPage() {
  const { id } = useParams<{ id: string }>()
  const sessionId = Number(id)
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()

  const [session, setSession] = useState<Session | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [allSessions, setAllSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sending, setSending] = useState(false)

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
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载会话失败')
    } finally {
      setLoading(false)
    }
  }, [sessionId])

  useEffect(() => {
    if (user) loadData()
  }, [user, loadData])

  // 发送 prompt（SSE 流）
  async function handleSend(prompt: string) {
    if (!session || session.status !== 'active') return
    setSending(true)
    setError('')

    await streamPrompt(
      sessionId,
      prompt,
      // onMessage: 合并连续的 assistant 消息（多种 kind）
      (msg) => {
        setMessages((prev) => {
          const last = prev[prev.length - 1]
          // 如果上一条是 assistant 且 sequence 连续，合并 content
          if (
            last &&
            last.role === 'assistant' &&
            last.sequence === msg.sequence - 1
          ) {
            return [
              ...prev.slice(0, -1),
              { ...last, content: last.content + msg.content },
            ]
          }
          return [...prev, msg]
        })
      },
      // onDone: 流结束
      () => {
        setSending(false)
        // 刷新会话状态
        loadData()
      },
      // onError: 错误处理
      (err) => {
        setSending(false)
        setError(err.message)
      },
    )
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

  // 恢复会话
  async function handleResume() {
    setError('')
    try {
      const resp = await resumeSession(sessionId)
      setSession(resp.data)
    } catch (err) {
      setError(err instanceof Error ? err.message : '恢复失败')
    }
  }

  // 关闭会话
  async function handleClose() {
    try {
      await closeSession(sessionId)
      navigate('/sessions')
    } catch (err) {
      setError(err instanceof Error ? err.message : '关闭失败')
    }
  }

  if (authLoading) return <LoadingSpinner text="验证登录状态..." />
  if (!user) return null
  if (loading) return <LoadingSpinner text="加载会话..." />

  return (
    <div className={styles.layout}>
      <SessionSidebar sessions={allSessions} currentId={sessionId} />

      <div className={styles.main}>
        {/* 顶部会话信息 */}
        <div className={styles.header}>
          <div className={styles.sessionInfo}>
            <span className={styles.agentType}>{session?.agent_type || '未知'}</span>
            <span className={`${styles.statusBadge} ${styles[`status_${session?.status}`] || ''}`}>
              {session?.status === 'active' ? '活跃' : session?.status === 'closed' ? '已关闭' : '错误'}
            </span>
            {session?.cwd && <span className={styles.cwd}>{session.cwd}</span>}
          </div>
          <div className={styles.actions}>
            {session?.status === 'error' && (
              <button className={styles.resumeBtn} onClick={handleResume} type="button">
                恢复会话
              </button>
            )}
            {session?.status === 'active' && (
              <button className={styles.closeBtn} onClick={handleClose} type="button">
                关闭会话
              </button>
            )}
          </div>
        </div>

        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        {/* 消息列表 */}
        <MessageList messages={messages} loading={sending} />

        {/* 底部输入 */}
        <PromptInput
          onSend={handleSend}
          onCancel={handleCancel}
          sending={sending}
          disabled={session?.status !== 'active'}
          placeholder={
            session?.status === 'closed'
              ? '会话已关闭'
              : session?.status === 'error'
                ? '会话状态异常，请先恢复'
                : '输入 prompt，Enter 发送，Shift+Enter 换行'
          }
        />
      </div>
    </div>
  )
}
