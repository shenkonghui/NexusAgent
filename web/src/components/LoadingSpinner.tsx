import styles from './LoadingSpinner.module.css'

export default function LoadingSpinner({ text = '加载中...' }: { text?: string }) {
  return (
    <div className={styles.container}>
      <div className={styles.spinner} />
      <span className={styles.text}>{text}</span>
    </div>
  )
}
