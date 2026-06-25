import type { ScheduledTask, Execution } from '../types'
import { apiFetch } from './client'

// 创建定时任务
export function createScheduledTask(payload: {
  name: string
  agent_type: string
  cwd?: string
  prompt: string
  cron_expr: string
  enabled?: boolean
  model_value?: string
  timeout_minutes?: number
}): Promise<{ data: ScheduledTask }> {
  return apiFetch('/scheduled-tasks', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

// 列出定时任务
export function listScheduledTasks(): Promise<{ data: { tasks: ScheduledTask[] } }> {
  return apiFetch('/scheduled-tasks')
}

// 获取单个定时任务
export function getScheduledTask(id: number): Promise<{ data: ScheduledTask }> {
  return apiFetch(`/scheduled-tasks/${id}`)
}

// 更新定时任务
export function updateScheduledTask(
  id: number,
  payload: Partial<{
    name: string
    agent_type: string
    cwd: string
    prompt: string
    cron_expr: string
    enabled: boolean
    model_value: string
    timeout_minutes: number
  }>,
): Promise<{ data: ScheduledTask }> {
  return apiFetch(`/scheduled-tasks/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

// 删除定时任务
export function deleteScheduledTask(id: number): Promise<void> {
  return apiFetch(`/scheduled-tasks/${id}`, { method: 'DELETE' })
}

// 手动触发一次执行
export function runScheduledTask(id: number): Promise<void> {
  return apiFetch(`/scheduled-tasks/${id}/run`, { method: 'POST' })
}

// 获取定时任务执行历史
export function listExecutions(id: number): Promise<{ data: { executions: Execution[] } }> {
  return apiFetch(`/scheduled-tasks/${id}/executions`)
}
