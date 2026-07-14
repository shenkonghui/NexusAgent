const DEFAULT_MAX = 10

export function truncateSelectLabel(text: string, max = DEFAULT_MAX): string {
  const trimmed = text.trim()
  if (trimmed.length <= max) return trimmed
  return `${trimmed.slice(0, max - 1)}…`
}

export function formatOptionLabel(name: string, description?: string, max?: number): string {
  const full = description ? `${name} — ${description}` : name
  return max != null ? truncateSelectLabel(full, max) : full
}

export function fullOptionLabel(name: string, description?: string): string {
  return description ? `${name} — ${description}` : name
}
