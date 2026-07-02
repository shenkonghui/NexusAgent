# NexusAgent Web UI（P6）设计文档

- 日期：2026-06-24
- 子项目：P6 — Web UI
- 状态：待审查

## 1. 背景与定位

NexusAgent 是一个用 Go 开发的全栈平台，目标是通过 ACP（Agent Client Protocol）协议调用 Claude Code、Codex 等编码 agent 执行任务，并支持用户认证。

整个平台被拆分为 6 个可独立设计的子项目：

| # | 子项目 | 依赖 |
|----|--------|------|
| P1 | ACP 客户端核心库（已完成） | 无 |
| P2 | Agent 注册与编排层（已完成） | P1 |
| P3 | 用户认证系统（已完成） | 无 |
| P4 | 会话消息持久化与会话恢复（已完成） | P1, P2 |
| P5 | REST API 接入层（已完成） | P2, P3, P4 |
| P6 | Web UI（本文档） | P5 |

本文档覆盖 **P6 Web UI**。P5 已提供完整的 REST API + SSE 流式接口。P6 构建一个前端单页应用（SPA），提供登录/注册、会话列表、聊天界面、消息历史查看等功能。

## 2. 需求摘要

- 用户可注册、登录、登出。
- 已登录用户可查看可用 agent 列表。
- 用户可创建会话（选择 agent 类型 + 可选工作目录）。
- 用户可在会话中发送 prompt，实时查看 agent 的流式响应（通过 SSE）。
- 用户可查看会话消息历史。
- 用户可关闭会话、恢复失效会话。
- 令牌自动刷新（Access Token 过期时用 Refresh Token 自动刷新，对用户透明）。
- 响应式布局，适配桌面浏览器。

## 3. 技术栈

| 组件 | 选型 |
|------|------|
| 框架 | React 18 + TypeScript |
| 构建工具 | Vite |
| 路由 | React Router v6 |
| HTTP 客户端 | 原生 `fetch`（封装统一拦截器） |
| SSE 客户端 | 原生 `EventSource` 不可用（不支持 POST + 自定义头），使用 `fetch` + `ReadableStream` 手动解析 SSE 流 |
| 样式 | CSS Modules（无额外 UI 库，保持轻量） |
| 状态管理 | React Context + `useState`/`useReducer`（无 Redux，YAGNI） |

## 4. 架构与目录结构

```
web/                           # 前端项目根目录（独立于 Go 后端）
  index.html
  package.json
  tsconfig.json
  vite.config.ts
  src/
    main.tsx                   # 应用入口，挂载 React Router
    App.tsx                    # 路由定义 + AuthProvider 包裹
    types.ts                   # TypeScript 类型定义（Session, Message, Agent, User 等）
    api/
      client.ts                # fetch 封装：baseURL、JWT 注入、401 自动刷新
      sse.ts                   # SSE 流解析器：fetch ReadableStream → 逐行解析 data: 行
      auth.ts                  # 认证 API：register/login/refresh/logout/me
      agents.ts                # agent API：list
      sessions.ts              # 会话 API：create/list/get/close/prompt/cancel/resume/messages
    context/
      AuthContext.tsx          # 认证上下文：当前用户、token 管理、登录/登出方法
    hooks/
      useAuth.ts               # 认证 hook（消费 AuthContext）
      useRequireAuth.ts        # 路由守卫 hook：未登录跳转 /login
    pages/
      LoginPage.tsx            # 登录/注册页
      SessionsPage.tsx         # 会话列表 + 新建会话
      ChatPage.tsx             # 聊天界面（单会话）
      NotesPage.tsx            # 笔记页
    components/
      MessageList.tsx          # 消息列表渲染（区分 user/assistant/tool）
      MessageBubble.tsx        # 单条消息气泡
      PromptInput.tsx          # prompt 输入框 + / 与 @ 补全
      MarkdownContent.tsx      # 笔记 Markdown 渲染
      SessionSidebar.tsx       # 会话列表侧边栏
      AgentSelector.tsx        # agent 类型选择器
      ErrorBanner.tsx          # 错误提示横幅
      LoadingSpinner.tsx       # 加载指示器
    styles/
      *.module.css             # 各组件/页面的 CSS Modules
```

## 5. 页面设计

### 5.1 登录/注册页（`/login`）

- 切换登录/注册两种模式（toggle）。
- 登录表单：account（用户名或邮箱）+ password。
- 注册表单：username + email + password。
- 登录成功后存储 access_token / refresh_token 到 localStorage，跳转 `/sessions`。
- 错误提示（弱密码、用户已存在、凭证错误等）。

### 5.2 会话列表页（`/sessions`）

- 顶部显示可用 agent 列表（`GET /api/v1/agents`）。
- "新建会话"按钮 → 弹出表单：选择 agent 类型 + 可选输入工作目录（cwd）→ `POST /api/v1/sessions`。
- 会话列表（`GET /api/v1/sessions`）：显示每个会话的 agent 类型、状态、创建时间、最近 prompt。
- 点击会话 → 跳转 `/sessions/:id`。
- 会话状态标识：active（绿）/ closed（灰）/ error（红，可点击恢复）。

### 5.3 聊天页（`/sessions/:id`）

- 左侧 SessionSidebar：当前用户的会话列表，高亮当前会话。
- 右侧聊天区域：
  - 顶部：会话信息（agent 类型、状态、cwd）。
  - 消息列表（`GET /api/v1/sessions/:id/messages`）：按时间顺序渲染。
  - 底部 PromptInput：输入框 + 发送按钮。
- 发送 prompt → `POST /api/v1/sessions/:id/prompt`（SSE 流）：
  - 流式接收消息，逐条追加到消息列表。
  - 流结束时（收到 `[DONE]`）停止 loading。
- 取消按钮（prompt 进行中显示）→ `POST /api/v1/sessions/:id/cancel`。
- 若会话状态为 error，显示"恢复会话"按钮 → `POST /api/v1/sessions/:id/resume`。
- 若会话状态为 closed，显示提示，禁用输入。

#### PromptInput 补全（已实现）

`PromptInput` 在输入框内提供两类补全菜单（↑↓ 选择，Enter 确认，Esc 返回上一级或关闭）：

| 触发符 | 行为 |
|--------|------|
| `/` | 平铺筛选 command、skill、mode；选中后插入 `/name`，后端展开本地 command / skill 文件 |
| `@` | 分级选择：一级为类型（Command / Skill / File / Note），二级为具体项 |

`@` 各类型说明：

- **Command / Skill**：二级列表选中后插入 `/name`
- **File**：浏览会话工作区目录（需传入 `cwd`）；目录可继续进入；文件插入 `@/绝对路径`
- **Note**：二级为标签列表，三级为该标签下的笔记；选中后插入 `@note:{id}`

数据来源：`commands` / `skills` / `modes` 由 ChatPage 从会话 API 加载；文件通过 `GET /api/v1/filesystem/list`；笔记通过 `GET /api/v1/notes` 与 `GET /api/v1/notes/tags`。

### 5.5 笔记页（`/notes`，已实现）

- 左侧 SessionSidebar，右侧笔记流式列表。
- 底部快速输入框：回车创建笔记；内容中的 `#tag` 自动解析为标签。
- 顶部标签栏筛选；支持标题 / 内容 / 标签搜索。
- 笔记卡片支持 Markdown 渲染（`MarkdownContent`）与 inline 编辑。
- 待分类笔记（`classify_pending`）会轮询刷新，配合后台 Agent 分类任务。

### 5.4 消息渲染规则

| role | kind | 渲染方式 |
|------|------|---------|
| user | user_message_chunk | 右对齐气泡，content 为正文 |
| assistant | agent_message_chunk | 左对齐气泡，content 为正文，支持基本 Markdown |
| assistant | agent_thought_chunk | 左对齐，浅色斜体，标注"思考" |
| tool | tool_call | 左对齐，卡片样式，显示 title + 状态 |
| tool | tool_call_update | 更新对应 tool_call 卡片的状态/输出 |
| assistant | plan | 左对齐，卡片样式，显示计划步骤 |

`raw_json` 保留完整数据，可展开查看详情（折叠面板）。

## 6. 核心实现

### 6.1 API 客户端（`api/client.ts`）

封装 `fetch`，统一处理：

- **baseURL**：从环境变量 `VITE_API_BASE` 读取，默认 `/api/v1`。
- **JWT 注入**：每次请求自动从 localStorage 读取 access_token，添加 `Authorization: Bearer` 头。
- **401 自动刷新**：收到 401 时，用 refresh_token 调用 `/auth/refresh`，成功后重试原请求；失败则清除 token 跳转 `/login`。
- **统一错误解析**：解析 `{ error: { code, message } }` 格式，抛出带 code 的 Error。

### 6.2 SSE 流解析（`api/sse.ts`）

由于 `EventSource` 不支持 POST 请求和自定义请求头，使用 `fetch` + `ReadableStream` 手动解析：

```typescript
async function streamPrompt(
  sessionId: number,
  prompt: string,
  onMessage: (msg: Message) => void,
  onDone: () => void,
  onError: (err: Error) => void
): Promise<void>
```

流程：

1. 用 `fetch` 发起 POST 请求（带 JWT 头）。
2. 读取 `response.body` 的 ReadableStream。
3. 用 TextDecoder 逐块解码，按 `\n\n` 分割 SSE 事件。
4. 解析每个事件的 `data:` 行：若为 `[DONE]` 调用 onDone，否则 JSON.parse 为 Message 调用 onMessage。
5. 请求中断（AbortController）时调用 onError。

### 6.3 认证上下文（`context/AuthContext.tsx`）

```typescript
interface AuthState {
  user: User | null;
  loading: boolean;
}
interface AuthContextValue extends AuthState {
  login: (account: string, password: string) => Promise<void>;
  register: (username: string, email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}
```

- 启动时调用 `GET /me` 验证 token 有效性，设置 user。
- token 存储在 localStorage（access_token + refresh_token）。
- 登出时调用 `POST /auth/logout` 并清除 localStorage。

### 6.4 Vite 开发代理

`vite.config.ts` 配置 dev server 代理，将 `/api` 请求转发到 Go 后端（默认 `http://localhost:8080`），避免 CORS 问题：

```typescript
export default defineConfig({
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
});
```

生产环境由 Go 后端直接提供静态文件服务（`embed` 或静态目录），或通过反向代理部署。

## 7. 类型定义（`types.ts`）

```typescript
interface User {
  id: number;
  username: string;
  email: string;
  role: string;
  status: string;
}

interface Agent {
  type: string;
  display_name: string;
  description: string;
}

interface Session {
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

interface Message {
  id: number;
  session_id: string;
  role: 'user' | 'assistant' | 'tool';
  kind: string;
  content: string;
  raw_json: string;
  sequence: number;
  created_at: string;
}
```

## 8. 测试策略

前端测试采用轻量策略（YAGNI，不引入复杂测试框架）：

- **类型安全**：TypeScript 严格模式，编译期捕获类型错误。
- **手动验证**：开发阶段通过 Vite dev server 手动验证各页面功能。
- **构建验证**：`npm run build` 确保 TypeScript 编译通过、无类型错误。

不引入 Jest/Vitest 等测试框架。前端逻辑相对简单（主要是 API 调用 + 渲染），核心业务逻辑在后端已充分测试。

## 9. 范围边界（不做 / 已实现补充）

原规格中以下项**已实现**，不再属于边界：

- 暗色主题切换（设置页可切换亮色 / 暗色）
- 国际化（中 / 英文，设置页切换）
- 笔记 Markdown 渲染（笔记页；聊天 prompt 仍为纯文本）

仍不做：

- 不实现 Markdown 编辑器（prompt 输入为纯文本）。
- 不实现代码语法高亮（agent 输出的代码块纯文本渲染）。
- 不实现拖拽上传文件。
- 不实现多标签页/多会话同时聊天。
- 不实现 PWA / 离线支持。
- 不实现单元测试框架（手动验证 + 类型安全）。
- 不实现真实终端模拟（tool_call 仅展示文本卡片）。

> 注：原规格写「不实现暗色主题 / 国际化」已过时，见上文已实现补充。

## 10. 成功标准

- 登录/注册功能可用，token 自动刷新。
- 会话列表页可查看/创建会话。
- 聊天页可发送 prompt 并实时查看 SSE 流式响应。
- 消息历史可正确渲染（区分 user/assistant/tool）。
- 会话关闭/恢复功能可用。
- `npm run build` 编译通过，无 TypeScript 错误。
- 响应式布局适配桌面浏览器。
