import type { DocFolder } from '../types'

// localStorage key：所有文档文件夹绑定（跨工作区存储）
export const DOCS_KEY = 'opennexus.documents'

// 顶层任务模式：coding / docs
export const TASK_MODE_KEY = 'opennexus.taskMode'

// 文档模式下「上次打开的文档」按工作区记忆的前缀
export const LAST_DOC_KEY_PREFIX = 'opennexus.lastDoc.'

// 文档模式「工作区级共享会话」按工作区记忆的前缀。
// 同一工作区的所有文档共用同一个 session，历史可跨文档 / 跨刷新恢复。
export const DOC_SESSION_KEY_PREFIX = 'opennexus.docSession.'

// 读取某工作区的文档共享会话 id（无则返回 null）
export function loadDocSession(workspaceId: number): number | null {
  try {
    const raw = localStorage.getItem(DOC_SESSION_KEY_PREFIX + (workspaceId || 0))
    if (raw) {
      const id = Number(raw)
      if (!isNaN(id) && id > 0) return id
    }
  } catch { /* ignore */ }
  return null
}

// 记忆某工作区的文档共享会话 id
export function saveDocSession(workspaceId: number, sessionId: number): void {
  try { localStorage.setItem(DOC_SESSION_KEY_PREFIX + (workspaceId || 0), String(sessionId)) } catch { /* ignore */ }
}

// 清除某工作区的文档共享会话记忆（会话被删除 / 失效时）
export function clearDocSession(workspaceId: number): void {
  try { localStorage.removeItem(DOC_SESSION_KEY_PREFIX + (workspaceId || 0)) } catch { /* ignore */ }
}

// 从 localStorage 加载所有文档文件夹绑定
export function loadDocFolders(): DocFolder[] {
  try {
    const raw = localStorage.getItem(DOCS_KEY)
    if (raw) return JSON.parse(raw) as DocFolder[]
  } catch { /* ignore */ }
  return []
}

// 保存文档文件夹绑定到 localStorage
export function saveDocFolders(docs: DocFolder[]): void {
  try { localStorage.setItem(DOCS_KEY, JSON.stringify(docs)) } catch { /* ignore */ }
}

// 文档目标（folderId + filePath），用于任务页文档模式打开指定文档
export interface DocTarget {
  folderId: string
  filePath: string
}

// 读取某工作区上次打开的文档
export function loadLastDoc(workspaceId: number): DocTarget | null {
  try {
    const raw = localStorage.getItem(LAST_DOC_KEY_PREFIX + (workspaceId || 0))
    if (raw) return JSON.parse(raw) as DocTarget
  } catch { /* ignore */ }
  return null
}

// 记忆某工作区上次打开的文档
export function saveLastDoc(workspaceId: number, doc: DocTarget): void {
  try {
    localStorage.setItem(LAST_DOC_KEY_PREFIX + (workspaceId || 0), JSON.stringify(doc))
  } catch { /* ignore */ }
}
