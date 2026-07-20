import { apiFetch } from './client'

// 单个 dir 配置视图
export interface DirConfigView {
  user_dirs: string[]
  project_dirs: string[]
}

// agents 配置视图（skills/commands/rules/subagents）
export interface AgentsConfigView {
  skills: DirConfigView
  commands: DirConfigView
  rules: DirConfigView
  subagents: DirConfigView
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

// 软重载：重新读取 config.yaml 并热刷新 skill/command/rule 扫描目录（不杀进程）。
// 浏览器访问远程后端时使用；桌面版走 IPC 硬重载（window.nexusagent.reloadBackend）。
export function reloadProgram(): Promise<{ data: { message: string; restarted: boolean } }> {
  return apiFetch('/config/reload', { method: 'POST' })
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
  model?: string
  tools?: string[]
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

// 扫描 subagent 定义文件
export function scanSubAgentFiles(path?: string): Promise<{ data: { subagents: ScannedFileItem[] } }> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiFetch(`/filesystem/sub-agents${qs}`)
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

// MCP 配置（全局共享 mcpServers JSON）
export interface MCPConfigResponse {
  config: string // 文件原始文本
  path: string   // 配置文件绝对路径
  count: number  // 解析到的 server 数量
}

// 获取 MCP 配置（mcp.json 原始内容）
export function getMCPConfig(): Promise<{ data: MCPConfigResponse }> {
  return apiFetch('/config/mcp')
}

// 更新 MCP 配置（保存即生效，新建会话自动注入）
export function updateMCPConfig(config: string): Promise<{ data: MCPConfigResponse }> {
  return apiFetch('/config/mcp', {
    method: 'PUT',
    body: JSON.stringify({ config }),
  })
}

// 单个 MCP 工具信息
export interface MCPToolInfo {
  name: string
  title?: string
  description?: string
}

// 单个 MCP server 探测结果
export interface MCPServerStatus {
  name: string
  type: string         // stdio | http | sse
  connected: boolean
  error?: string
  server_info?: string // InitializeResult.ServerInfo.Name
  tools: MCPToolInfo[]
}

// 探测所有 MCP server 的连接状态与工具列表
export function getMCPStatus(): Promise<{ data: { servers: MCPServerStatus[]; error?: string } }> {
  return apiFetch('/config/mcp/status')
}
