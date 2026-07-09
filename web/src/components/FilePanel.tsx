import { useState, useCallback } from 'react'
import { readSessionFile, writeSessionFile } from '../api/filesystem'
import FileExplorer from './FileExplorer'
import CodeEditor from './CodeEditor'
import { X, Circle } from 'lucide-react'
import styles from './FilePanel.module.css'

interface FilePanelProps {
  sessionId: number
  onClose: () => void
}

export default function FilePanel({ sessionId, onClose }: FilePanelProps) {
  const [selectedPath, setSelectedPath] = useState('')
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [error, setError] = useState('')

  const handleSelectFile = useCallback(async (path: string) => {
    if (dirty && !window.confirm('当前文件未保存，确定切换？')) return
    setSelectedPath(path)
    setDirty(false)
    setError('')
    setLoading(true)
    try {
      const resp = await readSessionFile(sessionId, path)
      setContent(resp.data.content)
    } catch (err) {
      setError(err instanceof Error ? err.message : '读取文件失败')
      setContent('')
    } finally {
      setLoading(false)
    }
  }, [sessionId, dirty])

  const handleContentChange = useCallback((value: string) => {
    setContent(value)
    setDirty(true)
  }, [])

  const handleSave = useCallback(async () => {
    if (!selectedPath) return
    setSaving(true)
    setError('')
    try {
      await writeSessionFile(sessionId, selectedPath, content)
      setDirty(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : '保存文件失败')
    } finally {
      setSaving(false)
    }
  }, [sessionId, selectedPath, content])

  // Ctrl/Cmd+S 保存
  function handleKeyDown(e: React.KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 's') {
      e.preventDefault()
      if (dirty && !saving) handleSave()
    }
  }

  return (
    <div className={styles.panel} onKeyDown={handleKeyDown}>
      <div className={styles.toolbar}>
        <span className={styles.fileName} title={selectedPath}>
          {selectedPath || '未选择文件'}
          {dirty && <span className={styles.dirtyMark}> <Circle size={8} fill="currentColor" strokeWidth={0} /></span>}
        </span>
        <div className={styles.toolbarActions}>
          {selectedPath && (
            <button
              className={styles.saveBtn}
              onClick={handleSave}
              disabled={!dirty || saving}
              type="button"
              title="保存 (Ctrl/Cmd+S)"
            >
              {saving ? '保存中...' : '保存'}
            </button>
          )}
          <button className={styles.closeBtn} onClick={onClose} type="button" title="关闭面板">
            <X size={16} />
          </button>
        </div>
      </div>

      {error && <div className={styles.error}>{error}</div>}

      <div className={styles.body}>
        <div className={styles.explorer}>
          <FileExplorer
            sessionId={sessionId}
            onSelectFile={handleSelectFile}
            selectedPath={selectedPath}
          />
        </div>
        <div className={styles.editorArea}>
          {loading ? (
            <div className={styles.placeholder}>加载中...</div>
          ) : selectedPath ? (
            <CodeEditor
              value={content}
              onChange={handleContentChange}
              filePath={selectedPath}
            />
          ) : (
            <div className={styles.placeholder}>选择左侧文件查看内容</div>
          )}
        </div>
      </div>
    </div>
  )
}
