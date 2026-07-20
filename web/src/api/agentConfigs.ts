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

// registry 刷新结果
export interface RegistryRefreshResult {
  version: string
  total: number
  added: number
  updated: number
}

// 在线拉取最新 ACP registry 并合并到本地存储。
// 新 agent 以 enabled=false 入库（需手动启用），已有 agent 仅刷新名称/描述。
// 对运行中的后端零影响。
export function refreshRegistry(): Promise<{ data: RegistryRefreshResult }> {
  return apiFetch('/agent-configs/registry/refresh', { method: 'POST' })
}

// 单个 agent 在内嵌 registry 中的默认值（供"重置为默认"按钮预填表单）。
export interface RegistryDefault {
  command: string
  args: string[]
  display_name: string
  description: string
}

// 取单个 agent 的 registry 默认 command/args。
// 不在 registry 中的 agent 后端返回 404。
export function getRegistryDefault(id: number): Promise<{ data: RegistryDefault }> {
  return apiFetch(`/agent-configs/${id}/registry-default`)
}

// 单个 agent 从 CDN 最新 registry 同步的结果。
export interface UpdateFromRegistryResult {
  version: string
  command: string
  args: string[]
  redownloaded: boolean // binary agent 是否清除了旧缓存（下次启动重下）
  source: string        // "cdn" | "embedded"
}

// 从 CDN 最新 registry 同步单个 agent：原子完成"拉取→更新配置→(binary 类)清缓存触发重下"。
// 不在 registry 中的 agent 后端返回 404。
export function updateAgentFromRegistry(id: number): Promise<{ data: UpdateFromRegistryResult }> {
  return apiFetch(`/agent-configs/${id}/update-from-registry`, { method: 'POST' })
}
