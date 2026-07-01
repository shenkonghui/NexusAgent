import type { ConfigOption } from '../types'
import { formatOptionLabel, fullOptionLabel } from '../utils/selectLabel'
import styles from './ModelSelector.module.css'

interface ModelSelectorProps {
  options: ConfigOption[]
  onApply: (configId: string, value: string) => void
  disabled?: boolean
}

function optionLabel(opt: ConfigOption, name: string, description?: string): string {
  const isModel = opt.category === 'model'
  return formatOptionLabel(name, description, isModel ? undefined : 20)
}

function currentSelectTitle(opt: ConfigOption): string | undefined {
  const current = opt.options.find((o) => o.value === opt.current_value)
  if (!current) return undefined
  return fullOptionLabel(current.name, current.description)
}

// ModelSelector 渲染 config option 下拉框；除模型外选项文案限制 20 字符。
export default function ModelSelector({ options, onApply, disabled }: ModelSelectorProps) {
  const selectable = options.filter((o) => o.type === 'select' && o.options.length > 0 && o.category !== 'mode')
  if (selectable.length === 0) return null

  return (
    <>
      {selectable.map((opt) => {
        const isModel = opt.category === 'model'
        return (
          <div key={opt.id} className={styles.item}>
            <label className={styles.label}>
              {isModel ? '模型' : opt.name}
            </label>
            <select
              className={`${styles.select} ${isModel ? '' : styles.selectCompact}`}
              value={opt.current_value}
              disabled={disabled}
              title={currentSelectTitle(opt)}
              onChange={(e) => onApply(opt.id, e.target.value)}
            >
              {opt.options.map((v) => (
                <option
                  key={v.value}
                  value={v.value}
                  title={fullOptionLabel(v.name, v.description)}
                >
                  {optionLabel(opt, v.name, v.description)}
                </option>
              ))}
            </select>
          </div>
        )
      })}
    </>
  )
}
