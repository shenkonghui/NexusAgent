import { useState, useEffect, createContext, useContext, type ReactNode, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'
import { PanelLeftOpen } from 'lucide-react'
import SessionSidebar from './SessionSidebar'
import styles from './AppLayout.module.css'

// 整体隐藏/展开侧边栏的状态，独立于 SessionSidebar 内部分组折叠状态
const STORAGE_KEY = 'opennexus.sidebar.hidden'

function loadHidden(): boolean {
  try { return localStorage.getItem(STORAGE_KEY) === '1' } catch { return false }
}

interface SidebarContextValue {
  collapsed: boolean
  toggle: () => void
}

const SidebarContext = createContext<SidebarContextValue>({ collapsed: false, toggle: () => {} })

export function useSidebar() {
  return useContext(SidebarContext)
}

/**
 * 折叠状态下显示的展开按钮（放在各页面 header 左侧）。
 * 仅在侧边栏被隐藏时渲染，与原 ChatPage 行为一致。
 */
export function SidebarToggleButton() {
  const { t } = useTranslation()
  const { collapsed, toggle } = useSidebar()
  if (!collapsed) return null
  return (
    <button className={styles.iconBtn} onClick={toggle} type="button" title={t('common.open') + ' (⌘B)'}>
      <PanelLeftOpen size={18} />
    </button>
  )
}

interface AppLayoutProps {
  // 透传给 SessionSidebar 的 props（onCollapse 由本组件自动注入，不可外部覆盖）
  sidebarProps: Omit<ComponentProps<typeof SessionSidebar>, 'onCollapse'>
  children: ReactNode
}

/**
 * 全局共享布局：统一渲染左侧侧边栏、管理折叠/展开状态与 ⌘B 快捷键。
 * 各页面通过 sidebarProps 透传数据/回调，children 即右侧主内容区。
 */
export default function AppLayout({ sidebarProps, children }: AppLayoutProps) {
  const [collapsed, setCollapsed] = useState(loadHidden)

  // 持久化整体折叠状态，使各页面切换后保持一致
  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, collapsed ? '1' : '0') } catch { /* ignore */ }
  }, [collapsed])

  // Cmd/Ctrl+B 切换侧边栏隐藏/展开（由各页面统一收口至此）
  useEffect(() => {
    function handleToggle(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'b') {
        e.preventDefault()
        setCollapsed((v) => !v)
      }
    }
    document.addEventListener('keydown', handleToggle)
    return () => document.removeEventListener('keydown', handleToggle)
  }, [])

  function toggle() { setCollapsed((v) => !v) }

  return (
    <SidebarContext.Provider value={{ collapsed, toggle }}>
      <div className={styles.layout}>
        {!collapsed && (
          <div className={styles.sidebarWrap}>
            <SessionSidebar {...sidebarProps} onCollapse={() => setCollapsed(true)} />
          </div>
        )}
        {children}
      </div>
    </SidebarContext.Provider>
  )
}
