import type { Agent } from '../types'
import { apiFetch } from './client'

// 获取可用 agent 列表
export function listAgents(): Promise<{ data: { agents: Agent[] } }> {
  return apiFetch('/agents')
}
