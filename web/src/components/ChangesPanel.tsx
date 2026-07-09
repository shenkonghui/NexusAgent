import { useState, useMemo, useCallback } from 'react'
import type { Message } from '../types'
import { readSessionFile } from '../api/filesystem'
import {
  parseDiffsFromMessage,
  diffLines,
  computeLineStats,
  toRelativePath,
  shortPath,
  type FileDiff,
  type DiffLine,
} from '../utils/diff'
import { DiffTable } from './DiffView'
import { X } from 'lucide-react'
import styles from './ChangesPanel.module.css'

interface ChangesPanelProps {
  messages: Message[]
  sessionId: number
  cwd: string
  onClose: () => void
}

// 聚合后的文件改动项
interface FileChangeItem {
  path: string // 原始路径
  relPath: string // 相对 cwd
  diff: FileDiff // 最新一次 diff
  added: number
  removed: number
  isNew: boolean
}

// aggregateChanges 遍历所有消息，按文件路径去重，保留最新一次 diff。
function aggregateChanges(messages: Message[], cwd: string): FileChangeItem[] {
  const map = new Map<string, FileChangeItem>()
  for (const msg of messages) {
    const diffs = parseDiffsFromMessage(msg)
    for (const d of diffs) {
      const relPath = toRelativePath(d.path, cwd)
      const stats = computeLineStats(d.oldText, d.newText)
      map.set(d.path, {
        path: d.path,
        relPath,
        diff: d,
        added: stats.added,
        removed: stats.removed,
        isNew: d.oldText == null,
      })
    }
  }
  return Array.from(map.values())
}

export default function ChangesPanel({ messages, sessionId, cwd, onClose }: ChangesPanelProps) {
  const changes = useMemo(() => aggregateChanges(messages, cwd), [messages, cwd])
  const [selectedPath, setSelectedPath] = useState<string | null>(null)
  const [diskLoading, setDiskLoading] = useState(false)
  const [diskError, setDiskError] = useState('')
  const [diskText, setDiskText] = useState<string | null>(null)
  const [showDisk, setShowDisk] = useState(false)

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

  // 选中文件的 diff 行（默认对比 oldText->newText；开启磁盘对比后用 diskText->newText）
  const diffLinesResult = useMemo<DiffLine[]>(() => {
    if (!selected) return []
    if (showDisk && diskText != null) {
      return diffLines(diskText, selected.diff.newText)
    }
    return diffLines(selected.diff.oldText, selected.diff.newText)
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
          {changes.length === 0 && (
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
