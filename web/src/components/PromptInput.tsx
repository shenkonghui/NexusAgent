import { useState, useMemo, useRef, useEffect, useCallback, type FormEvent, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { AgentCommand, SessionMode, AgentSkill, ScheduledTask } from '../types'
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
  /** 定时任务列表，用于 @task 引用 */
  tasks?: ScheduledTask[]
  /** 会话工作目录，用于 @file 文件引用浏览 */
  cwd?: string
}

type SlashTrigger = 'command'
type AtTrigger = 'mention'
type TriggerType = SlashTrigger | AtTrigger | null
type MentionKind = 'skill' | 'command' | 'file' | 'task'
type AtCategory = 'all' | MentionKind

interface MentionItem {
  kind: MentionKind | 'mode'
  label: string
  desc: string
  insertText: string
  icon: string
  isDir?: boolean
  filePath?: string
}

function detectTrigger(text: string, cursorPos: number): { type: TriggerType; query: string; startPos: number } {
  for (let i = cursorPos - 1; i >= 0; i--) {
    const ch = text[i]
    if (ch === ' ' || ch === '\n') break
    if (ch === '/' || ch === '@') {
      if (i === 0 || text[i - 1] === ' ' || text[i - 1] === '\n') {
        return {
          type: ch === '/' ? 'command' : 'mention',
          query: text.slice(i + 1, cursorPos),
          startPos: i,
        }
      }
      break
    }
  }
  return { type: null, query: '', startPos: -1 }
}

function parseAtQuery(query: string): { category: AtCategory; term: string } {
  const trimmed = query.trimStart()
  const colonIdx = trimmed.indexOf(':')
  if (colonIdx > 0) {
    const prefix = trimmed.slice(0, colonIdx).toLowerCase()
    if (prefix === 'skill' || prefix === 'command' || prefix === 'file' || prefix === 'task') {
      return { category: prefix, term: trimmed.slice(colonIdx + 1).toLowerCase() }
    }
  }
  const lower = trimmed.toLowerCase()
  if (lower === 'skill' || lower === 'command' || lower === 'file' || lower === 'task') {
    return { category: lower, term: '' }
  }
  return { category: 'all', term: lower }
}

function includesTerm(value: string, term: string) {
  return term === '' || value.toLowerCase().includes(term)
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
  tasks = [],
  cwd = '',
}: PromptInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [selectedIdx, setSelectedIdx] = useState(0)
  const [cursorPos, setCursorPos] = useState(0)
  const [fileEntries, setFileEntries] = useState<FileEntry[]>([])
  const [fileLoading, setFileLoading] = useState(false)
  const [fileBrowsePath, setFileBrowsePath] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const trigger = useMemo(() => detectTrigger(text, cursorPos), [text, cursorPos])
  const atQuery = useMemo(
    () => (trigger.type === 'mention' ? parseAtQuery(trigger.query) : { category: 'all' as AtCategory, term: '' }),
    [trigger.type, trigger.query],
  )

  const slashItems = useMemo<MentionItem[]>(() => {
    if (trigger.type !== 'command') return []
    const query = trigger.query.toLowerCase()
    const items: MentionItem[] = []

    for (const cmd of commands) {
      if (query === '' || cmd.name.toLowerCase().includes(query)) {
        items.push({
          kind: 'command',
          label: `/${cmd.name}`,
          desc: cmd.description,
          insertText: `/${cmd.name} `,
          icon: '⚡',
        })
      }
    }
    for (const mode of modes) {
      const name = mode.name.toLowerCase()
      const id = mode.id.toLowerCase()
      if (query === '' || name.includes(query) || id.includes(query)) {
        items.push({
          kind: 'mode',
          label: mode.name,
          desc: mode.description || t('chat.mode'),
          insertText: `/${mode.id} `,
          icon: '🎯',
        })
      }
    }
    for (const skill of skills) {
      const name = skill.name.toLowerCase()
      if (query === '' || name.includes(query)) {
        items.push({
          kind: 'skill',
          label: skill.name,
          desc: skill.description,
          insertText: `/${skill.name} `,
          icon: '🧩',
        })
      }
    }
    return items
  }, [trigger, commands, modes, skills, t])

  const atItems = useMemo<MentionItem[]>(() => {
    if (trigger.type !== 'mention') return []
    const { category, term } = atQuery
    const items: MentionItem[] = []
    const showFiles = category === 'file' || (category === 'all' && term !== '')

    if (category === 'all' && term === '') {
      items.push(
        { kind: 'skill', label: t('prompt.atSkill'), desc: t('prompt.atSkillHint'), insertText: '@skill:', icon: '🧩' },
        { kind: 'command', label: t('prompt.atCommand'), desc: t('prompt.atCommandHint'), insertText: '@command:', icon: '⚡' },
        { kind: 'file', label: t('prompt.atFile'), desc: t('prompt.atFileHint'), insertText: '@file:', icon: '📄' },
        { kind: 'task', label: t('prompt.atTask'), desc: t('prompt.atTaskHint'), insertText: '@task:', icon: '📋' },
      )
    }

    if (category === 'skill' || category === 'all') {
      for (const skill of skills) {
        if (includesTerm(skill.name, term)) {
          items.push({
            kind: 'skill',
            label: skill.name,
            desc: skill.description,
            insertText: `@skill:${skill.name} `,
            icon: '🧩',
          })
        }
      }
    }

    if (category === 'command' || category === 'all') {
      for (const cmd of commands) {
        if (includesTerm(cmd.name, term)) {
          items.push({
            kind: 'command',
            label: cmd.name,
            desc: cmd.description,
            insertText: `@command:${cmd.name} `,
            icon: '⚡',
          })
        }
      }
    }

    if (category === 'task' || category === 'all') {
      for (const task of tasks) {
        if (includesTerm(task.name, term)) {
          items.push({
            kind: 'task',
            label: task.name,
            desc: task.prompt.length > 60 ? `${task.prompt.slice(0, 59)}…` : task.prompt,
            insertText: `@task:${task.name} `,
            icon: '📋',
          })
        }
      }
    }

    if (showFiles) {
      const fileTerm = category === 'file' ? term : term
      for (const entry of fileEntries) {
        if (includesTerm(entry.name, fileTerm)) {
          items.push({
            kind: 'file',
            label: entry.name,
            desc: entry.is_dir ? t('prompt.directory') : t('prompt.fileItem'),
            insertText: '',
            icon: entry.is_dir ? '📁' : '📄',
            isDir: entry.is_dir,
            filePath: entry.path,
          })
        }
      }
    }

    return items
  }, [trigger.type, atQuery, skills, commands, tasks, fileEntries, t])

  const activeItems = trigger.type === 'command' ? slashItems : trigger.type === 'mention' ? atItems : []
  const showMenu = trigger.type !== null && activeItems.length > 0

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

  useEffect(() => {
    if (trigger.type !== 'mention' || !cwd) {
      setFileEntries([])
      return
    }
    const { category } = atQuery
    if (category !== 'file' && !(category === 'all' && atQuery.term !== '')) {
      setFileEntries([])
      return
    }
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      const term = category === 'file' ? atQuery.term : ''
      loadFileEntries(fileBrowsePath || cwd, term)
    }, 200)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [trigger.type, atQuery, cwd, fileBrowsePath, loadFileEntries])

  useEffect(() => {
    if (trigger.type !== 'mention') {
      setFileBrowsePath('')
    }
  }, [trigger.type])

  useEffect(() => {
    setSelectedIdx(0)
  }, [activeItems.length, atQuery.category, atQuery.term])

  function applyMention(item: MentionItem) {
    if (trigger.type === null) return
    const before = text.slice(0, trigger.startPos)
    const after = text.slice(cursorPos)

    if (item.kind === 'file' && item.isDir) {
      loadFileEntries(item.filePath || '', '')
      return
    }

    const insertText = item.kind === 'file' && item.filePath
      ? `@file:${item.filePath} `
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
        const before = text.slice(0, trigger.startPos)
        const after = text.slice(cursorPos)
        setText(before + after)
        return
      }
    }

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

  const menuTitle = trigger.type === 'command'
    ? t('prompt.slashMenuTitle')
    : trigger.type === 'mention'
      ? atQuery.category === 'file'
        ? t('prompt.atFileMenuTitle', { path: fileBrowsePath || cwd })
        : t('prompt.atMenuTitle')
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
              {fileLoading && <div className={styles.loadingHint}>{t('common.loading')}</div>}
              {activeItems.map((item, idx) => (
                <div
                  key={`${item.kind}-${item.label}-${idx}`}
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
            {t('session.cancelPrompt')}
          </button>
        ) : (
          <button
            className={styles.sendBtn}
            type="submit"
            disabled={disabled || sending || !text.trim()}
          >
            {t('common.send')}
          </button>
        )}
      </form>
    </div>
  )
}
