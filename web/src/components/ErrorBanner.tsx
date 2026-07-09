import { X } from 'lucide-react'
import styles from './ErrorBanner.module.css'

interface ErrorBannerProps {
  message: string
  onClose?: () => void
  /** 重试回调，提供时显示重试按钮 */
  onRetry?: () => void
  /** 重试按钮文字 */
  retryLabel?: string
}

export default function ErrorBanner({ message, onClose, onRetry, retryLabel = '重试' }: ErrorBannerProps) {
  return (
    <div className={styles.banner}>
      <span className={styles.message}>{message}</span>
      <div className={styles.actions}>
        {onRetry && (
          <button className={styles.retryBtn} onClick={onRetry} type="button">
            {retryLabel}
          </button>
        )}
        {onClose && (
          <button className={styles.closeBtn} onClick={onClose} type="button">
            <X size={16} />
          </button>
        )}
      </div>
    </div>
  )
}
