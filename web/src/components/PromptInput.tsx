import { useState, useMemo, useRef, useEffect, useCallback, type FormEvent, type KeyboardEvent } from 'react'
import type { AgentCommand, SessionMode, AgentSkill } from '../types'
import { listFiles, type FileEntry } from '../api/filesystem'
import styles from './PromptInput.module.css'

interface PromptInputProps {
  onSend: (prompt: string) => void
  onCancel?: () => void
  disabled?: boolean
  sending?: boolean
  placeholder?: string
  commands?: AgentCommand[]
  /** ACP 会话模式（如 plan/act） */
  modes?: SessionMode[]
  /** Agent Skills（agentskills.io 规范） */
  skills?: AgentSkill[]
  /** 会话工作目录，用于 @ 文件引用浏览 */
  cwd?: string
}

// 补全项类型
type MentionType = 'command' | 'skill' | 'file'

// 统一补全项
interface MentionItem {
  type: MentionType
  label: string       // 显示名称
  desc: string         // 描述
  insertText: string   // 插入文本（不含触发符）
  icon: string
  isDir?: boolean      // 文件项是否为目录（目录可继续浏览）
  filePath?: string    // 文件项的完整路径
}

// 检测光标位置的触发符：返回触发类型和查询文本
function detectTrigger(text: string, cursorPos: number): { type: MentionType | null; query: string; startPos: number } {
  // 从光标向前查找最近的 / 或 @（且前面是空格或行首）
  for (let i = cursorPos - 1; i >= 0; i--) {
    const ch = text[i]
    if (ch === ' ' || ch === '\n') break
    if (ch === '/' || ch === '@') {
      // 触发符前必须是空格、行首或无内容
      if (i === 0 || text[i - 1] === ' ' || text[i - 1] === '\n') {
        return { type: ch === '/' ? 'command' : 'file', query: text.slice(i + 1, cursorPos), startPos: i }
      }
      break
    }
  }
  return { type: null, query: '', startPos: -1 }
}

export default function PromptInput({
  onSend,
  onCancel,
  disabled = false,
  sending = false,
  placeholder = '输入 prompt...',
  commands = [],
  modes = [],
  skills = [],
  cwd = '',
}: PromptInputProps) {
  const [text, setText] = useState('')
  const [selectedIdx, setSelectedIdx] = useState(0)
  const [cursorPos, setCursorPos] = useState(0)
  const [fileEntries, setFileEntries] = useState<FileEntry[]>([])
  const [fileLoading, setFileLoading] = useState(false)
  const [fileBrowsePath, setFileBrowsePath] = useState('')  // @ 文件浏览当前路径
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // 检测当前触发状态
  const trigger = useMemo(() => detectTrigger(text, cursorPos), [text, cursorPos])

  // 构建 / 菜单的补全项（commands + modes + skills）
  const slashItems = useMemo<MentionItem[]>(() => {
    if (trigger.type !== 'command') return []
    const query = trigger.query.toLowerCase()
    const items: MentionItem[] = []

    // commands
    for (const cmd of commands) {
      if (query === '' || cmd.name.toLowerCase().includes(query)) {
        items.push({
          type: 'command',
          label: `/${cmd.name}`,
          desc: cmd.description,
          insertText: `/${cmd.name} `,
          icon: '⚡',
        })
      }
    }
    // modes (ACP 会话模式)
    for (const mode of modes) {
      const name = mode.name.toLowerCase()
      const id = mode.id.toLowerCase()
      if (query === '' || name.includes(query) || id.includes(query)) {
        items.push({
          type: 'skill',
          label: mode.name,
          desc: mode.description || '模式',
          insertText: `/${mode.id} `,
          icon: '🎯',
        })
      }
    }
    // skills (agentskills.io)
    for (const skill of skills) {
      const name = skill.name.toLowerCase()
      if (query === '' || name.includes(query)) {
        items.push({
          type: 'skill',
          label: skill.name,
          desc: skill.description,
          insertText: `/${skill.name} `,
          icon: '🧩',
        })
      }
    }
    return items
  }, [trigger, commands, modes, skills])

  // @ 文件补全项
  const fileItems = useMemo<MentionItem[]>(() => {
    if (trigger.type !== 'file') return []
    const query = trigger.query.toLowerCase()
    return fileEntries
      .filter((e) => query === '' || e.name.toLowerCase().includes(query))
      .map((e) => ({
        type: 'file' as MentionType,
        label: e.name,
        desc: e.is_dir ? '目录' : '文件',
        insertText: e.is_dir ? '' : '',  // 目录不直接插入，而是浏览
        icon: e.is_dir ? '📁' : '📄',
        isDir: e.is_dir,
        filePath: e.path,
      }))
  }, [trigger, fileEntries])

  // 当前活跃的补全列表
  const activeItems = trigger.type === 'command' ? slashItems : trigger.type === 'file' ? fileItems : []
  const showMenu = trigger.type !== null && activeItems.length > 0

  // 加载文件列表（@ 触发时）
  const loadFileEntries = useCallback(async (path: string, query: string) => {
    if (!path) return
    setFileLoading(true)
    try {
      const resp = await listFiles(path, query)
      setFileEntries(resp.data.entries)
      setFileBrowsePath(resp.data.current_path)
    } catch {
      setFileEntries([])
    } finally {
      setFileLoading(false)
    }
  }, [])

  // @ 触发时加载文件（debounce）
  useEffect(() => {
    if (trigger.type !== 'file' || !cwd) {
      setFileEntries([])
      return
    }
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      loadFileEntries(cwd, trigger.query)
    }, 200)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [trigger.type, trigger.query, cwd, loadFileEntries])

  // 补全列表变化时重置选中
  useEffect(() => {
    setSelectedIdx(0)
  }, [activeItems.length])

  // 插入补全项
  function applyMention(item: MentionItem) {
    if (trigger.type === null) return
    const before = text.slice(0, trigger.startPos)
    const after = text.slice(cursorPos)

    if (item.type === 'file' && item.isDir) {
      // 目录：浏览进入该目录，不插入文本
      loadFileEntries(item.filePath || '', '')
      return
    }

    const insertText = item.type === 'file' && item.filePath
      ? `@${item.filePath} `
      : item.insertText

    const newText = before + insertText + after
    setText(newText)
    const newCursor = before.length + insertText.length
    setCursorPos(newCursor)
    requestAnimationFrame(() => {
      const el = textareaRef.current
      if (el) {
        el.focus()
        el.setSelectionRange(newCursor, newCursor)
      }
    })
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const trimmed = text.trim()
    if (!trimmed || disabled || sending) return
    onSend(trimmed)
    setText('')
    setCursorPos(0)
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    // 补全菜单导航
    if (showMenu && activeItems.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSelectedIdx((i) => (i + 1) % activeItems.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSelectedIdx((i) => (i - 1 + activeItems.length) % activeItems.length)
        return
      }
      if (e.key === 'Tab' || (e.key === 'Enter' && !e.shiftKey)) {
        e.preventDefault()
        applyMention(activeItems[selectedIdx])
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        // 清除触发符关闭菜单
        const before = text.slice(0, trigger.startPos)
        const after = text.slice(cursorPos)
        setText(before + after)
        return
      }
    }

    // 普通 Enter 发送
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit(e as unknown as FormEvent)
    }
  }

  function handleChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    setText(e.target.value)
    setCursorPos(e.target.selectionStart ?? e.target.value.length)
  }

  function handleSelect(e: React.SyntheticEvent<HTMLTextAreaElement>) {
    setCursorPos(e.currentTarget.selectionStart ?? 0)
  }

  // 菜单标题
  const menuTitle = trigger.type === 'command'
    ? '命令 / 模式（↑↓ 选择，Enter/Tab 确认）'
    : trigger.type === 'file'
      ? `文件引用 — ${fileBrowsePath || cwd}（↑↓ 选择，Enter/Tab 确认，📁 进入目录）`
      : ''
  return (
    <div className={styles.container}>
      <form className={styles.form} onSubmit={handleSubmit}>
        <div className={styles.inputWrap}>
          <textarea
            ref={textareaRef}
            className={styles.input}
            value={text}
            onChange={handleChange}
            onKeyDown={handleKeyDown}
            onSelect={handleSelect}
            onClick={handleSelect}
            placeholder={placeholder}
            disabled={disabled || sending}
            rows={1}
          />
          {showMenu && (
            <div className={styles.commandMenu}>
              <div className={styles.commandMenuHeader}>{menuTitle}</div>
              {fileLoading && <div className={styles.loadingHint}>加载中...</div>}
              {activeItems.map((item, idx) => (
                <div
                  key={`${item.type}-${item.label}-${idx}`}
                  className={`${styles.commandItem} ${idx === selectedIdx ? styles.commandItemActive : ''}`}
                  onMouseEnter={() => setSelectedIdx(idx)}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    applyMention(item)
                  }}
                >
                  <span className={styles.itemIcon}>{item.icon}</span>
                  <span className={styles.commandName}>{item.label}</span>
                  <span className={styles.commandDesc}>{item.desc}</span>
                </div>
              ))}
            </div>
          )}
        </div>
        {sending && onCancel ? (
          <button className={styles.cancelBtn} type="button" onClick={onCancel}>
            取消
          </button>
        ) : (
          <button
            className={styles.sendBtn}
            type="submit"
            disabled={disabled || sending || !text.trim()}
          >
            发送
          </button>
        )}
      </form>
    </div>
  )
}
