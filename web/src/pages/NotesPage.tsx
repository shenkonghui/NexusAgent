import { useState, useEffect, useRef, useCallback, useMemo, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { useCurrentWorkspace } from '../hooks/useCurrentWorkspace'
import { listNotes, listNoteTags, createNote, updateNote, deleteNote } from '../api/notes'
import type { Note } from '../types'
import SessionSidebar from '../components/SessionSidebar'
import UserMenu from '../components/UserMenu'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import MarkdownContent from '../components/MarkdownContent'
import { formatTimeAgo } from '../utils/time'
import styles from './NotesPage.module.css'

export default function NotesPage() {
  const { t } = useTranslation()
  const { user, loading: authLoading } = useRequireAuth()
  const { workspaceId, sessions } = useCurrentWorkspace(!!user)

  const [notes, setNotes] = useState<Note[]>([])
  const [allTags, setAllTags] = useState<string[]>([])
  const [activeTag, setActiveTag] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [input, setInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const feedRef = useRef<HTMLDivElement>(null)

  const loadNotes = useCallback(async (tag?: string) => {
    const [notesResp, tagsResp] = await Promise.all([
      listNotes(tag || undefined),
      listNoteTags(),
    ])
    setNotes(notesResp.data.notes || [])
    setAllTags(tagsResp.data.tags || [])
  }, [])

  useEffect(() => {
    if (!user) return
    setLoading(true)
    setError('')
    loadNotes(activeTag)
      .catch((err) => setError(err instanceof Error ? err.message : t('notes.loadFailed')))
      .finally(() => setLoading(false))
  }, [user, activeTag, loadNotes, t])

  const filteredNotes = useMemo(() => {
    const q = searchQuery.trim().toLowerCase()
    const list = q
      ? notes.filter(
          (n) =>
            n.title.toLowerCase().includes(q)
            || n.content.toLowerCase().includes(q)
            || n.tags.some((tag) => tag.toLowerCase().includes(q)),
        )
      : notes
    return [...list].reverse()
  }, [notes, searchQuery])

  function scrollFeedToBottom() {
    requestAnimationFrame(() => {
      const el = feedRef.current
      if (el) el.scrollTop = el.scrollHeight
    })
  }

  useEffect(() => {
    if (!loading) scrollFeedToBottom()
  }, [loading, filteredNotes.length])

  const hasPendingClassify = useMemo(
    () => notes.some((n) => n.classify_pending),
    [notes],
  )

  useEffect(() => {
    if (!user || !hasPendingClassify) return
    const timer = window.setInterval(() => {
      loadNotes(activeTag).catch(() => {})
    }, 30_000)
    return () => window.clearInterval(timer)
  }, [user, hasPendingClassify, activeTag, loadNotes])

  async function handleCreate() {
    const content = input.trim()
    if (!content || submitting) return
    setError('')
    setSubmitting(true)
    try {
      await createNote(content)
      setInput('')
      await loadNotes(activeTag)
      scrollFeedToBottom()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('notes.createFailed'))
    } finally {
      setSubmitting(false)
    }
  }

  function handleInputKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleCreate()
    }
  }

  async function handleUpdate(id: number, content: string) {
    setError('')
    try {
      await updateNote(id, content)
      await loadNotes(activeTag)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('notes.saveFailed'))
      throw err
    }
  }

  async function handleDelete(id: number) {
    if (!window.confirm(t('notes.deleteConfirm'))) return
    setError('')
    try {
      await deleteNote(id)
      await loadNotes(activeTag)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('notes.deleteFailed'))
    }
  }

  if (authLoading || !user) return <LoadingSpinner />

  return (
    <div className={styles.layout}>
      <div className={styles.sidebarWrap}>
        <SessionSidebar sessions={sessions} workspaceId={workspaceId} />
      </div>
      <div className={styles.main}>
        <header className={styles.header}>
          <h1 className={styles.title}>{t('notes.title')}</h1>
          <UserMenu />
        </header>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}
        <div className={styles.statusBar}>
          <input
            type="search"
            className={styles.searchInput}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('notes.searchPlaceholder')}
          />
          <div className={styles.tagBar}>
            <button
              type="button"
              className={`${styles.tagChip} ${activeTag === '' ? styles.tagChipActive : ''}`}
              onClick={() => setActiveTag('')}
            >
              {t('notes.allTags')}
            </button>
            {allTags.map((tag) => (
              <button
                key={tag}
                type="button"
                className={`${styles.tagChip} ${activeTag === tag ? styles.tagChipActive : ''}`}
                onClick={() => setActiveTag(tag)}
              >
                #{tag}
              </button>
            ))}
          </div>
        </div>
        {loading ? (
          <LoadingSpinner />
        ) : (
          <>
            <div className={styles.feed} ref={feedRef}>
              {filteredNotes.length === 0 ? (
                <p className={styles.empty}>{t('notes.empty')}</p>
              ) : (
                filteredNotes.map((note) => (
                  <NoteCard
                    key={note.id}
                    note={note}
                    onUpdate={(content) => handleUpdate(note.id, content)}
                    onDelete={() => handleDelete(note.id)}
                  />
                ))
              )}
            </div>
            <div className={styles.inputBar}>
              <div className={styles.inputRow}>
                <textarea
                  className={styles.input}
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={handleInputKeyDown}
                  placeholder={t('notes.quickInputPlaceholder')}
                  rows={4}
                  disabled={submitting}
                />
                <button
                  type="button"
                  className={styles.sendBtn}
                  disabled={!input.trim() || submitting}
                  onClick={handleCreate}
                >
                  {t('prompt.send')}
                </button>
              </div>
              <div className={styles.inputHint}>{t('notes.quickInputHint')}</div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function NoteCard({
  note,
  onUpdate,
  onDelete,
}: {
  note: Note
  onUpdate: (content: string) => Promise<void>
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const [editing, setEditing] = useState(false)
  const [editContent, setEditContent] = useState(note.content)
  const [saving, setSaving] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const editRef = useRef<HTMLTextAreaElement>(null)
  const [truncated, setTruncated] = useState(false)

  useEffect(() => {
    if (!editing) setEditContent(note.content)
  }, [note.content, editing])

  useEffect(() => {
    if (editing) editRef.current?.focus()
  }, [editing])

  useEffect(() => {
    const el = contentRef.current
    if (!el || expanded || editing) return
    setTruncated(el.scrollHeight > el.clientHeight + 1)
  }, [note.content, expanded, editing])

  async function handleSave() {
    const content = editContent.trim()
    if (!content || saving) return
    setSaving(true)
    try {
      await onUpdate(content)
      setEditing(false)
      setExpanded(false)
    } catch {
      // error handled by parent
    } finally {
      setSaving(false)
    }
  }

  function handleCancel() {
    setEditContent(note.content)
    setEditing(false)
  }

  function handleEditKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Escape') {
      e.preventDefault()
      handleCancel()
    }
  }

  return (
    <article className={styles.noteCard}>
      <div className={styles.noteHeader}>
        <div className={styles.noteHeaderMain}>
          <h2 className={styles.noteTitle}>{note.title}</h2>
          <div className={styles.noteMeta}>
            {note.tags.length > 0 && (
              <span className={styles.noteTags}>
                {note.tags.map((tag) => (
                  <span key={tag} className={styles.noteTag}>#{tag}</span>
                ))}
              </span>
            )}
            {note.classify_pending && (
              <span className={styles.classifyingBadge}>{t('notes.classifying')}</span>
            )}
            <time className={styles.noteTime} title={note.updated_at}>
              {formatTimeAgo(note.updated_at, t)}
            </time>
          </div>
        </div>
        <div className={styles.noteActions}>
          {!editing && (
            <button
              type="button"
              className={styles.editBtn}
              onClick={() => setEditing(true)}
              title={t('common.edit')}
            >
              ✎
            </button>
          )}
          <button
            type="button"
            className={styles.deleteBtn}
            onClick={onDelete}
            title={t('common.delete')}
            disabled={editing}
          >
            ×
          </button>
        </div>
      </div>
      {editing ? (
        <>
          <textarea
            ref={editRef}
            className={styles.editInput}
            value={editContent}
            onChange={(e) => setEditContent(e.target.value)}
            onKeyDown={handleEditKeyDown}
            rows={6}
            disabled={saving}
          />
          <div className={styles.editActions}>
            <button
              type="button"
              className={styles.editCancelBtn}
              onClick={handleCancel}
              disabled={saving}
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              className={styles.editSaveBtn}
              onClick={handleSave}
              disabled={!editContent.trim() || saving}
            >
              {saving ? t('notes.saving') : t('common.save')}
            </button>
          </div>
        </>
      ) : (
        <>
          <div
            ref={contentRef}
            className={`${styles.noteContent} markdown-body ${expanded ? '' : styles.noteContentCollapsed}`}
            onClick={() => { if (truncated || expanded) setExpanded((v) => !v) }}
            role={(truncated || expanded) ? 'button' : undefined}
            tabIndex={(truncated || expanded) ? 0 : undefined}
            onKeyDown={(e) => {
              if ((truncated || expanded) && (e.key === 'Enter' || e.key === ' ')) {
                e.preventDefault()
                setExpanded((v) => !v)
              }
            }}
          >
            <MarkdownContent content={note.content} />
          </div>
          {(truncated || expanded) && (
            <button
              type="button"
              className={styles.expandBtn}
              onClick={() => setExpanded((v) => !v)}
            >
              {expanded ? t('notes.collapse') : t('notes.expand')}
            </button>
          )}
        </>
      )}
    </article>
  )
}
