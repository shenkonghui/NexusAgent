import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ConfigOptionValue } from '../types'
import { ChevronUp, ChevronDown } from 'lucide-react'
import styles from './ModelPicker.module.css'

interface ModelPickerProps {
  value: string
  options: ConfigOptionValue[]
  onChange: (value: string) => void
  disabled?: boolean
  placeholder?: string
}

export default function ModelPicker({ value, options, onChange, disabled, placeholder }: ModelPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [filter, setFilter] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  const selected = options.find((o) => o.value === value)
  const triggerLabel = selected?.name || value

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (!q) return options
    return options.filter((o) =>
      o.name.toLowerCase().includes(q)
      || o.value.toLowerCase().includes(q)
      || (o.description || '').toLowerCase().includes(q),
    )
  }, [options, filter])

  const showCustom = filter.trim() !== ''
    && !options.some((o) => o.value === filter.trim() || o.name === filter.trim())

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

  useEffect(() => {
    if (open) {
      setFilter('')
      requestAnimationFrame(() => searchRef.current?.focus())
    }
  }, [open])

  function handleSelect(next: string) {
    onChange(next)
    setOpen(false)
  }

  function toggleOpen() {
    if (disabled) return
    setOpen((v) => !v)
  }

  return (
    <div className={styles.container} ref={containerRef}>
      <button type="button" className={styles.trigger} onClick={toggleOpen} disabled={disabled}>
        <span className={`${styles.triggerLabel} ${!triggerLabel ? styles.triggerPlaceholder : ''}`}>
          {triggerLabel || placeholder || t('session.selectModel')}
        </span>
        <span className={styles.arrow}>{open ? <ChevronUp size={12} /> : <ChevronDown size={12} />}</span>
      </button>

      {open && (
        <div className={styles.dropdown}>
          <input
            ref={searchRef}
            className={styles.search}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={t('session.searchModels')}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                setOpen(false)
                return
              }
              if (e.key === 'Enter' && filter.trim()) {
                handleSelect(filter.trim())
              }
            }}
          />
          <div className={styles.list}>
            {filtered.length === 0 && !showCustom && (
              <div className={styles.empty}>{t('session.noModelsFound')}</div>
            )}
            {filtered.map((opt) => (
              <div
                key={opt.value}
                className={`${styles.item} ${opt.value === value ? styles.itemActive : ''}`}
                onClick={() => handleSelect(opt.value)}
              >
                <span className={styles.itemName}>{opt.name}</span>
                {opt.description && <span className={styles.itemDesc}>{opt.description}</span>}
              </div>
            ))}
            {showCustom && (
              <div className={`${styles.item} ${styles.customItem}`} onClick={() => handleSelect(filter.trim())}>
                <span className={styles.itemName}>{t('session.useCustomModel', { value: filter.trim() })}</span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
