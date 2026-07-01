import { useState, useMemo, useRef, useEffect, useCallback, type FormEvent, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { AgentCommand, SessionMode, AgentSkill } from '../types'
import { listFiles, type FileEntry } from '../api/filesystem'
import styles from './PromptInput.module.css'

interface PromptInputProps {
  onSend: (prompt: string) => void
  onCancel?: () => void
  disabled?: boolean
  sending?: boolean
  placeholder?: string
  embedded?: boolean
  value?: string
  onValueChange?: (text: string) => void
  rows?: number
  commands?: AgentCommand[]
  modes?: SessionMode[]
  skills?: AgentSkill[]
  cwd?: string
}

type MentionType = 'command' | 'skill' | 'mode' | 'file'

interface MentionItem {
  type: MentionType
  label: string
  desc: string
  insertText: string
  path: string
  kindLabel: string
  isDir?: boolean
  filePath?: string
}

function detectTrigger(text: string, cursorPos: number): { type: MentionType | null; query: string; startPos: number } {
  for (let i = cursorPos - 1; i >= 0; i--) {
    const ch = text[i]
    if (ch === ' ' || ch === '\n') break
    if (ch === '/' || ch === '@') {
      if (i === 0 || text[i - 1] === ' ' || text[i - 1] === '\n') {
        return { type: ch === '/' ? 'command' : 'file', query: text.slice(i + 1, cursorPos), startPos: i }
      }
      break
    }
  }
  return { type: null, query: '', startPos: -1 }
}

function matchesSlashQuery(fields: string[], query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  const tokens = q.split('/').filter(Boolean)
  const haystack = fields.join(' ').toLowerCase()
  return tokens.every((token) => haystack.includes(token))
}

export default function PromptInput({
  onSend,
  onCancel,
  disabled = false,
  sending = false,
  placeholder = '输入 prompt...',
  embedded = false,
  value: controlledValue,
  onValueChange,
  rows = 1,
  commands = [],
  modes = [],
  skills = [],
  cwd = '',
}: PromptInputProps) {
  const { t } = useTranslation()
  const [internalText, setInternalText] = useState('')
  const isControlled = controlledValue !== undefined
  const text = isControlled ? controlledValue : internalText
  const setText = (next: string) => {
    if (isControlled) onValueChange?.(next)
    else setInternalText(next)
  }
  const [selectedIdx, setSelectedIdx] = useState(0)
  const [cursorPos, setCursorPos] = useState(0)
  const [fileEntries, setFileEntries] = useState<FileEntry[]>([])
  const [fileLoading, setFileLoading] = useState(false)
  const [fileBrowsePath, setFileBrowsePath] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const trigger = useMemo(() => detectTrigger(text, cursorPos), [text, cursorPos])

  const slashItems = useMemo<MentionItem[]>(() => {
    if (trigger.type !== 'command') return []
    const query = trigger.query
    const items: MentionItem[] = []

    for (const cmd of commands) {
      const path = cmd.path || cmd.name
      const kindLabel = cmd.kind === 'agent' ? t('prompt.slashKindAgent') : t('prompt.slashKindCommand')
      if (!matchesSlashQuery([cmd.name, path, cmd.description, kindLabel], query)) continue
      items.push({
        type: 'command',
        label: path.includes('/') ? path : `/${cmd.name}`,
        path,
        desc: cmd.description,
        insertText: `/${cmd.name} `,
        kindLabel,
      })
    }

    for (const mode of modes) {
      const path = mode.id
      if (!matchesSlashQuery([mode.name, mode.id, mode.description || '', t('prompt.slashKindMode')], query)) continue
      items.push({
        type: 'mode',
        label: mode.name,
        path,
        desc: mode.description || t('prompt.slashKindMode'),
        insertText: `/${mode.id} `,
        kindLabel: t('prompt.slashKindMode'),
      })
    }

    for (const skill of skills) {
      const path = skill.path || skill.name
      if (!matchesSlashQuery([skill.name, path, skill.description, t('prompt.slashKindSkill')], query)) continue
      items.push({
        type: 'skill',
        label: path.includes('/') ? path : skill.name,
        path,
        desc: skill.description,
        insertText: `/${skill.name} `,
        kindLabel: t('prompt.slashKindSkill'),
      })
    }

    items.sort((a, b) => {
      const kindOrder = { command: 0, skill: 1, mode: 2, file: 3 }
      const ka = kindOrder[a.type] - kindOrder[b.type]
      if (ka !== 0) return ka
      return a.path.localeCompare(b.path)
    })
    return items
  }, [trigger, commands, modes, skills, t])

  const fileItems = useMemo<MentionItem[]>(() => {
    if (trigger.type !== 'file') return []
    const query = trigger.query.toLowerCase()
    return fileEntries
      .filter((e) => query === '' || e.name.toLowerCase().includes(query))
      .map((e) => ({
        type: 'file' as MentionType,
        label: e.name,
        path: e.name,
        desc: e.is_dir ? t('prompt.fileDir') : t('prompt.fileFile'),
        insertText: '',
        kindLabel: t('prompt.slashKindFile'),
        isDir: e.is_dir,
        filePath: e.path,
      }))
  }, [trigger, fileEntries, t])

  const activeItems = trigger.type === 'command' ? slashItems : trigger.type === 'file' ? fileItems : []
  const showMenu = trigger.type === 'command' || (trigger.type === 'file' && !!cwd)

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
    if (trigger.type !== 'file') setFileBrowsePath('')
  }, [trigger.type])

  useEffect(() => {
    if (trigger.type !== 'file' || !cwd) {
      setFileEntries([])
      return
    }
    const basePath = fileBrowsePath || cwd
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      loadFileEntries(basePath, trigger.query)
    }, 200)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [trigger.type, trigger.query, cwd, fileBrowsePath, loadFileEntries])

  useEffect(() => {
    setSelectedIdx(0)
  }, [activeItems.length, trigger.query])

  function applyMention(item: MentionItem) {
    if (trigger.type === null) return
    const before = text.slice(0, trigger.startPos)
    const after = text.slice(cursorPos)

    if (item.type === 'file' && item.isDir) {
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
    if (!isControlled) {
      setInternalText('')
      setCursorPos(0)
    }
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
    }

    if (showMenu && e.key === 'Escape') {
      e.preventDefault()
      const before = text.slice(0, trigger.startPos)
      const after = text.slice(cursorPos)
      setText(before + after)
      return
    }

    if (e.key === 'Enter' && !e.shiftKey && !embedded) {
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
    : trigger.type === 'file'
      ? t('prompt.fileMenuTitle', { path: fileBrowsePath || cwd })
      : ''

  return (
    <div className={`${styles.container} ${embedded ? styles.embedded : ''}`}>
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
            rows={rows}
          />
          {showMenu && (
            <div className={styles.commandMenu}>
              <div className={styles.commandMenuHeader}>{menuTitle}</div>
              {fileLoading && <div className={styles.loadingHint}>{t('common.loading')}</div>}
              {!fileLoading && activeItems.length === 0 && (
                <div className={styles.loadingHint}>{t('prompt.slashNoMatch')}</div>
              )}
              {activeItems.map((item, idx) => (
                <div
                  key={`${item.type}-${item.path}-${idx}`}
                  className={`${styles.commandItem} ${idx === selectedIdx ? styles.commandItemActive : ''}`}
                  onMouseEnter={() => setSelectedIdx(idx)}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    applyMention(item)
                  }}
                >
                  <span className={`${styles.itemKind} ${styles[`kind_${item.type}`]}`}>{item.kindLabel}</span>
                  <div className={styles.itemMain}>
                    <span className={styles.commandName}>{item.label}</span>
                  </div>
                  <span className={styles.commandDesc}>{item.desc}</span>
                </div>
              ))}
            </div>
          )}
        </div>
        {!embedded && (sending && onCancel ? (
          <button className={styles.cancelBtn} type="button" onClick={onCancel}>
            {t('session.cancelPrompt')}
          </button>
        ) : (
          <button
            className={styles.sendBtn}
            type="submit"
            disabled={disabled || sending || !text.trim()}
          >
            {t('prompt.send')}
          </button>
        ))}
      </form>
    </div>
  )
}
