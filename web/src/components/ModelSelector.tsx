import type { ConfigOption } from '../types'
import styles from './ModelSelector.module.css'

interface ModelSelectorProps {
  options: ConfigOption[]
  onApply: (configId: string, value: string) => void
  disabled?: boolean
}

// ModelSelector 渲染 category=model 的 config option 作为模型选择下拉框。
// 其它 config option（mode/thought_level 等）也一并渲染为下拉框。
export default function ModelSelector({ options, onApply, disabled }: ModelSelectorProps) {
  // 仅渲染 select 类型且有可选项的 config option
  const selectable = options.filter((o) => o.type === 'select' && o.options.length > 0)
  if (selectable.length === 0) return null

  return (
    <div className={styles.container}>
      {selectable.map((opt) => (
        <div key={opt.id} className={styles.item}>
          <label className={styles.label}>
            {opt.category === 'model' ? '模型' : opt.name}
          </label>
          <select
            className={styles.select}
            value={opt.current_value}
            disabled={disabled}
            onChange={(e) => onApply(opt.id, e.target.value)}
          >
            {opt.options.map((v) => (
              <option key={v.value} value={v.value}>
                {v.name}
                {v.description ? ` — ${v.description}` : ''}
              </option>
            ))}
          </select>
        </div>
      ))}
    </div>
  )
}
