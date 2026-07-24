import { useState, useEffect, useCallback } from 'react'
import type { ReactNode } from 'react'
import {
  listSessionFiles,
  listFiles,
  createWorkspaceEntry,
  deleteWorkspaceEntry,
  type SessionFileEntry,
} from '../api/filesystem'
import {
  ChevronDown,
  ChevronRight,
  Folder,
  FolderOpen,
  FileCode,
  FileJson,
  FileText,
  File,
  RefreshCw,
  FilePlus,
  FolderPlus,
  Trash2,
} from 'lucide-react'
import styles from './FileExplorer.module.css'

interface FileExplorerProps {
  onSelectFile: (path: string) => void
  selectedPath?: string
  /** 会话模式：相对 session cwd 懒加载（与 rootPath 二选一，优先） */
  sessionId?: number
  /** 工作区模式：以绝对路径为根懒加载（sessionId 未提供时生效） */
  rootPath?: string
}

// 新建输入框状态：在 parentPath 目录下新建文件/文件夹
interface CreatingState {
  parentPath: string
  isDir: boolean
}

// 右键菜单状态
interface ContextMenuState {
  x: number
  y: number
  entry: SessionFileEntry | null // null 表示空白区域（针对根目录）
}

// 文件图标根据扩展名（图标形状 + 颜色，接近 VS Code 风格）
function fileIcon(name: string): ReactNode {
  const ext = name.split('.').pop()?.toLowerCase() || ''
  switch (ext) {
    case 'go':
      return <FileCode size={14} color="#00add8" />
    case 'ts':
    case 'tsx':
      return <FileCode size={14} color="#3178c6" />
    case 'js':
    case 'jsx':
    case 'mjs':
      return <FileCode size={14} color="#f1e05a" />
    case 'py':
      return <FileCode size={14} color="#3572a5" />
    case 'rs':
      return <FileCode size={14} color="#dea584" />
    case 'java':
      return <FileCode size={14} color="#b07219" />
    case 'c':
    case 'cpp':
    case 'cc':
    case 'h':
    case 'hpp':
      return <FileCode size={14} color="#659ad2" />
    case 'css':
      return <FileCode size={14} color="#563d7c" />
    case 'html':
    case 'xml':
      return <FileCode size={14} color="#e34c26" />
    case 'sh':
    case 'bash':
      return <FileCode size={14} color="#89e051" />
    case 'sql':
      return <FileCode size={14} color="#e38c00" />
    case 'md':
      return <FileText size={14} color="#519aba" />
    case 'yaml':
    case 'yml':
      return <FileText size={14} color="#cb171e" />
    case 'json':
      return <FileJson size={14} color="#cbcb41" />
    default:
      return <File size={14} color="#8a929c" />
  }
}

// 目录图标颜色
const FOLDER_COLOR = '#dcb67a'

// 拼接绝对路径（工作区模式路径以 / 分隔）
function joinPath(dir: string, name: string): string {
  return dir.endsWith('/') ? dir + name : dir + '/' + name
}

export default function FileExplorer({ sessionId, rootPath, onSelectFile, selectedPath }: FileExplorerProps) {
  // 展开的目录路径 -> 子节点
  const [expanded, setExpanded] = useState<Map<string, SessionFileEntry[]>>(new Map())
  const [rootEntries, setRootEntries] = useState<SessionFileEntry[]>([])
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set())
  const [error, setError] = useState('')
  const [creating, setCreating] = useState<CreatingState | null>(null)
  const [newName, setNewName] = useState('')
  const [menu, setMenu] = useState<ContextMenuState | null>(null)

  // 仅工作区模式（绝对路径）支持新建/删除
  const canEdit = sessionId == null && !!rootPath

  // 统一数据源：优先会话模式(sessionId)，否则工作区模式(rootPath 绝对路径)
  const loadEntries = useCallback(async (path?: string): Promise<SessionFileEntry[]> => {
    if (sessionId != null) {
      const resp = await listSessionFiles(sessionId, path)
      return resp.data.entries
    }
    const target = path || rootPath
    if (!target) return []
    const resp = await listFiles(target)
    // 工作区 FileEntry 统一为 SessionFileEntry 结构；目录优先、同类按名排序
    return resp.data.entries
      .map((e) => ({ name: e.name, path: e.path, is_dir: e.is_dir }))
      .sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1))
  }, [sessionId, rootPath])

  // 加载根目录（数据源变化时重置展开态）
  const loadRoot = useCallback(async () => {
    setError('')
    setExpanded(new Map())
    if (sessionId == null && !rootPath) { setRootEntries([]); return }
    try {
      setRootEntries(await loadEntries())
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载文件列表失败')
    }
  }, [loadEntries, sessionId, rootPath])

  useEffect(() => {
    loadRoot()
  }, [loadRoot])

  // 重新加载指定目录（null 或 rootPath 表示根目录），刷新对应节点
  const reloadDir = useCallback(async (dirPath: string | null) => {
    try {
      if (dirPath == null || dirPath === rootPath) {
        setRootEntries(await loadEntries())
      } else {
        const entries = await loadEntries(dirPath)
        setExpanded((prev) => new Map(prev).set(dirPath, entries))
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '刷新目录失败')
    }
  }, [loadEntries, rootPath])

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
      const entries = await loadEntries(path)
      setExpanded((prev) => new Map(prev).set(path, entries))
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

  // 确保目录处于展开状态（新建时用），返回是否已有缓存
  const ensureExpanded = useCallback(async (path: string) => {
    if (expanded.has(path)) return
    try {
      const entries = await loadEntries(path)
      setExpanded((prev) => new Map(prev).set(path, entries))
    } catch {
      /* 忽略：新建后 reloadDir 会再次刷新 */
    }
  }, [expanded, loadEntries])

  // 开始新建：在 parent 目录下创建文件/文件夹
  async function startCreate(parent: string, isDir: boolean) {
    setMenu(null)
    if (parent !== rootPath) {
      await ensureExpanded(parent)
    }
    setNewName('')
    setCreating({ parentPath: parent, isDir })
  }

  // 提交新建
  async function submitCreate() {
    if (!creating) return
    const name = newName.trim()
    if (!name) { setCreating(null); return }
    const targetPath = joinPath(creating.parentPath, name)
    setError('')
    try {
      await createWorkspaceEntry(targetPath, creating.isDir)
      const parent = creating.parentPath
      setCreating(null)
      setNewName('')
      await reloadDir(parent === rootPath ? null : parent)
    } catch (err) {
      setError(err instanceof Error ? err.message : '新建失败')
      setCreating(null)
    }
  }

  // 删除文件/目录
  async function handleDelete(entry: SessionFileEntry) {
    setMenu(null)
    const tip = entry.is_dir
      ? `确定删除文件夹 "${entry.name}" 及其全部内容？`
      : `确定删除文件 "${entry.name}"？`
    if (!window.confirm(tip)) return
    setError('')
    try {
      await deleteWorkspaceEntry(entry.path)
      const idx = entry.path.lastIndexOf('/')
      const parent = idx > 0 ? entry.path.slice(0, idx) : (rootPath ?? '')
      await reloadDir(parent === rootPath ? null : parent)
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败')
    }
  }

  // 关闭右键菜单（点击其他区域 / 按 Esc）
  useEffect(() => {
    if (!menu) return
    const close = () => setMenu(null)
    document.addEventListener('click', close)
    document.addEventListener('contextmenu', close)
    return () => {
      document.removeEventListener('click', close)
      document.removeEventListener('contextmenu', close)
    }
  }, [menu])

  function openContextMenu(e: React.MouseEvent, entry: SessionFileEntry | null) {
    if (!canEdit) return
    e.preventDefault()
    e.stopPropagation()
    setMenu({ x: e.clientX, y: e.clientY, entry })
  }

  // 渲染新建输入行
  function renderCreateInput(level: number) {
    if (!creating) return null
    return (
      <div className={styles.item} style={{ paddingLeft: `${level * 14 + 8}px` }}>
        <span className={styles.icon}>
          {creating.isDir ? <Folder size={14} /> : <File size={14} />}
        </span>
        <input
          className={styles.newInput}
          autoFocus
          value={newName}
          placeholder={creating.isDir ? '文件夹名称' : '文件名称'}
          onChange={(e) => setNewName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') submitCreate()
            else if (e.key === 'Escape') { setCreating(null); setNewName('') }
          }}
          onBlur={submitCreate}
        />
      </div>
    )
  }

  // 递归渲染文件树节点
  function renderEntries(entries: SessionFileEntry[], level: number): React.ReactNode {
    return entries.map((entry) => {
      const isExpanded = expanded.has(entry.path)
      const isSelected = selectedPath === entry.path
      const isLoading = loadingPaths.has(entry.path)
      const showCreateHere = creating?.parentPath === entry.path

      return (
        <div key={entry.path}>
          <div
            className={`${styles.item} ${isSelected ? styles.itemSelected : ''}`}
            style={{ paddingLeft: `${level * 14 + 8}px` }}
            onClick={() => (entry.is_dir ? toggleDir(entry.path) : onSelectFile(entry.path))}
            onContextMenu={(e) => openContextMenu(e, entry)}
          >
            <span className={styles.icon}>
              {entry.is_dir ? (isExpanded ? <FolderOpen size={14} color={FOLDER_COLOR} /> : <Folder size={14} color={FOLDER_COLOR} />) : fileIcon(entry.name)}
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
              {showCreateHere && renderCreateInput(level + 1)}
              {renderEntries(expanded.get(entry.path)!, level + 1)}
            </div>
          )}
        </div>
      )
    })
  }

  // 根目录新建输入是否显示（parentPath === rootPath）
  const showRootCreate = creating != null && creating.parentPath === rootPath

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <span className={styles.title}>文件</span>
        <div className={styles.headerActions}>
          {canEdit && (
            <>
              <button
                className={styles.refreshBtn}
                onClick={() => startCreate(rootPath!, false)}
                title="新建文件"
                type="button"
              >
                <FilePlus size={14} />
              </button>
              <button
                className={styles.refreshBtn}
                onClick={() => startCreate(rootPath!, true)}
                title="新建文件夹"
                type="button"
              >
                <FolderPlus size={14} />
              </button>
            </>
          )}
          <button className={styles.refreshBtn} onClick={loadRoot} title="刷新" type="button">
            <RefreshCw size={14} />
          </button>
        </div>
      </div>
      {error && <div className={styles.error}>{error}</div>}
      <div className={styles.tree} onContextMenu={(e) => openContextMenu(e, null)}>
        {rootEntries.length === 0 && !showRootCreate && !error && (
          <p className={styles.empty}>暂无文件</p>
        )}
        {showRootCreate && renderCreateInput(0)}
        {renderEntries(rootEntries, 0)}
      </div>

      {menu && (
        <div className={styles.contextMenu} style={{ left: menu.x, top: menu.y }}>
          {menu.entry?.is_dir && (
            <>
              <button type="button" className={styles.menuItem} onClick={() => startCreate(menu.entry!.path, false)}>
                <FilePlus size={13} /> 新建文件
              </button>
              <button type="button" className={styles.menuItem} onClick={() => startCreate(menu.entry!.path, true)}>
                <FolderPlus size={13} /> 新建文件夹
              </button>
            </>
          )}
          {!menu.entry && (
            <>
              <button type="button" className={styles.menuItem} onClick={() => startCreate(rootPath!, false)}>
                <FilePlus size={13} /> 新建文件
              </button>
              <button type="button" className={styles.menuItem} onClick={() => startCreate(rootPath!, true)}>
                <FolderPlus size={13} /> 新建文件夹
              </button>
            </>
          )}
          {menu.entry && (
            <button type="button" className={`${styles.menuItem} ${styles.menuItemDanger}`} onClick={() => handleDelete(menu.entry!)}>
              <Trash2 size={13} /> 删除
            </button>
          )}
        </div>
      )}
    </div>
  )
}
