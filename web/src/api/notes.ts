import type { Note, NoteSettings } from '../types'
import { apiFetch } from './client'

export function getNoteSettings(): Promise<{ data: NoteSettings }> {
  return apiFetch('/notes/settings')
}

export function updateNoteSettings(payload: NoteSettings): Promise<{ data: NoteSettings }> {
  return apiFetch('/notes/settings', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

export function listNotes(tag?: string): Promise<{ data: { notes: Note[] } }> {
  const qs = tag ? `?tag=${encodeURIComponent(tag)}` : ''
  return apiFetch(`/notes${qs}`)
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
