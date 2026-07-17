import type { Note, NoteSettings } from '../types'
import { apiFetch, apiFetchRaw } from './client'

export function getNoteSettings(): Promise<{ data: NoteSettings }> {
  return apiFetch('/notes/settings')
}

export function updateNoteSettings(payload: NoteSettings): Promise<{ data: NoteSettings }> {
  return apiFetch('/notes/settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function generateNoteMCPToken(): Promise<{ data: { mcp_token: string } }> {
  return apiFetch('/notes/settings/mcp-token', { method: 'POST' })
}

export function listNotes(
  tag?: string,
  opts?: { q?: string; page?: number; limit?: number },
): Promise<{ data: { notes: Note[]; total: number; page: number; limit: number } }> {
  const params = new URLSearchParams()
  if (tag) params.set('tag', tag)
  if (opts?.q) params.set('q', opts.q)
  if (opts?.page) params.set('page', String(opts.page))
  if (opts?.limit) params.set('limit', String(opts.limit))
  const qs = params.toString()
  return apiFetch(`/notes${qs ? `?${qs}` : ''}`)
}

export function listNoteTags(): Promise<{ data: { tags: string[] } }> {
  return apiFetch('/notes/tags')
}

export function createNote(content: string): Promise<{ data: Note }> {
  return apiFetch('/notes', {
    method: 'POST',
    body: JSON.stringify({ content }),
  })
}

export function updateNote(id: number, content: string): Promise<{ data: Note }> {
  return apiFetch(`/notes/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  })
}

export function deleteNote(id: number): Promise<void> {
  return apiFetch(`/notes/${id}`, { method: 'DELETE' })
}

export function classifyNoteNow(id: number): Promise<{ data: Note }> {
  return apiFetch(`/notes/${id}/classify`, { method: 'POST' })
}

// 导出全部笔记为单个 Markdown 文件
export async function exportNotes(): Promise<Blob> {
  const resp = await apiFetchRaw('/notes/export')
  return resp.blob()
}

// 批量导入笔记（按内容去重）
export function importNotes(
  notes: { content: string; title?: string; tags?: string[] }[],
): Promise<{ data: { imported: number; skipped: number } }> {
  return apiFetch('/notes/import', {
    method: 'POST',
    body: JSON.stringify({ notes }),
  })
}
