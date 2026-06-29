import { useTranslation } from 'react-i18next'
import styles from './LoadingSpinner.module.css'

export default function LoadingSpinner({ text }: { text?: string }) {
  const { t } = useTranslation()
  return (
    <div className={styles.container}>
      <div className={styles.spinner} />
      <span className={styles.text}>{text || t('common.loading')}</span>
    </div>
  )
}
