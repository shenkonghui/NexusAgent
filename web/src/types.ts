// 用户信息
export interface User {
  id: number;
  username: string;
  email: string;
  role: string;
  status: string;
}

// Agent 描述
export interface Agent {
  type: string;
  display_name: string;
  description: string;
}

// Agent 连接状态（侧边栏展示用）
export interface AgentStatus {
  agent_type: string;
  status: 'connected' | 'connecting' | 'disconnected';
  active_count: number;
}

// Agent 配置（设置页面管理的本地 ACP agent）
export interface AgentConfig {
  id: number;
  type: string;
  display_name: string;
  description: string;
  command: string;
  args: string[];
  api_key_env: string;
  timeout: string;
  enabled: boolean;
}

// Slash command（ACP available command）
export interface AgentCommand {
  name: string;
  description: string;
  has_input: boolean;
  path?: string;
  scope?: string;
  kind?: 'command' | 'agent';
}

// Config option 可选项值
export interface ConfigOptionValue {
  value: string;
  name: string;
  description: string;
}

// Config option（含模型选择等）
export interface ConfigOption {
  id: string;
  name: string;
  category: string; // "model" | "mode" | "thought_level" | ...
  type: string; // "select" | "boolean"
  current_value: string;
  options: ConfigOptionValue[];
}

// Session mode（ACP 协议的会话模式，如 plan/act）
export interface SessionMode {
  id: string;
  name: string;
  description: string;
}

// 权限请求选项（ACP RequestPermission）
export interface PermissionOption {
  optionId: string;
  name: string;
  kind: 'allow_once' | 'allow_always' | 'reject_once' | 'reject_always' | string;
}

export interface PermissionRequestPayload {
  request_id: string;
  tool_call?: { title?: string; toolCallId?: string };
  options: PermissionOption[];
}

// Agent Skill（agentskills.io 规范，SKILL.md 格式）
export interface AgentSkill {
  name: string;
  description: string;
  location: string;
  scope: string; // "project" | "user"
  path?: string;
}

// 工作区
export interface Workspace {
  id: number;
  user_id: number;
  name: string;
  cwd: string;
  mode: 'persistent' | 'temporary';
  temp_dir?: string;
  session_count?: number;
  created_at: string;
  updated_at: string;
}

// 会话
export interface Session {
  id: number;
  session_id: string;
  agent_type: string;
  status: 'active' | 'closed' | 'error';
  user_id: number;
  workspace_id: number | null;
  workspace?: Workspace;
  last_prompt: string;
  title: string;
  source: 'manual' | 'scheduled' | 'classify';
  created_at: string;
  closed_at: string | null;
}

// 消息
export interface Message {
  id: number;
  session_id: string;
  role: 'user' | 'assistant' | 'tool';
  kind: string;
  content: string;
  raw_json: string;
  sequence: number;
  execution_id: number | null;
  created_at: string;
}

// 定时任务
export interface ScheduledTask {
  id: number;
  name: string;
  agent_type: string;
  workspace_id?: number;
  cwd: string;
  prompt: string;
  cron_expr: string;
  enabled: boolean;
  user_id: number;
  timeout_minutes: number;
  model_value: string;
  session_id: string;
  db_session_id: number;
  last_run_at: string | null;
  last_status: 'success' | 'running' | 'failed' | 'skipped' | '';
  last_error: string;
  created_at: string;
  updated_at: string;
}

// Agent 模型 config option（用于定时任务模型选择）
export interface ModelOption {
  id: string;
  name: string;
  current_value: string;
  options: ConfigOptionValue[];
}

// 定时任务执行块聚合
export interface Execution {
  execution_id: number;
  started_at: string;
  finished_at: string;
  message_count: number;
  status: 'success' | 'running' | 'failed' | 'skipped' | '';
  error: string;
}

// 认证响应
export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  user: User;
}

// API 统一错误响应
export interface ApiError {
  error: {
    code: string;
    message: string;
  };
}

// 全局笔记
export interface Note {
  id: number;
  title: string;
  content: string;
  tags: string[];
  classify_pending: boolean;
  created_at: string;
  updated_at: string;
}

export interface NoteSettings {
  agent_type: string;
  model_value: string;
  classify_prompt: string;
  classify_interval_minutes: number;
  classify_session_id?: string;
  classify_db_session_id?: number;
}
