import { useState, useEffect, useCallback } from 'react'
import { listWorkspaces, getWorkspace } from '../api/workspaces'
import type { Workspace, Session } from '../types'

export const WORKSPACE_STORAGE_KEY = 'nexus.current.workspace'

export function resolveWorkspaceId(
  workspaces: (Workspace & { session_count?: number })[],
  stored: string | null,
): number {
  if (stored) {
    const id = Number(stored)
    if (workspaces.some((w) => w.id === id)) return id
  }
  const persistent = workspaces.find((w) => w.mode === 'persistent')
  return persistent?.id ?? workspaces[0]?.id ?? 0
}

export function useCurrentWorkspace(enabled = true) {
  const [workspaces, setWorkspaces] = useState<(Workspace & { session_count?: number })[]>([])
  const [workspaceId, setWorkspaceIdState] = useState(0)
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(true)

  const persistId = useCallback((id: number) => {
    setWorkspaceIdState(id)
    localStorage.setItem(WORKSPACE_STORAGE_KEY, String(id))
  }, [])

  const loadSessions = useCallback(async (id: number) => {
    if (!id) { setSessions([]); return }
    const detail = await getWorkspace(id)
    setSessions(detail.data.sessions || [])
  }, [])

  const reload = useCallback(async () => {
    if (!enabled) return
    setLoading(true)
    try {
      const list = (await listWorkspaces()).data.workspaces || []
      setWorkspaces(list)
      const id = resolveWorkspaceId(list, localStorage.getItem(WORKSPACE_STORAGE_KEY))
      persistId(id)
      await loadSessions(id)
    } finally {
      setLoading(false)
    }
  }, [enabled, loadSessions, persistId])

  useEffect(() => { reload() }, [reload])

  const selectWorkspace = useCallback(async (id: number) => {
    persistId(id)
    await loadSessions(id)
  }, [loadSessions, persistId])

  return { workspaces, workspaceId, sessions, loading, reload, selectWorkspace }
}
