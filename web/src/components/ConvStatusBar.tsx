import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import styles from './ConvStatusBar.module.css'

export type ConvState = 'idle' | 'connecting' | 'streaming' | 'reconnecting' | 'waiting_permission'

interface ConvStatusBarProps {
  state: ConvState
  children?: ReactNode
}

const stateKeys: Record<Exclude<ConvState, 'idle'>, string> = {
  connecting: 'session.conv_connecting',
  streaming: 'session.conv_streaming',
  reconnecting: 'session.conv_reconnecting',
  waiting_permission: 'session.conv_waiting_permission',
}

export default function ConvStatusBar({ state, children }: ConvStatusBarProps) {
  const { t } = useTranslation()
  if (state === 'idle') return null

  return (
    <div className={`${styles.bar} ${styles[`bar_${state}`]}`} role="status" aria-live="polite">
      <span className={styles.spinner} aria-hidden="true" />
      <span className={styles.text}>{t(stateKeys[state])}</span>
      {children && <div className={styles.content}>{children}</div>}
    </div>
  )
}
