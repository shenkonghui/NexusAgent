import type { User, AuthResponse } from '../types'
import { apiFetch } from './client'

// 注册
export function register(username: string, email: string, password: string): Promise<AuthResponse> {
  return apiFetch('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, email, password }),
  })
}

// 登录
export function login(account: string, password: string): Promise<AuthResponse> {
  return apiFetch('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ account, password }),
  })
}

// 登出
export function logout(): Promise<void> {
  return apiFetch('/auth/logout', { method: 'POST' })
}

// 获取当前用户信息
export function getMe(): Promise<{ data: User }> {
  return apiFetch('/me')
}
