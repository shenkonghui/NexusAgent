import { useState, useRef, useMemo, useEffect, Fragment, type MouseEvent as ReactMouseEvent, type ReactNode } from 'react'
import layoutStyles from '../modes/LayoutRenderer.module.css'

/**
 * SplitPane：可拖拽调整子区域大小的分隔容器。
 * 拖拽逻辑与样式与 modes/LayoutRenderer 的 SplitView 一致（复用其 .row/.col/.resizerV/.resizerH），
 * 但脱离面板注册表/PanelCtx，便于在普通页面中直接使用。
 *
 * - dir='row'：左右排列，插入纵向可拖拽分隔条（.resizerV）
 * - dir='col'：上下排列，插入横向可拖拽分隔条（.resizerH）
 * - storageKey：拖拽后的比例持久化到 localStorage（opennexus.split.<storageKey>）
 * - children 数量即子区域数量，默认每个 flex=1
 */
const SPLIT_STORE_PREFIX = 'opennexus.split.'
const MIN_FLEX = 0.15

interface SplitPaneProps {
  dir: 'row' | 'col'
  storageKey: string
  defaultFlexes?: number[]
  children: ReactNode[]
}

export default function SplitPane({ dir, storageKey, defaultFlexes, children }: SplitPaneProps) {
  const isRow = dir === 'row'
  const containerRef = useRef<HTMLDivElement>(null)
  const count = children.length
  const key = useMemo(() => storageKey, [storageKey])

  const [flexes, setFlexes] = useState<number[]>(() => loadFlexes(key, count, defaultFlexes))

  useEffect(() => {
    setFlexes(loadFlexes(key, count, defaultFlexes))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, count])

  function startDrag(index: number, e: ReactMouseEvent) {
    e.preventDefault()
    const container = containerRef.current
    if (!container) return
    const rect = container.getBoundingClientRect()
    const size = isRow ? rect.width : rect.height
    if (size <= 0) return
    const startPos = isRow ? e.clientX : e.clientY
    const startFlexes = flexes.slice()
    const total = startFlexes.reduce((a, b) => a + b, 0)
    const flexPerPx = total / size
    let current = startFlexes

    function onMove(ev: MouseEvent) {
      const pos = isRow ? ev.clientX : ev.clientY
      let delta = (pos - startPos) * flexPerPx
      // 夹紧：两侧都不小于 MIN_FLEX
      delta = Math.max(delta, MIN_FLEX - startFlexes[index])
      delta = Math.min(delta, startFlexes[index + 1] - MIN_FLEX)
      const next = startFlexes.slice()
      next[index] = startFlexes[index] + delta
      next[index + 1] = startFlexes[index + 1] - delta
      current = next
      setFlexes(next)
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      try {
        localStorage.setItem(SPLIT_STORE_PREFIX + key, JSON.stringify(current))
      } catch {
        /* ignore */
      }
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    document.body.style.cursor = isRow ? 'col-resize' : 'row-resize'
    document.body.style.userSelect = 'none'
  }

  return (
    // flex:1 使分栏容器填满 flex 父容器（否则只按内容宽度，右侧会留空白）
    <div ref={containerRef} className={isRow ? layoutStyles.row : layoutStyles.col} style={{ flex: 1, minWidth: 0, minHeight: 0 }}>
      {children.map((child, i) => (
        <Fragment key={i}>
          <div style={{ flex: flexes[i] ?? 1, minWidth: 0, minHeight: 0 }}>{child}</div>
          {i < children.length - 1 && (
            <div
              className={isRow ? layoutStyles.resizerV : layoutStyles.resizerH}
              onMouseDown={(e) => startDrag(i, e)}
              role="separator"
              aria-orientation={isRow ? 'vertical' : 'horizontal'}
            />
          )}
        </Fragment>
      ))}
    </div>
  )
}

function loadFlexes(key: string, count: number, defaultFlexes?: number[]): number[] {
  const fallback = defaultFlexes && defaultFlexes.length === count
    ? defaultFlexes.slice()
    : Array.from({ length: count }, () => 1)
  try {
    const raw = localStorage.getItem(SPLIT_STORE_PREFIX + key)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length === count && arr.every((v) => typeof v === 'number' && v > 0)) {
        return arr
      }
    }
  } catch {
    /* ignore */
  }
  return fallback
}
