import { useState, useEffect, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import styles from './LoginPage.module.css'

export default function LoginPage() {
  const { user, loading, login, register } = useAuth()
  const navigate = useNavigate()

  // 如果用户已登录，跳转到会话列表页
  useEffect(() => {
    if (!loading && user) {
      navigate('/sessions', { replace: true })
    }
  }, [user, loading, navigate])

  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [account, setAccount] = useState('')
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setSubmitting(true)

    try {
      if (mode === 'login') {
        await login(account, password)
      } else {
        await register(username, email, password)
      }
      navigate('/sessions', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={styles.container}>
      <div className={styles.card}>
        <h1 className={styles.title}>NexusAgent</h1>
        <p className={styles.subtitle}>编码 Agent 编排平台</p>

        <div className={styles.tabs}>
          <button
            className={`${styles.tab} ${mode === 'login' ? styles.tabActive : ''}`}
            onClick={() => setMode('login')}
            type="button"
          >
            登录
          </button>
          <button
            className={`${styles.tab} ${mode === 'register' ? styles.tabActive : ''}`}
            onClick={() => setMode('register')}
            type="button"
          >
            注册
          </button>
        </div>

        <form className={styles.form} onSubmit={handleSubmit}>
          {mode === 'login' ? (
            <div className={styles.field}>
              <label className={styles.label}>用户名或邮箱</label>
              <input
                className={styles.input}
                type="text"
                value={account}
                onChange={(e) => setAccount(e.target.value)}
                required
                placeholder="输入用户名或邮箱"
              />
            </div>
          ) : (
            <>
              <div className={styles.field}>
                <label className={styles.label}>用户名</label>
                <input
                  className={styles.input}
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  placeholder="输入用户名"
                />
              </div>
              <div className={styles.field}>
                <label className={styles.label}>邮箱</label>
                <input
                  className={styles.input}
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  placeholder="输入邮箱"
                />
              </div>
            </>
          )}

          <div className={styles.field}>
            <label className={styles.label}>密码</label>
            <input
              className={styles.input}
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              placeholder="输入密码"
            />
          </div>

          {error && <div className={styles.error}>{error}</div>}

          <button className={styles.submitBtn} type="submit" disabled={submitting}>
            {submitting ? '处理中...' : mode === 'login' ? '登录' : '注册'}
          </button>
        </form>
      </div>
    </div>
  )
}
