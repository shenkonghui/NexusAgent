import { useState, useEffect, useCallback } from 'react'
import type { ReactNode } from 'react'
import { listSessionFiles, type SessionFileEntry } from '../api/filesystem'
import { ChevronDown, ChevronRight, Folder, FolderOpen, FileCode, FileJson, FileText, File, RefreshCw } from 'lucide-react'
import styles from './FileExplorer.module.css'

interface FileExplorerProps {
  sessionId: number
  onSelectFile: (path: string) => void
  selectedPath?: string
}

// 文件图标根据扩展名
function fileIcon(name: string): ReactNode {
  if (name.endsWith('.go')) return <FileCode size={14} />
  if (name.endsWith('.ts') || name.endsWith('.tsx')) return <FileCode size={14} />
  if (name.endsWith('.js') || name.endsWith('.jsx')) return <FileCode size={14} />
  if (name.endsWith('.py')) return <FileCode size={14} />
  if (name.endsWith('.md')) return <FileText size={14} />
  if (name.endsWith('.json')) return <FileJson size={14} />
  if (name.endsWith('.css')) return <FileCode size={14} />
  if (name.endsWith('.html')) return <FileCode size={14} />
  if (name.endsWith('.yaml') || name.endsWith('.yml')) return <FileText size={14} />
  if (name.endsWith('.sh')) return <FileCode size={14} />
  return <File size={14} />
}

export default function FileExplorer({ sessionId, onSelectFile, selectedPath }: FileExplorerProps) {
  // 展开的目录路径 -> 子节点
  const [expanded, setExpanded] = useState<Map<string, SessionFileEntry[]>>(new Map())
  const [rootEntries, setRootEntries] = useState<SessionFileEntry[]>([])
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set())
  const [error, setError] = useState('')

  // 加载根目录
  const loadRoot = useCallback(async () => {
    setError('')
    try {
      const resp = await listSessionFiles(sessionId)
      setRootEntries(resp.data.entries)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载文件列表失败')
    }
  }, [sessionId])

  useEffect(() => {
    loadRoot()
  }, [loadRoot])

  // 切换目录展开/折叠
  async function toggleDir(path: string) {
    if (expanded.has(path)) {
      setExpanded((prev) => {
        const next = new Map(prev)
        next.delete(path)
        return next
      })
      return
    }
    setLoadingPaths((prev) => new Set(prev).add(path))
    setError('')
    try {
      const resp = await listSessionFiles(sessionId, path)
      setExpanded((prev) => new Map(prev).set(path, resp.data.entries))
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载目录失败')
    } finally {
      setLoadingPaths((prev) => {
        const next = new Set(prev)
        next.delete(path)
        return next
      })
    }
  }

  // 递归渲染文件树节点
  function renderEntries(entries: SessionFileEntry[], level: number): React.ReactNode {
    return entries.map((entry) => {
      const isExpanded = expanded.has(entry.path)
      const isSelected = selectedPath === entry.path
      const isLoading = loadingPaths.has(entry.path)

      return (
        <div key={entry.path}>
          <div
            className={`${styles.item} ${isSelected ? styles.itemSelected : ''}`}
            style={{ paddingLeft: `${level * 14 + 8}px` }}
            onClick={() => (entry.is_dir ? toggleDir(entry.path) : onSelectFile(entry.path))}
          >
            <span className={styles.icon}>
              {entry.is_dir ? (isExpanded ? <FolderOpen size={14} /> : <Folder size={14} />) : fileIcon(entry.name)}
            </span>
            <span className={styles.name}>{entry.name}</span>
            {entry.is_dir && (
              <span className={styles.chevron}>
                {isLoading ? '⋯' : isExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              </span>
            )}
          </div>
          {entry.is_dir && isExpanded && expanded.get(entry.path) && (
            <div className={styles.children}>
              {renderEntries(expanded.get(entry.path)!, level + 1)}
            </div>
          )}
        </div>
      )
    })
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <span className={styles.title}>文件</span>
        <button className={styles.refreshBtn} onClick={loadRoot} title="刷新" type="button">
          <RefreshCw size={14} />
        </button>
      </div>
      {error && <div className={styles.error}>{error}</div>}
      <div className={styles.tree}>
        {rootEntries.length === 0 && !error && (
          <p className={styles.empty}>暂无文件</p>
        )}
        {renderEntries(rootEntries, 0)}
      </div>
    </div>
  )
}
