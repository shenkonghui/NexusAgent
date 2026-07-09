import type { TaskSettings } from '../types'
import { apiFetch } from './client'

export function getTaskSettings(): Promise<{ data: TaskSettings }> {
  return apiFetch('/tasks/settings')
}

export function updateTaskSettings(payload: TaskSettings): Promise<{ data: TaskSettings }> {
  return apiFetch('/tasks/settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}
