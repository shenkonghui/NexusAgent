import type { Agent, ModelOption, ConfigOption, AgentStatus, AgentCommand, SessionMode } from '../types'
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

// 探测指定 agent 类型的全部 config options（服务端预连接时已缓存）。
const probeCache = new Map<string, ConfigOption[]>()

export function probeAgentConfigs(agentType: string): Promise<{ data: { config_options: ConfigOption[] } }> {
  const cached = probeCache.get(agentType)
  if (cached) {
    return Promise.resolve({ data: { config_options: cached } })
  }
  return apiFetch<{ data: { config_options: ConfigOption[] } }>(
    `/agents/${encodeURIComponent(agentType)}/probe`,
    { method: 'POST' },
  ).then((resp) => {
    probeCache.set(agentType, resp.data.config_options || [])
    return resp
  })
}

// 获取指定 agent 类型 slash command（Agent 原生 + 配置 commands；可选 cwd 扫描项目级）
export function listAgentCommands(agentType: string, cwd?: string): Promise<{ data: { commands: AgentCommand[] } }> {
  const qs = cwd ? `?path=${encodeURIComponent(cwd)}` : ''
  return apiFetch(`/agents/${encodeURIComponent(agentType)}/commands${qs}`)
}

// 获取指定 agent 类型缓存的 session mode（新建任务页用）
export function listAgentModes(agentType: string): Promise<{ data: { modes: SessionMode[] } }> {
  return apiFetch(`/agents/${encodeURIComponent(agentType)}/modes`)
}
