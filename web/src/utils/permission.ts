import type { PermissionOption, PermissionRequestPayload } from '../types'

export function parsePermissionRequest(rawJSON: string): PermissionRequestPayload | null {
  if (!rawJSON) return null
  try {
    const data = JSON.parse(rawJSON) as PermissionRequestPayload
    if (!data.request_id || !Array.isArray(data.options)) return null
    return data
  } catch {
    return null
  }
}

export function permissionOptionStyle(kind: string): 'allow' | 'reject' | 'neutral' {
  if (kind === 'allow_once' || kind === 'allow_always') return 'allow'
  if (kind === 'reject_once' || kind === 'reject_always') return 'reject'
  return 'neutral'
}

export function sortPermissionOptions(options: PermissionOption[]): PermissionOption[] {
  const order: Record<string, number> = {
    allow_once: 0,
    allow_always: 1,
    reject_once: 2,
    reject_always: 3,
  }
  return [...options].sort((a, b) => (order[a.kind] ?? 9) - (order[b.kind] ?? 9))
}
