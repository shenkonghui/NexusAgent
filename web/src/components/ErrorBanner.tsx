import styles from './ErrorBanner.module.css'

export default function ErrorBanner({ message, onClose }: { message: string; onClose?: () => void }) {
  return (
    <div className={styles.banner}>
      <span className={styles.message}>{message}</span>
      {onClose && (
        <button className={styles.closeBtn} onClick={onClose} type="button">
          ×
        </button>
      )}
    </div>
  )
}
