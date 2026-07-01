import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { listWorkspaces, createWorkspace, deleteWorkspace, updateWorkspace, saveWorkspace } from '../api/workspaces'
import type { Workspace } from '../types'
import CreateWorkspaceDialog from './CreateWorkspaceDialog'
import styles from './WorkspaceSelector.module.css'

interface Props {
  value: number
  onChange: (id: number) => void
  onRefresh?: () => void
  onError?: (message: string) => void
}

export default function WorkspaceSelector({ value, onChange, onRefresh, onError }: Props) {
  const { t } = useTranslation()
  const [workspaces, setWorkspaces] = useState<(Workspace & { session_count?: number })[]>([])
  const [open, setOpen] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const [contextMenu, setContextMenu] = useState<{ id: number; x: number; y: number } | null>(null)
  const [renaming, setRenaming] = useState<number | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const ref = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const current = workspaces.find((w) => w.id === value)

  async function loadWorkspaces() {
    const list = (await listWorkspaces()).data.workspaces || []
    setWorkspaces(list)
    onRefresh?.()
    return list
  }

  useEffect(() => { loadWorkspaces().catch((e) => onError?.(e instanceof Error ? e.message : t('common.failed'))) }, [])

  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  useEffect(() => {
    if (!contextMenu) return
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setContextMenu(null)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [contextMenu])

  async function handleSelect(id: number) {
    onChange(id)
    setOpen(false)
  }

  async function handleCreate(name: string, cwd: string) {
    try {
      const resp = await createWorkspace(name, cwd)
      await loadWorkspaces()
      onChange(resp.data.id)
      setShowCreate(false)
    } catch (e) {
      onError?.(e instanceof Error ? e.message : t('common.failed'))
    }
  }

  async function handleDelete(id: number) {
    if (!window.confirm(t('workspace.deleteConfirm'))) return
    try {
      await deleteWorkspace(id)
      const list = await loadWorkspaces()
      if (id === value) onChange(list[0]?.id ?? 0)
      setContextMenu(null)
    } catch (e) {
      onError?.(e instanceof Error ? e.message : t('common.failed'))
    }
  }

  async function handleRenameSubmit(id: number) {
    const name = renameValue.trim()
    if (!name) { setRenaming(null); return }
    try {
      await updateWorkspace(id, name)
      await loadWorkspaces()
    } catch (e) {
      onError?.(e instanceof Error ? e.message : t('common.failed'))
    }
    setRenaming(null)
  }

  return (
    <div className={styles.container} ref={ref}>
      <button type="button" className={styles.trigger} onClick={() => setOpen((v) => !v)} title={t('workspace.title')}>
        <span className={styles.icon}>{current?.mode === 'temporary' ? '🕐' : '📁'}</span>
        <span className={styles.label}>{current?.name || t('workspace.default')}</span>
        <span className={styles.arrow}>{open ? '▲' : '▼'}</span>
      </button>
      <button type="button" className={styles.newBtn} onClick={() => setShowCreate(true)} title={t('workspace.create')}>+</button>

      {open && (
        <div className={styles.dropdown}>
          {workspaces.length === 0 ? (
            <div className={styles.item}><span className={styles.itemName}>{t('workspace.empty')}</span></div>
          ) : workspaces.map((ws) => (
            <div key={ws.id}
              className={`${styles.item} ${ws.id === value ? styles.itemActive : ''}`}
              onClick={() => renaming !== ws.id && handleSelect(ws.id)}
            >
              <span>{ws.mode === 'temporary' ? '🕐' : '📁'}</span>
              {renaming === ws.id ? (
                <input className={styles.renameInput} value={renameValue} autoFocus
                  onChange={(e) => setRenameValue(e.target.value)}
                  onBlur={() => handleRenameSubmit(ws.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleRenameSubmit(ws.id)
                    if (e.key === 'Escape') setRenaming(null)
                  }}
                  onClick={(e) => e.stopPropagation()}
                />
              ) : (
                <span className={styles.itemName}>{ws.name}</span>
              )}
              {ws.session_count !== undefined && <span className={styles.itemCount}>{ws.session_count}</span>}
              <button type="button" className={styles.menuBtn}
                onClick={(e) => { e.stopPropagation(); setContextMenu({ id: ws.id, x: e.clientX, y: e.clientY }) }}
              >⋯</button>
            </div>
          ))}
        </div>
      )}

      {contextMenu && (
        <div ref={menuRef} className={styles.contextMenu} style={{ top: contextMenu.y, left: contextMenu.x }}>
          <div className={styles.menuItem} onClick={() => {
            const ws = workspaces.find((w) => w.id === contextMenu.id)
            if (ws) { setRenaming(ws.id); setRenameValue(ws.name) }
            setContextMenu(null)
          }}>{t('workspace.rename')}</div>
          <div className={styles.menuItem} onClick={async () => {
            const ws = workspaces.find((w) => w.id === contextMenu.id)
            if (ws?.mode === 'temporary') {
              try { await saveWorkspace(contextMenu.id, ws.name, ws.cwd); await loadWorkspaces() }
              catch (e) { onError?.(e instanceof Error ? e.message : t('common.failed')) }
            }
            setContextMenu(null)
          }}>{t('workspace.save')}</div>
          <div className={`${styles.menuItem} ${styles.danger}`}
            onClick={() => handleDelete(contextMenu.id)}
          >{t('workspace.delete')}</div>
        </div>
      )}

      {showCreate && (
        <CreateWorkspaceDialog onSubmit={handleCreate} onClose={() => setShowCreate(false)} />
      )}
    </div>
  )
}
