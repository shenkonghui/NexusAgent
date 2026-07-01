import { apiFetch } from './client'
import type { AgentCommand, AgentSkill } from '../types'

// 目录项
export interface DirEntry {
  name: string
  path: string
}

// 目录列表响应
export interface DirListResponse {
  current_path: string
  parent_path: string
  dirs: DirEntry[]
}

// 文件/目录项（用于 @ 文件引用）
export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
}

// 文件列表响应
export interface FileListResponse {
  current_path: string
  parent_path: string
  entries: FileEntry[]
}

// 列出指定路径下的子目录（path 为空时从用户主目录开始）
export function listDirs(path?: string): Promise<{ data: DirListResponse }> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/filesystem/dirs${query}`)
}

// 列出指定路径下的文件和目录（用于 @ 文件引用补全）
export function listFiles(path?: string, query?: string): Promise<{ data: FileListResponse }> {
  const params = new URLSearchParams()
  if (path) params.set('path', path)
  if (query) params.set('query', query)
  const qs = params.toString()
  return apiFetch(`/filesystem/list${qs ? `?${qs}` : ''}`)
}

// 扫描指定目录下的 Slash Commands（Claude Code 规范；path 为空时仅扫用户目录）
export function listCommandsByPath(path?: string): Promise<{ data: { commands: AgentCommand[] } }> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/filesystem/commands${qs}`)
}

// 扫描指定目录下的 Agent Skills（新建任务页 / 命令补全用；path 为空时仅扫用户目录）
export function listSkillsByPath(path?: string): Promise<{ data: { skills: AgentSkill[] } }> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/filesystem/skills${qs}`)
}

// ===== 会话工作目录文件 API =====

// 会话文件树节点
export interface SessionFileEntry {
  name: string
  path: string // 相对 session cwd 的路径
  is_dir: boolean
  size?: number
}

// 会话文件树响应
export interface SessionFileTreeResponse {
  cwd: string
  path: string
  entries: SessionFileEntry[]
}

// 会话文件内容响应
export interface SessionFileContentResponse {
  path: string
  content: string
  size: number
}

// 列出会话工作目录下的文件和目录（单层，懒加载）
export function listSessionFiles(sessionId: number, path?: string): Promise<{ data: SessionFileTreeResponse }> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/sessions/${sessionId}/files${query}`)
}

// 读取会话工作目录下的文件内容
export function readSessionFile(sessionId: number, path: string): Promise<{ data: SessionFileContentResponse }> {
  return apiFetch(`/sessions/${sessionId}/files/content?path=${encodeURIComponent(path)}`)
}

// 保存文件内容到会话工作目录
export function writeSessionFile(sessionId: number, path: string, content: string): Promise<{ data: { path: string; size: number } }> {
  return apiFetch(`/sessions/${sessionId}/files/content`, {
    method: 'PUT',
    body: JSON.stringify({ path, content }),
  })
}
