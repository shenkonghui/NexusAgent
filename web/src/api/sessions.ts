import type { Session, Message, AgentCommand, ConfigOption, SessionMode, AgentSkill } from '../types'
import { apiFetch } from './client'

// 创建会话
export function createSession(agentType: string, cwd?: string): Promise<{ data: Session }> {
  return apiFetch('/sessions', {
    method: 'POST',
    body: JSON.stringify({ agent_type: agentType, cwd: cwd || '' }),
  })
}

// 获取会话列表（可选 source 过滤：manual / scheduled）
export function listSessions(source?: 'manual' | 'scheduled'): Promise<{ data: { sessions: Session[] } }> {
  const qs = source ? `?source=${source}` : ''
  return apiFetch(`/sessions${qs}`)
}

// 获取单个会话
export function getSession(id: number): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}`)
}

// 更新会话标题
export function updateSessionTitle(id: number, title: string): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}/title`, {
    method: 'PUT',
    body: JSON.stringify({ title }),
  })
}

// 关闭会话
export function closeSession(id: number): Promise<void> {
  return apiFetch(`/sessions/${id}`, { method: 'DELETE' })
}

// 彻底删除会话及其消息
export function deleteSession(id: number): Promise<void> {
  return apiFetch(`/sessions/${id}/delete`, { method: 'POST' })
}

// 取消会话当前 prompt
export function cancelSession(id: number): Promise<void> {
  return apiFetch(`/sessions/${id}/cancel`, { method: 'POST' })
}

// 恢复/重开会话，可选 cwd 用于指定新工作目录
export function resumeSession(id: number, cwd?: string): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}/resume`, {
    method: 'POST',
    body: JSON.stringify({ cwd: cwd || '' }),
  })
}

// 获取会话消息历史
export function listMessages(id: number): Promise<{ data: { messages: Message[] } }> {
  return apiFetch(`/sessions/${id}/messages`)
}

// 获取会话可用的 slash command
export function listCommands(id: number): Promise<{ data: { commands: AgentCommand[] } }> {
  return apiFetch(`/sessions/${id}/commands`)
}

// 获取会话可用的 mode 列表（ACP 会话模式）
export function listModes(id: number): Promise<{ data: { modes: SessionMode[] } }> {
  return apiFetch(`/sessions/${id}/modes`)
}

// 获取会话工作目录下发现的 Agent Skills（agentskills.io 规范）
export function listSkills(id: number): Promise<{ data: { skills: AgentSkill[] } }> {
  return apiFetch(`/sessions/${id}/skills`)
}

// 获取会话的 config option（含模型选择）
export function listConfigOptions(id: number): Promise<{ data: { config_options: ConfigOption[] } }> {
  return apiFetch(`/sessions/${id}/config-options`)
}

// 设置会话的 config option 值（如切换模型）
export function setConfigOption(id: number, configId: string, value: string): Promise<void> {
  return apiFetch(`/sessions/${id}/config-options`, {
    method: 'POST',
    body: JSON.stringify({ config_id: configId, value }),
  })
}
