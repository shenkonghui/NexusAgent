import { useState, useEffect, useMemo, useRef, memo } from 'react'
import { useTranslation } from 'react-i18next'
import type { Message } from '../types'
import { parseDiffsFromMessage } from '../utils/diff'
import { restoreToCheckpoint } from '../api/filesystem'
import DiffView from './DiffView'
import MarkdownContent from './MarkdownContent'
import { ChevronDown, ChevronRight, MoreHorizontal } from 'lucide-react'
import styles from './MessageBubble.module.css'

interface MessageBubbleProps {
  message: Message
  defaultOpen?: boolean
  forceCollapsed?: boolean
  streaming?: boolean
  sessionId?: number
  cwd?: string
  canRestore?: boolean // 是否显示恢复按钮（仅历史用户消息）
  onRestored?: (promptText: string) => void // 恢复完成后回调，返回被恢复消息的文本
}

const kindLabels: Record<string, string> = {
  user_message_chunk: 'chat.user',
  agent_message_chunk: 'chat.assistant',
  agent_thought_chunk: 'chat.thinking',
  tool_call: 'chat.toolCall',
  tool_call_update: 'chat.toolCallUpdate',
  plan: 'chat.plan',
  usage_update: 'chat.usage',
}

// 从工具调用 content 中提取首行作为摘要，回退到默认标签
export function toolSummary(content: string): string {
  const firstLine = content.split('\n')[0]?.trim() || ''
  if (firstLine) return firstLine
  return 'chat.toolCall'
}

// 用 memo 包裹：虚拟化后仅可视区几个气泡参与比较，配合 ChatPage 稳定的 onRestored 回调，
// 历史气泡（其 message 引用在 groupMessages 后仍可能变化，但虚拟化路径下它们多不在可视区）
// 与未变化的可视区气泡可被跳过，减少 reconcile。
function MessageBubble({ message, defaultOpen = false, forceCollapsed = false, streaming = false, sessionId, cwd, canRestore, onRestored }: MessageBubbleProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(defaultOpen && !forceCollapsed)
  const [restoring, setRestoring] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  // 点击菜单外部关闭
  useEffect(() => {
    if (!menuOpen) return
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [menuOpen])

  // 检测 tool_call 消息是否携带文件 diff
  const hasDiff = useMemo(
    () => (message.kind === 'tool_call' || message.kind === 'tool_call_update')
      && parseDiffsFromMessage(message).length > 0,
    [message],
  )

  // 流式思考：进行中展开，本轮结束后强制折叠
  useEffect(() => {
    if (forceCollapsed) {
      setOpen(false)
    } else {
      setOpen(defaultOpen)
    }
  }, [defaultOpen, forceCollapsed])

  const isUser = message.role === 'user'
  const isThought = message.kind === 'agent_thought_chunk'
  const isTool = message.role === 'tool'
  const isPlan = message.kind === 'plan'
  // 助手正文消息不显示角色标签
  const showRole = !isUser && message.kind !== 'agent_message_chunk'

  // 思考和工具调用可折叠
  const collapsible = isThought || isTool

  const bubbleClass = isUser
    ? styles.userBubble
    : isThought
      ? styles.thoughtBubble
      : isTool
        ? styles.toolBubble
        : styles.assistantBubble

  const headerClick = collapsible
    ? () => setOpen((v) => !v)
    : undefined

  // 恢复到此处：确认后反向应用该消息之后所有轮次的文件改动 + 删除后续消息
  const handleRestore = async () => {
    setMenuOpen(false)
    if (!sessionId || restoring) return
    if (!window.confirm(t('chat.restoreConfirm'))) return
    setRestoring(true)
    try {
      const resp = await restoreToCheckpoint(sessionId, message.sequence)
      onRestored?.(resp.data.prompt_text || '')
    } catch {
      window.alert(t('chat.restoreFailed'))
    } finally {
      setRestoring(false)
    }
  }

  // ⋯ 菜单（用户消息上的更多操作）
  const restoreMenu = isUser && canRestore && (
    <div className={styles.menuWrap} ref={menuRef}>
      <button
        className={styles.menuTrigger}
        onClick={(e) => {
          e.stopPropagation()
          setMenuOpen((v) => !v)
        }}
        disabled={restoring}
        type="button"
        title={t('common.more')}
      >
        <MoreHorizontal size={14} />
      </button>
      {menuOpen && (
        <div className={styles.menuDropdown} onClick={(e) => e.stopPropagation()}>
          <button
            className={styles.menuItem}
            onClick={() => handleRestore()}
            disabled={restoring}
            type="button"
          >
            {restoring ? t('chat.restoring') : t('chat.restoreHere')}
          </button>
        </div>
      )}
    </div>
  )

  return (
    <div className={`${styles.container} ${isUser ? styles.containerUser : ''}`}>
      <div className={`${styles.bubble} ${bubbleClass}`}>
        <div
          className={`${styles.header} ${collapsible ? styles.headerClickable : ''}`}
          onClick={headerClick}
        >
          {showRole && <span className={styles.role}>{t(kindLabels[message.kind] || message.role)}</span>}
          {isPlan && <span className={styles.badge}>{t('chat.plan')}</span>}
          {isTool && (
            <span className={styles.summary}>{t(toolSummary(message.content))}</span>
          )}
          {collapsible && (
            <span className={styles.toggle}>{open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}</span>
          )}
        </div>
        {!collapsible || open ? (
          <>
            {isUser ? (
              <div className={styles.inlineRow}>
                {message.content ? (
                  <div className={styles.content}>{message.content}</div>
                ) : (
                  !isPlan && <div className={styles.contentMuted}>{t('common.noData')}</div>
                )}
                {restoreMenu}
              </div>
            ) : (
              <>
                {message.content && (
                  message.kind === 'agent_message_chunk' && !streaming ? (
                    <MarkdownContent
                      content={message.content}
                      className={`markdown-body ${styles.mdContent}`}
                    />
                  ) : (
                    <div className={styles.content}>{message.content}</div>
                  )
                )}
                {!message.content && !isPlan && (
                  <div className={styles.contentMuted}>{t('common.noData')}</div>
                )}
              </>
            )}
            {hasDiff && sessionId != null && cwd != null && (
              <DiffView
                message={message}
                sessionId={sessionId}
                cwd={cwd}
                defaultExpanded={defaultOpen && !forceCollapsed}
              />
            )}
          </>
        ) : null}
      </div>
    </div>
  )
}

export default memo(MessageBubble)
