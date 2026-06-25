import type { User, AuthResponse } from '../types'
import { apiFetch } from './client'

// 注册
export async function register(username: string, email: string, password: string): Promise<AuthResponse> {
  const resp = await apiFetch<{ data: AuthResponse }>('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, email, password }),
  })
  return resp.data
}

// 登录
export async function login(account: string, password: string): Promise<AuthResponse> {
  const resp = await apiFetch<{ data: AuthResponse }>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ account, password }),
  })
  return resp.data
}

// 登出
export function logout(): Promise<void> {
  return apiFetch('/auth/logout', { method: 'POST' })
}

// 获取当前用户信息
export function getMe(): Promise<{ data: User }> {
  return apiFetch('/me')
}
