import type { Session, Message, AgentCommand, ConfigOption, SessionMode, AgentSkill, Execution, RunningTask } from '../types'
import { apiFetch } from './client'

// 创建会话（可选 model_value 指定初始模型，可选 workspace_id）
export function createSession(agentType: string, workspaceId?: number, modelValue?: string): Promise<{ data: Session }> {
  return apiFetch('/sessions', {
    method: 'POST',
    body: JSON.stringify({ agent_type: agentType, workspace_id: workspaceId || 0, model_value: modelValue || '' }),
  })
}

// 获取会话列表（可选 source 过滤：manual / scheduled / classify）
export function listSessions(source?: 'manual' | 'scheduled' | 'classify'): Promise<{ data: { sessions: Session[] } }> {
  const qs = source ? `?source=${source}` : ''
  return apiFetch(`/sessions${qs}`)
}

// 获取当前用户正在运行的会话 db_session_id 列表（侧边栏运行状态图标用）
export function listRunningSessions(): Promise<{ data: { db_session_ids: number[] } }> {
  return apiFetch('/sessions/running')
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

// 彻底删除会话及其消息
export function deleteSession(id: number): Promise<void> {
  return apiFetch(`/sessions/${id}`, { method: 'DELETE' })
}

// 取消会话当前 prompt
export function cancelSession(id: number): Promise<void> {
  return apiFetch(`/sessions/${id}/cancel`, { method: 'POST' })
}

// 恢复/重开会话
export function resumeSession(id: number): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}/resume`, {
    method: 'POST',
  })
}

// 清理会话上下文：重置底层 ACP 会话，token 占用归零，保留会话与历史消息
export function clearContext(id: number): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}/clear-context`, {
    method: 'POST',
  })
}

// 获取会话消息历史
export function listMessages(id: number): Promise<{ data: { messages: Message[] } }> {
  return apiFetch(`/sessions/${id}/messages`)
}

// 获取会话执行块（定时任务 / 笔记分类）
export function listSessionExecutions(id: number): Promise<{ data: { executions: Execution[] } }> {
  return apiFetch(`/sessions/${id}/executions`)
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

// 切换会话模式（ask / agent / edit 等）
export function setSessionMode(id: number, modeId: string): Promise<void> {
  return apiFetch(`/sessions/${id}/mode`, {
    method: 'POST',
    body: JSON.stringify({ mode_id: modeId }),
  })
}

// 响应 Agent 权限请求（编辑文件、运行命令等）
export function respondPermission(
  id: number,
  requestId: string,
  optionId: string,
  cancelled = false,
): Promise<void> {
  return apiFetch(`/sessions/${id}/permissions/${encodeURIComponent(requestId)}/respond`, {
    method: 'POST',
    body: JSON.stringify({ option_id: optionId, cancelled }),
  })
}

// 获取会话因服务重启而中断的任务列表
export function getInterruptedTasks(id: number): Promise<{ data: { tasks: RunningTask[] } }> {
  return apiFetch(`/sessions/${id}/interrupted-tasks`)
}

// 恢复中断的任务（ResumeSession + 重发原 prompt）
export function resumeInterruptedTaskURL(taskId: number): string {
  return `/running-tasks/${taskId}/resume`
}

export interface DebugMeta {
  enabled: boolean
  dir: string
  event_count: number
  raw_count: number
  last_ts?: string
}

export interface DebugEvent {
  ts: string
  event: string
  session_id?: string
  db_session_id?: string
  detail?: unknown
}

export interface DebugRaw {
  ts: string
  direction: 'send' | 'recv' | string
  session_id?: string
  db_session_id?: string
  line: unknown
}

export function getDebugMeta(id: number): Promise<{ data: DebugMeta }> {
  return apiFetch(`/sessions/${id}/debug`)
}

export function listDebugEvents(
  id: number,
  since = 0,
  limit = 200,
): Promise<{ data: { events: DebugEvent[] } }> {
  return apiFetch(`/sessions/${id}/debug/events?since=${since}&limit=${limit}`)
}

export function listDebugRaw(
  id: number,
  since = 0,
  limit = 0,
): Promise<{ data: { raw: DebugRaw[] } }> {
  return apiFetch(`/sessions/${id}/debug/raw?since=${since}&limit=${limit}`)
}

