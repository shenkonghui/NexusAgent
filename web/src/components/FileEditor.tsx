import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { readFileContent } from '../api/config'
import type { ScannedFileItem } from '../api/config'
import styles from './FileEditor.module.css'

interface Props {
  file: ScannedFileItem | null
  saving: boolean
  onSave: (path: string, content: string) => void
  onClose: () => void
}

export default function FileEditor({ file, saving, onSave, onClose }: Props) {
  const { t } = useTranslation()
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (file && file.location) {
      loadFile(file.location)
    } else {
      setContent('')
    }
  }, [file])

  async function loadFile(filePath: string) {
    setLoading(true)
    setError('')
    try {
      const resp = await readFileContent(filePath)
      setContent(resp.data.content)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('configEditor.readFailed'))
    } finally {
      setLoading(false)
    }
  }

  function handleSave() {
    if (!file) return
    onSave(file.location, content)
  }

  if (!file) return null

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.dialog} onClick={(e) => e.stopPropagation()}>
        <div className={styles.dialogHeader}>
          <div className={styles.dialogTitleRow}>
            <h3 className={styles.dialogTitle}>
              📄 {file.name}
              {file.description && <span className={styles.fileDesc}> — {file.description}</span>}
            </h3>
            <div className={styles.fileMeta}>
              <span className={styles.fileScope}>{file.scope}</span>
              <span className={styles.filePath}>{file.location}</span>
            </div>
          </div>
          <button type="button" className={styles.closeBtn} onClick={onClose} disabled={saving}>
            ✕
          </button>
        </div>

        <div className={styles.dialogBody}>
          {loading ? (
            <div className={styles.loading}>{t('common.loading')}</div>
          ) : error ? (
            <div className={styles.errorMsg}>{error}</div>
          ) : (
            <textarea
              className={styles.editor}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              disabled={saving}
              spellCheck={false}
            />
          )}
        </div>

        <div className={styles.dialogFooter}>
          <button type="button" className={styles.cancelBtn} onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </button>
          <button type="button" className={styles.saveBtn} onClick={handleSave} disabled={saving || loading}>
            {saving ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </div>
    </div>
  )
}
