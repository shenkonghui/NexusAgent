import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  diffLines,
  shortPath,
  type FileChangeItem,
  type DiffLine,
} from '../utils/diff'
import { undoFileChanges } from '../api/filesystem'
import { DiffTable } from './DiffView'
import { FileText, ChevronDown, ChevronRight, Undo2 } from 'lucide-react'
import styles from './ChangesSummary.module.css'

interface ChangesSummaryProps {
  changes: FileChangeItem[]
  sessionId?: number
  cwd?: string
  messageId?: number // 快照消息 id，用于撤销
}

// 每轮对话末尾的文件改动汇总卡片。
// 两级展开：0 级显示文件总数 + 总增减行数；1 级展开文件列表；2 级展开单个文件 diff。
// 右侧撤销按钮可恢复该轮 prompt 修改的文件到修改前状态。
export default function ChangesSummary({ changes, sessionId, messageId }: ChangesSummaryProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [openFile, setOpenFile] = useState<string | null>(null)
  const [undoing, setUndoing] = useState(false)
  const [undoMsg, setUndoMsg] = useState('')

  // 总增减行数（基于去重后的最新 diff）
  const totals = useMemo(() => {
    let added = 0
    let removed = 0
    for (const c of changes) {
      added += c.added
      removed += c.removed
    }
    return { added, removed }
  }, [changes])

  // 当前展开文件的 diff 行（懒计算）
  const expandedLines = useMemo<DiffLine[]>(() => {
    if (!openFile) return []
    const item = changes.find((c) => c.path === openFile)
    if (!item) return []
    return diffLines(item.diff.oldText, item.diff.newText)
  }, [openFile, changes])

  if (changes.length === 0) return null

  const toggleFile = (path: string) => {
    setOpenFile((prev) => (prev === path ? null : path))
  }

  const handleUndo = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (!sessionId || !messageId || undoing) return
    setUndoing(true)
    setUndoMsg('')
    try {
      const resp = await undoFileChanges(sessionId, messageId)
      setUndoMsg(t('chat.undoSuccess', { restored: resp.data.restored, deleted: resp.data.deleted }))
    } catch {
      setUndoMsg(t('chat.undoFailed'))
    } finally {
      setUndoing(false)
    }
  }

  return (
    <div className={styles.card}>
      <div
        className={`${styles.header} ${open ? styles.headerOpen : ''}`}
        onClick={() => setOpen((v) => !v)}
        role="button"
        tabIndex={0}
      >
        <span className={styles.iconWrap}>
          <FileText size={14} />
        </span>
        <span className={styles.title}>
          {t('chat.filesChanged', { count: changes.length })}
        </span>
        <span className={styles.totalStats}>
          <span className={styles.added}>+{totals.added}</span>{' '}
          <span className={styles.removed}>-{totals.removed}</span>
        </span>
        {sessionId && messageId && (
          <button
            type="button"
            className={styles.undoBtn}
            onClick={handleUndo}
            disabled={undoing}
            title={t('chat.undo')}
          >
            <Undo2 size={13} />
            {undoing ? '' : t('chat.undo')}
          </button>
        )}
        <span className={styles.arrow}>
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
      </div>

      {undoMsg && (
        <div className={styles.undoMsg}>{undoMsg}</div>
      )}

      {open && (
        <div className={styles.fileList}>
          {changes.map((c) => {
            const expanded = openFile === c.path
            return (
              <div key={c.path} className={styles.fileEntry}>
                <div
                  className={`${styles.fileRow} ${expanded ? styles.fileRowActive : ''}`}
                  onClick={() => toggleFile(c.path)}
                  role="button"
                  tabIndex={0}
                  title={c.path}
                >
                  <span className={styles.fileArrow}>
                    {expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                  </span>
                  <span className={styles.filePath}>{shortPath(c.relPath, 3)}</span>
                  {c.isNew && <span className={styles.badgeNew}>{t('chat.newFile')}</span>}
                  <span className={styles.fileStats}>
                    <span className={styles.added}>+{c.added}</span>
                    <span className={styles.removed}>-{c.removed}</span>
                  </span>
                </div>
                {expanded && (
                  <div className={styles.fileDiff}>
                    <DiffTable lines={expandedLines} />
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
