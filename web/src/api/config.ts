import { apiFetch } from './client'

// 单个 dir 配置视图
export interface DirConfigView {
  user_dirs: string[]
  project_dirs: string[]
}

// agents 配置视图（skills/commands/rules）
export interface AgentsConfigView {
  skills: DirConfigView
  commands: DirConfigView
  rules: DirConfigView
}

// 获取 agents 配置（skills/commands/rules 目录路径）
export function getAgentsConfig(): Promise<{ data: AgentsConfigView }> {
  return apiFetch('/config/agents')
}

// 更新 agents 配置
export function updateAgentsConfig(config: AgentsConfigView): Promise<{ data: { message: string } }> {
  return apiFetch('/config/agents', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

// 扫描到的文件项
export interface ScannedFileItem {
  name: string
  description: string
  location: string
  scope: string
  path: string
  always_apply?: boolean
  globs?: string
}

// 扫描技能文件（外部 skill.md）
export function scanSkillFiles(path?: string): Promise<{ data: { skills: ScannedFileItem[] } }> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/filesystem/skills${qs}`)
}

// 扫描命令文件
export function scanCommandFiles(path?: string): Promise<{ data: { commands: ScannedFileItem[] } }> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/filesystem/commands${qs}`)
}

// 扫描规则文件
export function scanRuleFiles(path?: string): Promise<{ data: { rules: ScannedFileItem[] } }> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/filesystem/rules${qs}`)
}

// 读取文件内容
export interface FileContentResponse {
  path: string
  content: string
  size: number
}

export function readFileContent(filePath: string): Promise<{ data: FileContentResponse }> {
  return apiFetch(`/filesystem/file?path=${encodeURIComponent(filePath)}`)
}

// 保存文件内容
export function writeFileContent(filePath: string, content: string): Promise<{ data: { path: string; size: number } }> {
  return apiFetch('/filesystem/file', {
    method: 'PUT',
    body: JSON.stringify({ path: filePath, content }),
  })
}
