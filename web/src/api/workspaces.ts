import type { Workspace, Session } from '../types'
import { apiFetch } from './client'

export function createWorkspace(name: string, cwd: string): Promise<{ data: Workspace }> {
  return apiFetch('/workspaces', {
    method: 'POST',
    body: JSON.stringify({ name, cwd }),
  })
}

export function listWorkspaces(): Promise<{ data: { workspaces: (Workspace & { session_count: number })[] } }> {
  return apiFetch('/workspaces')
}

export function getWorkspace(id: number): Promise<{ data: { workspace: Workspace; sessions: Session[] } }> {
  return apiFetch(`/workspaces/${id}`)
}

export function updateWorkspace(id: number, name: string): Promise<{ data: Workspace }> {
  return apiFetch(`/workspaces/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  })
}

export function deleteWorkspace(id: number): Promise<void> {
  return apiFetch(`/workspaces/${id}`, { method: 'DELETE' })
}

export function saveWorkspace(id: number, name: string, cwd: string): Promise<{ data: Workspace }> {
  return apiFetch(`/workspaces/${id}/save`, {
    method: 'POST',
    body: JSON.stringify({ name, cwd }),
  })
}
