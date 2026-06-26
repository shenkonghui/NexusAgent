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
import styles from './DiffView.module.css'

interface DiffViewProps {
  message: Message
  sessionId: number
  cwd: string
  // 折叠态默认是否展开（流式进行中可传 true）
  defaultExpanded?: boolean
}

// 单个文件 diff 卡片
function FileDiffCard({
  diff,
  sessionId,
  cwd,
  defaultExpanded,
}: {
  diff: FileDiff
  sessionId: number
  cwd: string
  defaultExpanded: boolean
}) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  const [diskLoading, setDiskLoading] = useState(false)
  const [diskError, setDiskError] = useState('')
  const [diskText, setDiskText] = useState<string | null>(null)
  const [showDisk, setShowDisk] = useState(false)

  const stats = useMemo(() => computeLineStats(diff.oldText, diff.newText), [diff])
  const lines = useMemo<DiffLine[]>(
    () => (expanded ? diffLines(diff.oldText, diff.newText) : []),
    [expanded, diff],
  )

  const relPath = useMemo(() => toRelativePath(diff.path, cwd), [diff.path, cwd])
  const isNewFile = diff.oldText == null

  // 加载磁盘当前内容，与 newText 对比
  const loadDisk = useCallback(async () => {
    setDiskLoading(true)
    setDiskError('')
    try {
      const resp = await readSessionFile(sessionId, relPath)
      setDiskText(resp.data.content)
      setShowDisk(true)
    } catch (err) {
      setDiskError(err instanceof Error ? err.message : '读取文件失败')
      setDiskText(null)
    } finally {
      setDiskLoading(false)
    }
  }, [sessionId, relPath])

  // 磁盘内容 vs newText 的行级 diff
  const diskLines = useMemo<DiffLine[]>(() => {
    if (!showDisk || diskText == null) return []
    return diffLines(diskText, diff.newText)
  }, [showDisk, diskText, diff.newText])

  return (
    <div className={styles.fileCard}>
      <div
        className={styles.fileHeader}
        onClick={() => setExpanded((v) => !v)}
        role="button"
        tabIndex={0}
      >
        <span className={styles.arrow}>{expanded ? '▾' : '▸'}</span>
        <span className={styles.filePath} title={diff.path}>
          {shortPath(relPath, 3)}
        </span>
        {isNewFile && <span className={styles.badgeNew}>新文件</span>}
        <span className={styles.stats}>
          <span className={styles.added}>+{stats.added}</span>{' '}
          <span className={styles.removed}>-{stats.removed}</span>
        </span>
        <button
          type="button"
          className={styles.diskBtn}
          onClick={(e) => {
            e.stopPropagation()
            if (showDisk) {
              setShowDisk(false)
            } else if (diskText != null) {
              setShowDisk(true)
            } else {
              loadDisk()
            }
          }}
          title="查看磁盘当前内容与本次改动的差异"
        >
          {diskLoading ? '读取中...' : showDisk ? '隐藏磁盘对比' : '对比磁盘'}
        </button>
      </div>

      {expanded && (
        <div className={styles.diffBody}>
          {diskError && <div className={styles.diskError}>磁盘读取失败：{diskError}</div>}
          {showDisk && diskText != null ? (
            <>
              <div className={styles.diskHint}>
                左侧为磁盘当前内容，右侧为本次改动后的内容
              </div>
              <DiffTable lines={diskLines} />
            </>
          ) : (
            <DiffTable lines={lines} />
          )}
        </div>
      )}
    </div>
  )
}

// 行级 diff 表格渲染（导出供 ChangesPanel 复用）
export function DiffTable({ lines }: { lines: DiffLine[] }) {
  if (lines.length === 0) {
    return <div className={styles.empty}>（无差异内容）</div>
  }
  return (
    <table className={styles.diffTable}>
      <tbody>
        {lines.map((l, idx) => {
          const cls =
            l.type === 'add'
              ? styles.lineAdd
              : l.type === 'del'
                ? styles.lineDel
                : styles.lineCtx
          const sign = l.type === 'add' ? '+' : l.type === 'del' ? '-' : ' '
          return (
            <tr key={idx} className={cls}>
              <td className={styles.lineNo}>{l.oldNo ?? ''}</td>
              <td className={styles.lineNo}>{l.newNo ?? ''}</td>
              <td className={styles.lineSign}>{sign}</td>
              <td className={styles.lineText}>
                <pre>{l.text}</pre>
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

export default function DiffView({ message, sessionId, cwd, defaultExpanded = false }: DiffViewProps) {
  const diffs = useMemo(() => parseDiffsFromMessage(message), [message])
  if (diffs.length === 0) return null
  return (
    <div className={styles.container}>
      {diffs.map((d, i) => (
        <FileDiffCard
          key={`${d.path}-${i}`}
          diff={d}
          sessionId={sessionId}
          cwd={cwd}
          defaultExpanded={defaultExpanded}
        />
      ))}
    </div>
  )
}
