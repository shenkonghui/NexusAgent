import type { ConfigOption } from '../types'

// 注意：这两个 key 的值必须保留旧名（nexus.*），用于读取并迁移老用户浏览器里已存在的旧 localStorage 数据。
// 改名后老数据会读不到，迁移失效。
const LEGACY_MODELS_KEY = 'nexus.agent.models'
const LEGACY_DEFAULT_AGENT_KEY = 'nexus.default.agent'

/** 优先用已记忆的值（且仍在 options 中），否则回退探测默认。 */
export function resolveConfigValue(opt: ConfigOption, saved?: string): string {
  if (saved && opt.options.some((o) => o.value === saved)) return saved
  if (saved && opt.options.length === 0 && saved.trim()) return saved
  return opt.current_value || opt.options[0]?.value || ''
}

/** 把 agent 的 category→value 偏好应用到探测结果。 */
export function applyPrefsToConfigs(opts: ConfigOption[], agentPrefs?: Record<string, string>): ConfigOption[] {
  if (!agentPrefs || Object.keys(agentPrefs).length === 0) return opts
  return opts.map((o) => {
    if (o.type !== 'select') return o
    const next = resolveConfigValue(o, agentPrefs[o.category])
    return next === o.current_value ? o : { ...o, current_value: next }
  })
}

/** 从当前配置栏提取 category→value（仅 select）。 */
export function configsFromProbe(opts: ConfigOption[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const o of opts) {
    if (o.type !== 'select' || !o.category || !o.current_value) continue
    out[o.category] = o.current_value
  }
  return out
}

/** 读取并清除旧 localStorage 偏好，供首次迁移到服务端。 */
export function takeLegacyLocalAgentPrefs(): {
  last_agent_type: string
  prefs: Record<string, Record<string, string>>
} | null {
  let last = ''
  const prefs: Record<string, Record<string, string>> = {}
  try {
    last = localStorage.getItem(LEGACY_DEFAULT_AGENT_KEY) || ''
    const raw = localStorage.getItem(LEGACY_MODELS_KEY)
    if (raw) {
      const map = JSON.parse(raw) as Record<string, string>
      if (map && typeof map === 'object') {
        for (const [agent, model] of Object.entries(map)) {
          if (agent && model) prefs[agent] = { model }
        }
      }
    }
  } catch {
    /* ignore */
  }
  if (!last && Object.keys(prefs).length === 0) return null
  try {
    localStorage.removeItem(LEGACY_DEFAULT_AGENT_KEY)
    localStorage.removeItem(LEGACY_MODELS_KEY)
  } catch {
    /* ignore */
  }
  return { last_agent_type: last, prefs }
}
