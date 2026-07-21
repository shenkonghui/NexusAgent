import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronUp, ChevronDown } from 'lucide-react'
import { MODES } from '../modes/registry'
import styles from './TaskModeSwitch.module.css'

/**
 * 模式 id 的类型。从注册表派生——新增模式不需要改这里。
 * 保留显式联合让旧代码（localStorage 读写、类型断言）有字面量提示；
 * 任意 string 也接受，向后兼容老数据。
 */
export type TaskMode = string

interface TaskModeSwitchProps {
  value: TaskMode
  onChange: (mode: TaskMode) => void
}

/**
 * 模式切换器：下拉框形式，选项直接来自 MODES 注册表。
 * 新增一个模式只需在 registry.tsx 加一条 + 对应 i18n key，
 * 这里会自动多出一个选项，ChatPage 也会自动渲染其布局。
 */
export default function TaskModeSwitch({ value, onChange }: TaskModeSwitchProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const current = MODES.find((m) => m.id === value) || MODES[0]

  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  function handleSelect(id: string) {
    onChange(id)
    setOpen(false)
  }

  return (
    <div className={styles.container} ref={containerRef}>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        title={current ? t(current.titleKey) : undefined}
      >
        {current?.icon}
        <span className={styles.label}>{current ? t(current.titleKey) : ''}</span>
        <span className={styles.arrow}>{open ? <ChevronUp size={12} /> : <ChevronDown size={12} />}</span>
      </button>

      {open && (
        <div className={styles.dropdown} role="listbox">
          {MODES.map((opt) => (
            <button
              key={opt.id}
              type="button"
              role="option"
              aria-selected={value === opt.id}
              className={`${styles.item} ${value === opt.id ? styles.itemActive : ''}`}
              onClick={() => handleSelect(opt.id)}
            >
              {opt.icon}
              <span className={styles.label}>{t(opt.titleKey)}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
