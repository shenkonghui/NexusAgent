import { useState, useEffect, useCallback } from 'react'
import { listDirs, type DirEntry } from '../api/filesystem'
import LoadingSpinner from './LoadingSpinner'
import styles from './DirectoryPicker.module.css'

interface DirectoryPickerProps {
  /** 初始路径，为空时从用户主目录开始 */
  initialPath?: string
  /** 选择目录后回调 */
  onSelect: (path: string) => void
  /** 关闭弹窗 */
  onClose: () => void
}

export default function DirectoryPicker({ initialPath, onSelect, onClose }: DirectoryPickerProps) {
  const [currentPath, setCurrentPath] = useState(initialPath || '')
  const [parentPath, setParentPath] = useState('')
  const [dirs, setDirs] = useState<DirEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const loadDirs = useCallback(async (path: string) => {
    setLoading(true)
    setError('')
    try {
      const resp = await listDirs(path)
      setCurrentPath(resp.data.current_path)
      setParentPath(resp.data.parent_path)
      setDirs(resp.data.dirs)
    } catch (err) {
      setError(err instanceof Error ? err.message : '读取目录失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadDirs(initialPath || '')
  }, [initialPath, loadDirs])

  // ESC 关闭
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h3 className={styles.title}>选择工作目录</h3>
          <button className={styles.closeBtn} onClick={onClose} type="button">×</button>
        </div>

        {/* 当前路径 + 返回上级 */}
        <div className={styles.pathBar}>
          <button
            className={styles.upBtn}
            onClick={() => parentPath && loadDirs(parentPath)}
            disabled={!parentPath || loading}
            type="button"
            title="返回上级目录"
          >
            ↑ 上级
          </button>
          <span className={styles.currentPath} title={currentPath}>{currentPath}</span>
        </div>

        {error && <div className={styles.error}>{error}</div>}

        <div className={styles.dirList}>
          {loading ? (
            <LoadingSpinner />
          ) : dirs.length === 0 ? (
            <p className={styles.empty}>该目录下没有子目录</p>
          ) : (
            dirs.map((dir) => (
              <button
                key={dir.path}
                className={styles.dirItem}
                onClick={() => loadDirs(dir.path)}
                onDoubleClick={() => onSelect(dir.path)}
                type="button"
                title={dir.path}
              >
                <span className={styles.dirIcon}>📁</span>
                <span className={styles.dirName}>{dir.name}</span>
              </button>
            ))
          )}
        </div>

        <div className={styles.footer}>
          <button className={styles.cancelBtn} onClick={onClose} type="button">取消</button>
          <button
            className={styles.confirmBtn}
            onClick={() => onSelect(currentPath)}
            disabled={!currentPath}
            type="button"
          >
            选择此目录
          </button>
        </div>
      </div>
    </div>
  )
}
