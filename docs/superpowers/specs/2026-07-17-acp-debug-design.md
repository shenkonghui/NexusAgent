# ACP Debug 模式设计文档

- 日期：2026-07-17
- 状态：已实现

## 1. 背景与目标

对话页需要查看 ACP（Agent Client Protocol）协议交互数据，便于排查会话激活、prompt、权限与配置切换等问题。

目标：

1. 在连接级 tee 捕获 stdin/stdout 上的 line-delimited JSON-RPC 报文。
2. 按报文中的 `sessionId` 路由到对应 DB session 的本地文件。
3. 对话页工作区新增「调试」Tab，分页/增量展示事件与原始报文。
4. 默认关闭，由 `config.yaml` 开关控制。

## 2. 配置

```yaml
debug:
  acp:
    enabled: false
    dir: ~/.openNexus/acp-debug
```

- `enabled`：默认 `false`；关闭时不包装 tee、不写文件，调试 Tab 隐藏。
- `dir`：空则默认 `~/.openNexus/acp-debug`，启动时展开 `~`。

## 3. 目录结构

```text
~/.openNexus/acp-debug/
├── <dbSessionID>/
│   ├── raw.ndjson      # {ts,direction,session_id,db_session_id,line}
│   └── events.ndjson   # {ts,event,session_id,db_session_id,detail}
└── _<agentType>_.ndjson  # 无 sessionId 的连接级报文（initialize/authenticate）
```

- `direction`：`send`（客户端→agent）/ `recv`（agent→客户端）
- `event`：`new_session` / `prompt` / `cancel` / `set_config` / `set_mode` / `resume_session` 等

## 4. 架构

```text
agent 子进程 stdout/stdin
        │
   teeReader / teeWriter  ──► ACPDebugger.RouteLine
        │                          │
        ▼                          ▼
  SDK Connection            解析 sessionId → 映射 dbSessionID → append ndjson

Service.SetDebugConfig → NewACPDebugger
Service 在激活/Prompt/Cancel/配置切换等处 LogEvent + RegisterSession
```

## 5. API（JWT 保护，校验 session 归属）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/sessions/:id/debug` | `{enabled,dir,event_count,raw_count,last_ts}` |
| GET | `/api/v1/sessions/:id/debug/events?since=&limit=` | 事件数组 |
| GET | `/api/v1/sessions/:id/debug/raw?since=&limit=` | 原始报文数组 |

`since` 为行偏移（从 0 起），`limit` 默认 200；`limit=0` 表示不限制条数（安全上限 100000）。路径固定在数据目录内，不接受用户传入 path。

## 6. 前端

- `WorkspacePanel` 探测 `getDebugMeta().enabled`，为 true 时显示「调试」Tab。
- `DebugPanel`：事件 / 原始报文双 Tab，2s 轮询增量，可折叠 JSON，↑/↓ 区分收发。
- 原始报文：默认拉取全部、最新在上；可按下拉框按 method/方向过滤，默认「全部」。

## 7. 安全与健壮性

- 写入错误只 `slog.Warn`，不影响对话主流程。
- tee 在 debugger 未启用时不包装，零开销。
- raw 可能含 prompt/文件内容，属本地调试数据，敏感性与会话文件浏览器一致。

## 8. 验收

1. 开启 `debug.acp.enabled`，发 prompt 后目录下有 `raw.ndjson` / `events.ndjson`。
2. 对话页「调试」Tab 可见事件与原始报文，方向正确。
3. 增量轮询可用；关闭开关后 Tab 隐藏且不再写文件。
