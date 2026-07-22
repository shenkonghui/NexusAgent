import type { SessionMode } from '../types'
import { formatOptionLabel, fullOptionLabel } from '../utils/selectLabel'
import styles from './ModelSelector.module.css'

interface SessionModeSelectorProps {
  modes: SessionMode[]
  currentModeId: string
  onChange: (modeId: string) => void
  disabled?: boolean
}

export default function SessionModeSelector({
  modes,
  currentModeId,
  onChange,
  disabled = false,
}: SessionModeSelectorProps) {
  if (modes.length === 0) return null

  const current = modes.find((m) => m.id === currentModeId)

  return (
    <div className={styles.item}>
      <select
        className={`${styles.select} ${styles.selectCompact}`}
        value={currentModeId}
        disabled={disabled}
        title={current ? fullOptionLabel(current.name, current.description) : undefined}
        onChange={(e) => onChange(e.target.value)}
      >
        {modes.map((mode) => (
          <option
            key={mode.id}
            value={mode.id}
            title={fullOptionLabel(mode.name, mode.description)}
          >
            {formatOptionLabel(mode.name, mode.description, 10)}
          </option>
        ))}
      </select>
    </div>
  )
}
