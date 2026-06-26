import { useState, type FormEvent } from 'react'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { useAuthContext } from '../context/AuthContext'
import { updateProfile, changePassword } from '../api/auth'
import { listSessions } from '../api/sessions'
import type { Session } from '../types'
import { useEffect } from 'react'
import SessionSidebar from '../components/SessionSidebar'
import UserMenu from '../components/UserMenu'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import styles from './ProfilePage.module.css'

export default function ProfilePage() {
  const { user, loading: authLoading } = useRequireAuth()
  const { refreshUser } = useAuthContext()

  const [sessions, setSessions] = useState<Session[]>([])
  const [profileForm, setProfileForm] = useState({ username: '', email: '' })
  const [passwordForm, setPasswordForm] = useState({
    old_password: '',
    new_password: '',
    confirm_password: '',
  })
  const [savingProfile, setSavingProfile] = useState(false)
  const [savingPassword, setSavingPassword] = useState(false)
  const [error, setError] = useState('')
  const [profileMsg, setProfileMsg] = useState('')
  const [passwordMsg, setPasswordMsg] = useState('')

  useEffect(() => {
    if (user) {
      setProfileForm({ username: user.username, email: user.email })
      listSessions().then((r) => setSessions(r.data.sessions || [])).catch(() => {})
    }
  }, [user])

  async function handleProfileSubmit(e: FormEvent) {
    e.preventDefault()
    if (!profileForm.username || !profileForm.email) {
      setError('用户名与邮箱不能为空')
      return
    }
    setSavingProfile(true)
    setError('')
    setProfileMsg('')
    try {
      await updateProfile(profileForm.username, profileForm.email)
      await refreshUser()
      setProfileMsg('个人信息已更新')
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新失败')
    } finally {
      setSavingProfile(false)
    }
  }

  async function handlePasswordSubmit(e: FormEvent) {
    e.preventDefault()
    if (passwordForm.new_password !== passwordForm.confirm_password) {
      setError('两次输入的新密码不一致')
      return
    }
    if (passwordForm.new_password.length < 8) {
      setError('新密码至少 8 位，需含字母与数字')
      return
    }
    setSavingPassword(true)
    setError('')
    setPasswordMsg('')
    try {
      await changePassword(passwordForm.old_password, passwordForm.new_password)
      setPasswordMsg('密码已修改')
      setPasswordForm({ old_password: '', new_password: '', confirm_password: '' })
    } catch (err) {
      setError(err instanceof Error ? err.message : '修改密码失败')
    } finally {
      setSavingPassword(false)
    }
  }

  if (authLoading) return <LoadingSpinner text="验证登录状态..." />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <div className={styles.sidebarWrap}>
        <SessionSidebar sessions={sessions} />
      </div>

      <div className={styles.main}>
        <div className={styles.header}>
          <h1 className={styles.title}>个人中心</h1>
          <UserMenu />
        </div>

        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        <div className={styles.content}>
          {/* 账户信息展示 */}
          <section className={styles.section}>
            <h2 className={styles.sectionTitle}>账户信息</h2>
            <div className={styles.infoGrid}>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>用户 ID</span>
                <span className={styles.infoValue}>{user.id}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>角色</span>
                <span className={styles.infoValue}>{user.role === 'admin' ? '管理员' : '普通用户'}</span>
              </div>
              <div className={styles.infoItem}>
                <span className={styles.infoLabel}>状态</span>
                <span className={styles.infoValue}>{user.status === 'active' ? '正常' : '已禁用'}</span>
              </div>
            </div>
          </section>

          {/* 修改个人信息 */}
          <section className={styles.section}>
            <h2 className={styles.sectionTitle}>修改个人信息</h2>
            {profileMsg && <p className={styles.successMsg}>{profileMsg}</p>}
            <form className={styles.form} onSubmit={handleProfileSubmit}>
              <div className={styles.field}>
                <label className={styles.label}>用户名</label>
                <input
                  className={styles.input}
                  type="text"
                  value={profileForm.username}
                  onChange={(e) => setProfileForm({ ...profileForm, username: e.target.value })}
                  required
                />
              </div>
              <div className={styles.field}>
                <label className={styles.label}>邮箱</label>
                <input
                  className={styles.input}
                  type="email"
                  value={profileForm.email}
                  onChange={(e) => setProfileForm({ ...profileForm, email: e.target.value })}
                  required
                />
              </div>
              <button className={styles.submitBtn} type="submit" disabled={savingProfile}>
                {savingProfile ? '保存中...' : '保存'}
              </button>
            </form>
          </section>

          {/* 修改密码 */}
          <section className={styles.section}>
            <h2 className={styles.sectionTitle}>修改密码</h2>
            {passwordMsg && <p className={styles.successMsg}>{passwordMsg}</p>}
            <form className={styles.form} onSubmit={handlePasswordSubmit}>
              <div className={styles.field}>
                <label className={styles.label}>原密码</label>
                <input
                  className={styles.input}
                  type="password"
                  value={passwordForm.old_password}
                  onChange={(e) => setPasswordForm({ ...passwordForm, old_password: e.target.value })}
                  required
                  autoComplete="current-password"
                />
              </div>
              <div className={styles.field}>
                <label className={styles.label}>新密码</label>
                <input
                  className={styles.input}
                  type="password"
                  value={passwordForm.new_password}
                  onChange={(e) => setPasswordForm({ ...passwordForm, new_password: e.target.value })}
                  required
                  autoComplete="new-password"
                  placeholder="至少 8 位，含字母与数字"
                />
              </div>
              <div className={styles.field}>
                <label className={styles.label}>确认新密码</label>
                <input
                  className={styles.input}
                  type="password"
                  value={passwordForm.confirm_password}
                  onChange={(e) => setPasswordForm({ ...passwordForm, confirm_password: e.target.value })}
                  required
                  autoComplete="new-password"
                />
              </div>
              <button className={styles.submitBtn} type="submit" disabled={savingPassword}>
                {savingPassword ? '修改中...' : '修改密码'}
              </button>
            </form>
          </section>
        </div>
      </div>
    </div>
  )
}
