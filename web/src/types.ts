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

// 会话
export interface Session {
  id: number;
  session_id: string;
  agent_type: string;
  cwd: string;
  status: 'active' | 'closed' | 'error';
  user_id: number;
  workspace_mode: string;
  last_prompt: string;
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
  created_at: string;
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
