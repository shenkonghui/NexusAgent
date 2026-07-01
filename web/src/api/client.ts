import type { ApiError } from '../types'

// baseURL 从环境变量读取，默认 /api/v1
const BASE_URL = import.meta.env.VITE_API_BASE || '/api/v1'

// 获取存储的 access token
function getAccessToken(): string | null {
  return localStorage.getItem('access_token')
}

// 获取存储的 refresh token
function getRefreshToken(): string | null {
  return localStorage.getItem('refresh_token')
}

// 用 refresh token 刷新 access token
async function refreshAccessToken(): Promise<boolean> {
  const refreshToken = getRefreshToken()
  if (!refreshToken) return false

  try {
    const resp = await fetch(`${BASE_URL}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
    if (!resp.ok) return false

    const body = await resp.json()
    const data = body.data ?? body
    localStorage.setItem('access_token', data.access_token)
    localStorage.setItem('refresh_token', data.refresh_token)
    return true
  } catch {
    return false
  }
}

export type ApiFetchOptions = RequestInit & {
  // 401 且 refresh 失败时不硬跳转 /login，留给 AuthContext 尝试 auto_login
  skipAuthRedirect?: boolean
}

// 清除 token；可选跳转登录页
export function clearTokensAndRedirect(redirect = true): void {
  localStorage.removeItem('access_token')
  localStorage.removeItem('refresh_token')
  if (redirect) {
    window.location.href = '/login'
  }
}

// 带认证的 fetch 封装
export async function apiFetch<T>(
  path: string,
  options: ApiFetchOptions = {},
): Promise<T> {
  const { skipAuthRedirect, ...fetchOptions } = options
  const token = getAccessToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...fetchOptions.headers as Record<string, string>,
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  let resp = await fetch(`${BASE_URL}${path}`, { ...fetchOptions, headers })

  // 401 时尝试刷新 token 并重试
  if (resp.status === 401) {
    const refreshed = await refreshAccessToken()
    if (refreshed) {
      const newToken = getAccessToken()
      if (newToken) {
        headers['Authorization'] = `Bearer ${newToken}`
      }
      resp = await fetch(`${BASE_URL}${path}`, { ...fetchOptions, headers })
    } else {
      clearTokensAndRedirect(!skipAuthRedirect)
      throw new Error('认证已过期，请重新登录')
    }
  }

  if (!resp.ok) {
    let errMsg = `请求失败 (${resp.status})`
    let errCode = 'UNKNOWN'
    try {
      const errBody: ApiError = await resp.json()
      if (errBody.error) {
        errMsg = errBody.error.message
        errCode = errBody.error.code
      }
    } catch {
      // 响应体非 JSON，使用默认错误消息
    }
    const err = new Error(errMsg) as Error & { code: string }
    err.code = errCode
    throw err
  }

  // 204 No Content
  if (resp.status === 204) {
    return undefined as T
  }

  return resp.json()
}

// 获取认证头（供 SSE 使用）
export function getAuthHeaders(): Record<string, string> {
  const token = getAccessToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  return headers
}

// 获取 baseURL（供 SSE 使用）
export function getBaseURL(): string {
  return BASE_URL
}
