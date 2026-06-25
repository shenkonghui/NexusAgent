import type { Agent, ModelOption } from '../types'
import { apiFetch } from './client'

// 获取可用 agent 列表
export function listAgents(): Promise<{ data: { agents: Agent[] } }> {
  return apiFetch('/agents')
}

// 获取指定 agent 类型的可用模型列表（从已有会话缓存获取，可能为空）
export function getAgentModels(agentType: string): Promise<{ data: { model_options: ModelOption[] } }> {
  return apiFetch(`/agents/${encodeURIComponent(agentType)}/models`)
}
