import type { ConfigOption } from '../types'

const STORAGE_KEY = 'nexus.agent.models'

function readMap(): Record<string, string> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const map = JSON.parse(raw) as Record<string, string>
    return map && typeof map === 'object' ? map : {}
  } catch {
    return {}
  }
}

/** 读取指定 agent 上次使用的模型。 */
export function getLastAgentModel(agentType: string): string {
  if (!agentType) return ''
  return readMap()[agentType] || ''
}

/** 保存指定 agent 上次使用的模型（按 agent 类型隔离）。 */
export function setLastAgentModel(agentType: string, modelValue: string) {
  if (!agentType || !modelValue.trim()) return
  try {
    const map = readMap()
    map[agentType] = modelValue.trim()
    localStorage.setItem(STORAGE_KEY, JSON.stringify(map))
  } catch {
    /* ignore quota / private mode */
  }
}

/** 优先使用上次保存的模型，否则回退到 agent 探测默认值。 */
export function resolveAgentModel(agentType: string, modelOpt?: ConfigOption): string {
  const saved = getLastAgentModel(agentType)
  if (saved) return saved
  if (!modelOpt) return ''
  return modelOpt.current_value || modelOpt.options[0]?.value || ''
}
