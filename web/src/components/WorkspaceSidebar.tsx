import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Workspace } from '../types'
import styles from './WorkspaceSidebar.module.css'

interface WorkspaceSidebarProps {
  workspaces: (Workspace & { session_count?: number })[]
  currentId?: number
  onDelete: (id: number) => void
  onRename: (id: number, name: string) => void
  onSave: (id: number) => void
  onCreateClick: () => void
}

export default function WorkspaceSidebar({
  workspaces,
  currentId,
  onDelete,
  onRename,
  onSave,
  onCreateClick,
}: WorkspaceSidebarProps) {
  const [contextMenu, setContextMenu] = useState<{ id: number; x: number; y: number } | null>(null)
  const [renaming, setRenaming] = useState<number | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const navigate = useNavigate()

  function handleRenameStart(ws: Workspace) {
    setRenaming(ws.id)
    setRenameValue(ws.name)
    setContextMenu(null)
  }

  function handleRenameSubmit(id: number) {
    if (renameValue.trim()) onRename(id, renameValue.trim())
    setRenaming(null)
  }

  return (
    <div className={styles.sidebar}>
      <div className={styles.header}>
        <span className={styles.title}>工作区</span>
        <button className={styles.newBtn} onClick={onCreateClick} title="新建工作区">+</button>
      </div>
      <div className={styles.list}>
        {workspaces.map((ws) => (
          <div
            key={ws.id}
            className={`${styles.item} ${ws.id === currentId ? styles.active : ''}`}
            onClick={() => navigate(`/workspaces/${ws.id}`)}
            onContextMenu={(e) => { e.preventDefault(); setContextMenu({ id: ws.id, x: e.clientX, y: e.clientY }) }}
          >
            <span className={styles.icon}>{ws.mode === 'temporary' ? '🕐' : '📁'}</span>
            {renaming === ws.id ? (
              <input className={styles.renameInput} value={renameValue}
                onChange={e => setRenameValue(e.target.value)}
                onBlur={() => handleRenameSubmit(ws.id)}
                onKeyDown={e => { if (e.key === 'Enter') handleRenameSubmit(ws.id); if (e.key === 'Escape') setRenaming(null) }}
                autoFocus onClick={e => e.stopPropagation()}
              />
            ) : (
              <span className={styles.name}>{ws.name}</span>
            )}
            {ws.session_count !== undefined && (
              <span className={styles.count}>{ws.session_count}</span>
            )}
          </div>
        ))}
      </div>
      {contextMenu && (
        <div className={styles.contextMenu} style={{ top: contextMenu.y, left: contextMenu.x }}>
          <div className={styles.menuItem} onClick={() => {
            const ws = workspaces.find(w => w.id === contextMenu.id)
            if (ws) handleRenameStart(ws)
          }}>重命名</div>
          <div className={styles.menuItem} onClick={() => {
            const ws = workspaces.find(w => w.id === contextMenu.id)
            if (ws?.mode === 'temporary') onSave(contextMenu.id)
            setContextMenu(null)
          }}>保存为正式工作区</div>
          <div className={`${styles.menuItem} ${styles.danger}`} onClick={() => {
            onDelete(contextMenu.id); setContextMenu(null)
          }}>删除</div>
        </div>
      )}
    </div>
  )
}
