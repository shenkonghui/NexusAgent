import type { Session, Message } from '../types'
import { apiFetch } from './client'

// 创建会话
export function createSession(agentType: string, cwd?: string): Promise<{ data: Session }> {
  return apiFetch('/sessions', {
    method: 'POST',
    body: JSON.stringify({ agent_type: agentType, cwd: cwd || '' }),
  })
}

// 获取会话列表
export function listSessions(): Promise<{ data: { sessions: Session[] } }> {
  return apiFetch('/sessions')
}

// 获取单个会话
export function getSession(id: number): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}`)
}

// 关闭会话
export function closeSession(id: number): Promise<void> {
  return apiFetch(`/sessions/${id}`, { method: 'DELETE' })
}

// 取消会话当前 prompt
export function cancelSession(id: number): Promise<void> {
  return apiFetch(`/sessions/${id}/cancel`, { method: 'POST' })
}

// 恢复会话
export function resumeSession(id: number): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}/resume`, { method: 'POST' })
}

// 获取会话消息历史
export function listMessages(id: number): Promise<{ data: { messages: Message[] } }> {
  return apiFetch(`/sessions/${id}/messages`)
}
