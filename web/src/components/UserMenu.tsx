import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthContext } from '../context/AuthContext'
import { useTheme } from '../context/ThemeContext'
import styles from './UserMenu.module.css'

// UserMenu 是右上角用户菜单：显示用户名，点击展开下拉菜单（个人中心、退出登录）。
export default function UserMenu() {
  const { user, logout } = useAuthContext()
  const { theme, toggleTheme } = useTheme()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  // 点击外部关闭菜单
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  if (!user) return null

  async function handleLogout() {
    setOpen(false)
    await logout()
    navigate('/login')
  }

  function handleProfile() {
    setOpen(false)
    navigate('/profile')
  }

  function handleToggleTheme() {
    toggleTheme()
  }

  return (
    <div className={styles.container} ref={ref}>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => setOpen((v) => !v)}
      >
        <span className={styles.avatar}>{user.username.charAt(0).toUpperCase()}</span>
        <span className={styles.username}>{user.username}</span>
        <span className={styles.arrow}>{open ? '▲' : '▼'}</span>
      </button>
      {open && (
        <div className={styles.dropdown}>
          <button type="button" className={styles.menuItem} onClick={handleProfile}>
            <span className={styles.menuIcon}>👤</span>
            个人中心
          </button>
          <button type="button" className={styles.menuItem} onClick={handleToggleTheme}>
            <span className={styles.menuIcon}>{theme === 'dark' ? '☀️' : '🌙'}</span>
            {theme === 'dark' ? '浅色模式' : '深色模式'}
          </button>
          <div className={styles.divider} />
          <button type="button" className={styles.menuItem} onClick={handleLogout}>
            <span className={styles.menuIcon}>🚪</span>
            退出登录
          </button>
        </div>
      )}
    </div>
  )
}
