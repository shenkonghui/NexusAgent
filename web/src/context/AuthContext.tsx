import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'
import type { User } from '../types'
import * as authApi from '../api/auth'
import LoadingSpinner from '../components/LoadingSpinner'

// 认证状态
interface AuthState {
  user: User | null
  loading: boolean
}

// 认证上下文值
interface AuthContextValue extends AuthState {
  login: (account: string, password: string) => Promise<void>
  register: (username: string, email: string, password: string) => Promise<void>
  logout: () => Promise<void>
  refreshUser: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

// AuthProvider 提供认证状态管理
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  // 启动时验证 token；无 token 或 token 失效时尝试 auto_login
  useEffect(() => {
    async function initAuth() {
      try {
        const token = localStorage.getItem('access_token')
        if (token) {
          try {
            const resp = await authApi.getMe({ skipAuthRedirect: true })
            setUser(resp.data)
            return
          } catch {
            localStorage.removeItem('access_token')
            localStorage.removeItem('refresh_token')
          }
        }
        localStorage.removeItem('access_token')
        localStorage.removeItem('refresh_token')
        try {
          const resp = await authApi.autoLogin()
          localStorage.setItem('access_token', resp.access_token)
          localStorage.setItem('refresh_token', resp.refresh_token)
          setUser(resp.user)
        } catch {
          // auto_login 未启用或失败，展示登录页
        }
      } finally {
        setLoading(false)
      }
    }
    void initAuth()
  }, [])

  const login = useCallback(async (account: string, password: string) => {
    const resp = await authApi.login(account, password)
    localStorage.setItem('access_token', resp.access_token)
    localStorage.setItem('refresh_token', resp.refresh_token)
    setUser(resp.user)
  }, [])

  const register = useCallback(async (username: string, email: string, password: string) => {
    const resp = await authApi.register(username, email, password)
    localStorage.setItem('access_token', resp.access_token)
    localStorage.setItem('refresh_token', resp.refresh_token)
    setUser(resp.user)
  }, [])

  const logout = useCallback(async () => {
    try {
      await authApi.logout()
    } catch {
      // 忽略登出 API 错误
    }
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
    setUser(null)
  }, [])

  // 重新拉取当前用户信息（个人中心更新后刷新）
  const refreshUser = useCallback(async () => {
    const resp = await authApi.getMe()
    setUser(resp.data)
  }, [])

  const value: AuthContextValue = { user, loading, login, register, logout, refreshUser }

  if (loading) {
    return <LoadingSpinner />
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

// useAuthContext 消费认证上下文（内部使用）
export function useAuthContext(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuthContext 必须在 AuthProvider 内使用')
  }
  return ctx
}
