# 工作区切换器设计（恢复会话主界面）

## 背景

工作区功能初版将 `HomePage` + 左侧 `WorkspaceSidebar` 作为默认入口。用户希望恢复原来的会话列表主界面，工作区改为右上角切换。

## 目标

- `/` 使用 `ChatPage` 无会话模式（左侧 SessionSidebar + 新建对话，与对话页同一布局）
- 右上角提供工作区下拉选择器 +「+」新建
- 左侧会话列表仅显示当前工作区下的会话
- 新建会话使用当前选中的 `workspace_id`

## 方案

采用 localStorage（`nexus.current.workspace`）持久化当前工作区，不改动 URL 路由结构。

## 组件

| 组件 | 职责 |
|------|------|
| `WorkspaceSelector` | 下拉切换、新建、重命名/删除/保存 |
| `SessionsPage` | 已移除，统一由 `ChatPage` 承担 |
| `ChatPage` | 主界面（无 sid）+ 对话页（有 sid）；header 展示工作区选择器 |

## 路由

| 路由 | 行为 |
|------|------|
| `/` | `ChatPage`（无会话：侧边栏 + 新建；有会话：同页对话） |
| `/workspaces/:wid` | 重定向到 `/` 并选中该工作区 |
| `/workspaces/:wid/sessions/:sid` | 保留 |

## 删除

- `HomePage`、`WorkspacePage`、`WorkspaceSidebar` 及其样式
