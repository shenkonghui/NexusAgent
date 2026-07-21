import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { useCurrentWorkspace } from '../hooks/useCurrentWorkspace'
import { readWorkspaceFile } from '../api/filesystem'
import type { DocFolder } from '../types'
import AppLayout, { SidebarToggleButton } from '../components/AppLayout'
import UserMenu from '../components/UserMenu'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import MarkdownContent from '../components/MarkdownContent'
import { ArrowLeft, BookOpenText } from 'lucide-react'
import styles from './DocViewerPage.module.css'

const DOCS_KEY = 'opennexus.documents'

// 从 localStorage 加载所有文档文件夹绑定（跨工作区存储，路由进来时按 folderId 定位）
function loadFolders(): DocFolder[] {
  try {
    const raw = localStorage.getItem(DOCS_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore */ }
  return []
}

// 从文件相对路径提取文件名作为标题
function fileNameOf(relPath: string): string {
  return relPath.split('/').pop() || relPath
}

export default function DocViewerPage() {
  const { t } = useTranslation()
  // folderId：文档所属的绑定文件夹 id；'*'：react-router v6 通配捕获的剩余路径（文档相对路径）
  const { folderId, '*': filePath } = useParams<{ folderId: string; '*': string }>()
  const navigate = useNavigate()
  const { user, loading: authLoading } = useRequireAuth()
  const { workspaceId, sessions } = useCurrentWorkspace(!!user)

  const [absPath, setAbsPath] = useState('')
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!folderId || !filePath) {
      setError(t('documents.fileNotFound'))
      setLoading(false)
      return
    }
    const found = loadFolders().find((d) => d.id === folderId)
    if (!found) {
      setError(t('documents.fileNotFound'))
      setLoading(false)
      return
    }
    // 拼接绝对路径：文件夹绑定根 + 相对路径（filePath 来自 URL，已是正斜杠形式）
    const full = `${found.path.replace(/\/$/, '')}/${filePath}`
    setAbsPath(full)
    setLoading(true)
    setError('')
    readWorkspaceFile(full)
      .then((resp) => setContent(resp.data.content))
      .catch((err) => setError(err instanceof Error ? err.message : t('documents.readFailed')))
      .finally(() => setLoading(false))
  }, [folderId, filePath, t])

  const reload = () => {
    if (!absPath) return
    setLoading(true)
    setError('')
    readWorkspaceFile(absPath)
      .then((resp) => setContent(resp.data.content))
      .catch((err) => setError(err instanceof Error ? err.message : t('documents.readFailed')))
      .finally(() => setLoading(false))
  }

  if (authLoading || !user) return <LoadingSpinner />

  return (
    <AppLayout sidebarProps={{ sessions, workspaceId }}>
      <div className={styles.main}>
        <header className={styles.header}>
          <div className={styles.headerLeft}>
            <SidebarToggleButton />
            <button
              type="button"
              className={styles.backBtn}
              onClick={() => navigate(-1)}
              title={t('common.back')}
            >
              <ArrowLeft size={16} />
            </button>
            <BookOpenText size={18} className={styles.docIcon} />
            <h1 className={styles.title}>{filePath ? fileNameOf(filePath) : t('documents.title')}</h1>
          </div>
          <div className={styles.headerRight}>
            <span className={styles.docPath} title={absPath}>{absPath}</span>
            <UserMenu />
          </div>
        </header>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}
        <div className={styles.content}>
          {loading ? (
            <LoadingSpinner />
          ) : error ? (
            <div className={styles.errorState}>
              <p>{error}</p>
              <button type="button" className={styles.retryBtn} onClick={reload}>
                {t('common.retry')}
              </button>
            </div>
          ) : (
            <div className="markdown-body">
              <MarkdownContent content={content} />
            </div>
          )}
        </div>
      </div>
    </AppLayout>
  )
}
