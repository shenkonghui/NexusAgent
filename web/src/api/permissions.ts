import type { PermissionSettings } from '../types'
import { apiFetch } from './client'

export function getPermissionSettings(): Promise<{ data: PermissionSettings }> {
  return apiFetch('/permissions/settings')
}

export function updatePermissionSettings(payload: PermissionSettings): Promise<{ data: PermissionSettings }> {
  return apiFetch('/permissions/settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}
