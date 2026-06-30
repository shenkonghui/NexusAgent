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

// 更新当前用户名与邮箱
export function updateProfile(username: string, email: string): Promise<{ data: User }> {
  return apiFetch('/me', {
    method: 'PUT',
    body: JSON.stringify({ username, email }),
  })
}

// 自动登录（免登录）
export function autoLogin(): Promise<AuthResponse> {
  return apiFetch<{ data: AuthResponse }>('/auth/auto-login', { method: 'GET' }).then(r => r.data)
}

// 修改当前用户密码
export function changePassword(oldPassword: string, newPassword: string): Promise<void> {
  return apiFetch('/me/password', {
    method: 'POST',
    body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
  })
}
