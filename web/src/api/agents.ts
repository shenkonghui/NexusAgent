import type { Agent, ModelOption, ConfigOption, AgentStatus } from '../types'
import { apiFetch } from './client'

// 获取可用 agent 列表
export function listAgents(): Promise<{ data: { agents: Agent[] } }> {
  return apiFetch('/agents')
}

// 获取所有 agent 类型的 ACP 连接状态
export function listAgentStatus(): Promise<{ data: { agents: AgentStatus[] } }> {
  return apiFetch('/agents/status')
}

// 获取指定 agent 类型的可用模型列表（从已有会话缓存获取，可能为空）
export function getAgentModels(agentType: string): Promise<{ data: { model_options: ModelOption[] } }> {
  return apiFetch(`/agents/${encodeURIComponent(agentType)}/models`)
}

// 探测指定 agent 类型的全部 config options（创建临时会话后删除）。
// 返回与 /sessions/:id/config-options 相同结构的 config_options。
export function probeAgentConfigs(agentType: string): Promise<{ data: { config_options: ConfigOption[] } }> {
  return apiFetch(`/agents/${encodeURIComponent(agentType)}/probe`, { method: 'POST' })
}
