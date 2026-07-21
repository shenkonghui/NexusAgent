// 上下文占用与工具调用解析工具。
// 数据来源：
//   - usage_update 消息的 raw_json._meta["codebuddy.ai/usageByCategory"] 提供按类别
//     （系统提示 / 工具 / MCP / 技能 / 对话）的 token 占用明细；
//   - tool_call / tool_call_update 消息的 raw_json 提供每一次工具调用的
//     kind / title / status / rawInput（shell 命令等），按 toolCallId 合并。

import type { Message } from '../types'

// 按类别拆分的上下文占用（token 数）
export interface CategoryUsage {
  systemPrompt: number
  tools: number
  mcp: number
  skills: number
  conversation: number
  // 各类别之和（用于计算占比；可能与 usage.used 略有出入，仅作展示参考）
  total: number
}

function toNum(v: unknown): number {
  return typeof v === 'number' && isFinite(v) ? v : 0
}

// parseCategoryUsage 取最新一条 usage_update 中的类别明细；不存在则返回 null。
export function parseCategoryUsage(messages: Message[]): CategoryUsage | null {
  let latest: CategoryUsage | null = null
  for (const msg of messages) {
    if (msg.kind !== 'usage_update' || !msg.raw_json) continue
    try {
      const parsed = JSON.parse(msg.raw_json)
      const cat = parsed?._meta?.['codebuddy.ai/usageByCategory']
      if (cat && typeof cat === 'object') {
        const systemPrompt = toNum(cat.systemPrompt)
        const tools = toNum(cat.tools)
        const mcp = toNum(cat.mcp)
        const skills = toNum(cat.skills)
        const conversation = toNum(cat.conversation)
        latest = {
          systemPrompt,
          tools,
          mcp,
          skills,
          conversation,
          total: systemPrompt + tools + mcp + skills + conversation,
        }
      }
    } catch {
      // 忽略解析失败
    }
  }
  return latest
}

// 工具调用分类
export type ToolCategory = 'shell' | 'mcp' | 'skill' | 'tool'

// 工具调用状态（ACP ToolCallStatus）
export type ToolCallStatus = 'pending' | 'in_progress' | 'completed' | 'failed' | string

// 一次工具调用的聚合记录（tool_call 起始 + 若干 tool_call_update 合并后）
export interface ToolCallEntry {
  id: string // toolCallId
  title: string
  kind: string // ACP ToolKind：read/edit/search/execute/fetch/...
  category: ToolCategory
  status: ToolCallStatus
  command?: string // shell 命令（kind=execute 时的 rawInput.command）
}

// classify 依据 kind 与标题/ID 推断工具类别。
// shell 由 kind=execute 精确判定；mcp / skill 依据命名启发式（best-effort），
// 其余归为通用 tool。类别 token 占比以 parseCategoryUsage 的权威数据为准。
function classify(kind: string, title: string, id: string): ToolCategory {
  if (kind === 'execute') return 'shell'
  const t = title.toLowerCase()
  const i = id.toLowerCase()
  if (t.includes('mcp') || i.includes('mcp')) return 'mcp'
  if (t === 'skill' || t.startsWith('skill ') || t.startsWith('skill:') || i.includes('skill')) {
    return 'skill'
  }
  return 'tool'
}

// parseToolCalls 遍历消息，按 toolCallId 合并 tool_call 与后续 tool_call_update，
// 返回按首次出现顺序排列的调用记录。snapshot- 开头的内部快照调用会被跳过。
export function parseToolCalls(messages: Message[]): ToolCallEntry[] {
  const map = new Map<string, ToolCallEntry>()
  const order: string[] = []
  for (const msg of messages) {
    if (msg.kind !== 'tool_call' && msg.kind !== 'tool_call_update') continue
    if (!msg.raw_json) continue
    let p: any
    try {
      p = JSON.parse(msg.raw_json)
    } catch {
      continue
    }
    const id = typeof p?.toolCallId === 'string' ? p.toolCallId : ''
    if (!id || id.includes('snapshot-')) continue
    let entry = map.get(id)
    if (!entry) {
      entry = { id, title: '', kind: '', category: 'tool', status: 'pending' }
      map.set(id, entry)
      order.push(id)
    }
    if (typeof p.title === 'string' && p.title) entry.title = p.title
    if (typeof p.kind === 'string' && p.kind) entry.kind = p.kind
    if (typeof p.status === 'string' && p.status) entry.status = p.status
    const cmd = p?.rawInput?.command
    if (typeof cmd === 'string' && cmd) entry.command = cmd
    entry.category = classify(entry.kind, entry.title, entry.id)
  }
  return order.map((id) => map.get(id)!)
}

// 工具调用按类别汇总的计数
export interface ToolCallSummary {
  shell: number
  mcp: number
  skill: number
  tool: number
  total: number
}

export function summarizeToolCalls(entries: ToolCallEntry[]): ToolCallSummary {
  const s: ToolCallSummary = { shell: 0, mcp: 0, skill: 0, tool: 0, total: entries.length }
  for (const e of entries) s[e.category]++
  return s
}
