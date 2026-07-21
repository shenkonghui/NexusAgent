import { useState, useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { readWorkspaceFile, writeWorkspaceFile } from '../api/filesystem'
import ErrorBanner from './ErrorBanner'
import LoadingSpinner from './LoadingSpinner'
import MarkdownContent from './MarkdownContent'
import CodeEditor from './CodeEditor'
import { Pencil, Eye, Save, Check } from 'lucide-react'
import { loadDocFolders } from '../utils/docs'
import styles from './DocWorkspace.module.css'

// 从文件相对路径提取文件名作为标题
function fileNameOf(relPath: string): string {
  return relPath.split('/').pop() || relPath
}

export type DocEditMode = 'view' | 'edit'

export interface DocWorkspaceProps {
  /** 文档所属的绑定文件夹 id（前端 localStorage UUID）。传 absPath 时可留空。 */
  folderId?: string
  /** 文档相对路径（相对文件夹根，正斜杠形式）。传 absPath 时用作标题回退。 */
  filePath?: string
  /** 直接指定文档绝对路径（来自左侧文件浏览器点击）；优先于 folderId+filePath。 */
  absPath?: string
  /** 外部触发重新从磁盘读取（如 AI 编辑完文件后刷新预览）。变化即 reload。 */
  reloadKey?: number
  /** 右上角关闭按钮（退回空态） */
  onClose?: () => void
  /**
   * 文档正文（受控）。由父组件（ChatPage）持有，便于 AI 流式生成时直接更新。
   * 不传时组件内部自管理（用于独立场景）。
   */
  content?: string
  onContentChange?: (next: string) => void
  /** 外部强制切到某模式（例如 AI 生成完强制切预览）。不传则完全由内部 toggle 控制。 */
  forceMode?: DocEditMode
}

// 文档工作区：只负责文档的读/写/编辑/预览。
// AI 对话能力已上移到 ChatPage（复用原生 PromptInput + MessageList），
// 通过 onContentChange 回调把 AI 生成的 drawio XML 写回文档。
export default function DocWorkspace({
  folderId,
  filePath,
  absPath: absPathProp,
  reloadKey,
  onClose,
  content: controlledContent,
  onContentChange,
  forceMode,
}: DocWorkspaceProps) {
  const { t } = useTranslation()

  const [absPath, setAbsPath] = useState('')
  const [internalContent, setInternalContent] = useState('')
  const content = controlledContent !== undefined ? controlledContent : internalContent
  const setContent = (next: string) => {
    if (onContentChange) onContentChange(next)
    else setInternalContent(next)
  }

  const [savedContent, setSavedContent] = useState('') // 最近一次落盘内容，用于 dirty 检测
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [mode, setMode] = useState<DocEditMode>('view')
  const [saving, setSaving] = useState(false)
  const [justSaved, setJustSaved] = useState(false)

  const dirty = content !== savedContent
  const justSavedTimer = useRef<number | undefined>(undefined)

  // 外部强制模式（AI 生成完成后切预览）
  useEffect(() => {
    if (forceMode) setMode(forceMode)
  }, [forceMode])

  useEffect(() => {
    // 优先使用外部传入的绝对路径（文件浏览器点击）；否则由 folderId+相对路径解析。
    let full = absPathProp || ''
    if (!full) {
      if (!folderId || !filePath) {
        setError(t('documents.fileNotFound'))
        setLoading(false)
        return
      }
      const found = loadDocFolders().find((d) => d.id === folderId)
      if (!found) {
        setError(t('documents.fileNotFound'))
        setLoading(false)
        return
      }
      full = `${found.path.replace(/\/$/, '')}/${filePath}`
    }
    setAbsPath(full)
    setLoading(true)
    setError('')
    readWorkspaceFile(full)
      .then((resp) => {
        setContent(resp.data.content)
        setSavedContent(resp.data.content)
      })
      .catch((err) => setError(err instanceof Error ? err.message : t('documents.readFailed')))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [folderId, filePath, absPathProp, reloadKey, t])

  const reload = useCallback(() => {
    if (!absPath) return
    setLoading(true)
    setError('')
    readWorkspaceFile(absPath)
      .then((resp) => {
        setContent(resp.data.content)
        setSavedContent(resp.data.content)
      })
      .catch((err) => setError(err instanceof Error ? err.message : t('documents.readFailed')))
      .finally(() => setLoading(false))
  }, [absPath, t, setContent])

  const handleSave = useCallback(async () => {
    if (!absPath || saving || !dirty) return
    setSaving(true)
    setError('')
    try {
      await writeWorkspaceFile(absPath, content)
      setSavedContent(content)
      setJustSaved(true)
      window.clearTimeout(justSavedTimer.current)
      justSavedTimer.current = window.setTimeout(() => setJustSaved(false), 2000)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('docEditor.saveFailed'))
    } finally {
      setSaving(false)
    }
  }, [absPath, content, dirty, saving, t])

  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (dirty) {
        e.preventDefault()
        e.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [dirty])

  return (
    <div className={styles.workspace}>
      <div className={styles.docHeader}>
        <div className={styles.docHeaderLeft}>
          <span className={styles.docTitle}>{(filePath || absPathProp) ? fileNameOf(filePath || absPathProp || '') : t('documents.title')}</span>
          <span className={styles.docPath} title={absPath}>{absPath}</span>
        </div>
        <div className={styles.docHeaderRight}>
          <div className={styles.modeSwitch}>
            <button
              type="button"
              className={`${styles.modeBtn} ${mode === 'view' ? styles.modeBtnActive : ''}`}
              onClick={() => setMode('view')}
              title={t('docEditor.preview')}
            >
              <Eye size={14} />
              <span className={styles.modeLabel}>{t('docEditor.preview')}</span>
            </button>
            <button
              type="button"
              className={`${styles.modeBtn} ${mode === 'edit' ? styles.modeBtnActive : ''}`}
              onClick={() => setMode('edit')}
              title={t('docEditor.edit')}
            >
              <Pencil size={14} />
              <span className={styles.modeLabel}>{t('docEditor.edit')}</span>
            </button>
          </div>
          <button
            type="button"
            className={styles.saveBtn}
            onClick={handleSave}
            disabled={!dirty || saving}
            title={t('docEditor.save')}
          >
            {justSaved ? <Check size={14} /> : <Save size={14} />}
            <span className={styles.modeLabel}>
              {saving ? t('docEditor.saving') : justSaved ? t('docEditor.saved') : t('docEditor.save')}
            </span>
          </button>
          {onClose && (
            <button
              type="button"
              className={styles.iconBtn}
              onClick={() => {
                if (dirty && !window.confirm(t('docEditor.unsavedConfirm'))) return
                onClose()
              }}
              title={t('common.close')}
            >
              ✕
            </button>
          )}
        </div>
      </div>

      {error && <ErrorBanner message={error} onClose={() => setError('')} />}

      <div className={styles.content}>
        {loading ? (
          <LoadingSpinner />
        ) : error && !content ? (
          <div className={styles.errorState}>
            <p>{error}</p>
            <button type="button" className={styles.retryBtn} onClick={reload}>
              {t('common.retry')}
            </button>
          </div>
        ) : mode === 'edit' ? (
          <div className={styles.editorBox}>
            <CodeEditor value={content} onChange={setContent} filePath={filePath || fileNameOf(absPathProp || '') || 'doc.md'} />
          </div>
        ) : (
          <div className="markdown-body">
            <MarkdownContent content={content} />
          </div>
        )}
      </div>
    </div>
  )
}
