import { useTranslation } from 'react-i18next'
import type { PermissionRequestPayload } from '../types'
import { permissionOptionStyle, sortPermissionOptions } from '../utils/permission'
import styles from './PermissionDialog.module.css'

interface PermissionDialogProps {
  request: PermissionRequestPayload
  title?: string
  responding?: boolean
  onRespond: (optionId: string) => void
  onCancel: () => void
}

export default function PermissionDialog({
  request,
  title,
  responding = false,
  onRespond,
  onCancel,
}: PermissionDialogProps) {
  const { t } = useTranslation()
  const displayTitle = title || request.tool_call?.title || t('session.permission.defaultTitle')
  const options = sortPermissionOptions(request.options)

  return (
    <div className={styles.inline}>
      <span className={styles.desc} title={displayTitle}>{displayTitle}</span>
      <div className={styles.actions}>
        {options.map((opt) => (
          <button
            key={opt.optionId}
            type="button"
            className={`${styles.btn} ${styles[`btn_${permissionOptionStyle(opt.kind)}`]}`}
            disabled={responding}
            onClick={() => onRespond(opt.optionId)}
          >
            {opt.name}
          </button>
        ))}
        <button
          type="button"
          className={`${styles.btn} ${styles.btn_cancel}`}
          disabled={responding}
          onClick={onCancel}
        >
          {t('common.cancel')}
        </button>
      </div>
    </div>
  )
}
