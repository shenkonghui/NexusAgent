import type { AgentConfig } from '../types'
import { apiFetch } from './client'

// 获取全部 agent 配置
export function listAgentConfigs(): Promise<{ data: { agent_configs: AgentConfig[] } }> {
  return apiFetch('/agent-configs')
}

// 创建 agent 配置
export function createAgentConfig(payload: {
  type: string
  display_name: string
  description?: string
  command: string
  args?: string[]
  env?: Record<string, string>
  api_key_env?: string
  timeout?: string
  enabled?: boolean
}): Promise<{ data: AgentConfig }> {
  return apiFetch('/agent-configs', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

// 更新 agent 配置
export function updateAgentConfig(
  id: number,
  payload: {
    type: string
    display_name: string
    description?: string
    command: string
    args?: string[]
    env?: Record<string, string>
    api_key_env?: string
    timeout?: string
    enabled?: boolean
  },
): Promise<{ data: AgentConfig }> {
  return apiFetch(`/agent-configs/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

// 删除 agent 配置
export function deleteAgentConfig(id: number): Promise<void> {
  return apiFetch(`/agent-configs/${id}`, { method: 'DELETE' })
}
