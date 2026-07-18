import { useState, useMemo, useCallback, useEffect } from 'react'
import { readSessionFile, listFileChanges, type FileChangeEntry } from '../api/filesystem'
import {
  diffLines,
  computeLineStats,
  shortPath,
  type DiffLine,
} from '../utils/diff'
import { DiffTable } from './DiffView'
import { X } from 'lucide-react'
import styles from './ChangesPanel.module.css'

interface ChangesPanelProps {
  sessionId: number
  onClose: () => void
  // refreshKey 变化时重新拉取（恢复操作后刷新）
  refreshKey?: number
}

// 带统计的改动项（基于后端 FileChangeEntry + 本地计算的增减行数）
interface ChangeItem {
  path: string
  relPath: string
  oldText: string | null
  newText: string
  isNew: boolean
  added: number
  removed: number
}

export default function ChangesPanel({ sessionId, onClose, refreshKey }: ChangesPanelProps) {
  const [rawChanges, setRawChanges] = useState<FileChangeEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedPath, setSelectedPath] = useState<string | null>(null)
  const [diskLoading, setDiskLoading] = useState(false)
  const [diskError, setDiskError] = useState('')
  const [diskText, setDiskText] = useState<string | null>(null)
  const [showDisk, setShowDisk] = useState(false)

  // 从后端拉取文件改动（基于持久化快照消息）
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    listFileChanges(sessionId)
      .then((resp) => {
        if (!cancelled) setRawChanges(resp.data.changes)
      })
      .catch(() => {
        if (!cancelled) setRawChanges([])
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [sessionId, refreshKey])

  // 计算增减行数 + 相对路径
  const changes: ChangeItem[] = useMemo(() => {
    return rawChanges.map((c) => {
      const oldText = c.is_new ? null : c.old_text
      const stats = computeLineStats(oldText, c.new_text)
      return {
        path: c.path,
        relPath: c.path, // 后端返回的已是相对 cwd 的路径
        oldText,
        newText: c.new_text,
        isNew: c.is_new,
        added: stats.added,
        removed: stats.removed,
      }
    })
  }, [rawChanges])

  const selected = useMemo(
    () => changes.find((c) => c.path === selectedPath) || null,
    [changes, selectedPath],
  )

  // 选中文件时加载磁盘内容
  const handleSelect = useCallback(
    async (path: string) => {
      setSelectedPath(path)
      setShowDisk(false)
      setDiskText(null)
      setDiskError('')
      const item = changes.find((c) => c.path === path)
      if (!item) return
      setDiskLoading(true)
      try {
        const resp = await readSessionFile(sessionId, item.relPath)
        setDiskText(resp.data.content)
      } catch (err) {
        setDiskError(err instanceof Error ? err.message : '读取文件失败')
        setDiskText(null)
      } finally {
        setDiskLoading(false)
      }
    },
    [sessionId, changes],
  )

  // 选中文件的 diff 行
  const diffLinesResult = useMemo<DiffLine[]>(() => {
    if (!selected) return []
    if (showDisk && diskText != null) {
      return diffLines(diskText, selected.newText)
    }
    return diffLines(selected.oldText, selected.newText)
  }, [selected, showDisk, diskText])

  return (
    <div className={styles.panel}>
      <div className={styles.toolbar}>
        <span className={styles.title}>
          改动文件
          {changes.length > 0 && <span className={styles.count}>{changes.length}</span>}
        </span>
        <button className={styles.closeBtn} onClick={onClose} type="button" title="关闭面板">
          <X size={16} />
        </button>
      </div>

      <div className={styles.body}>
        <div className={styles.fileList}>
          {loading && (
            <div className={styles.empty}>加载中...</div>
          )}
          {!loading && changes.length === 0 && (
            <div className={styles.empty}>本次会话暂无文件改动</div>
          )}
          {changes.map((c) => (
            <button
              key={c.path}
              type="button"
              className={`${styles.fileItem} ${selectedPath === c.path ? styles.fileItemActive : ''}`}
              onClick={() => handleSelect(c.path)}
              title={c.path}
            >
              <span className={styles.fileName}>{shortPath(c.relPath, 3)}</span>
              {c.isNew && <span className={styles.badgeNew}>新</span>}
              <span className={styles.fileStats}>
                <span className={styles.added}>+{c.added}</span>
                <span className={styles.removed}>-{c.removed}</span>
              </span>
            </button>
          ))}
        </div>

        <div className={styles.detail}>
          {!selected ? (
            <div className={styles.placeholder}>选择左侧文件查看差异</div>
          ) : (
            <>
              <div className={styles.detailHeader}>
                <span className={styles.detailPath} title={selected.path}>
                  {shortPath(selected.relPath, 4)}
                </span>
                <button
                  type="button"
                  className={styles.diskToggle}
                  onClick={() => setShowDisk((v) => !v)}
                  disabled={diskLoading || diskText == null}
                  title={
                    diskText == null
                      ? '磁盘文件不可读'
                      : showDisk
                        ? '切换为本次改动前后对比'
                        : '切换为磁盘当前内容与本次改动对比'
                  }
                >
                  {diskLoading
                    ? '读取中...'
                    : showDisk
                      ? '对比磁盘中'
                      : '对比磁盘'}
                </button>
              </div>
              {diskError && (
                <div className={styles.diskError}>磁盘读取失败：{diskError}</div>
              )}
              {showDisk && diskText != null && (
                <div className={styles.diskHint}>
                  左侧为磁盘当前内容，右侧为本次改动后的内容
                </div>
              )}
              <div className={styles.diffScroll}>
                <DiffTable lines={diffLinesResult} />
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
