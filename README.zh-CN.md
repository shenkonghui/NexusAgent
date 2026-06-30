# NexusAgent

基于 [Agent Client Protocol (ACP)](https://github.com/coder/acp-go-sdk) 的多 Agent 统一管理与对话平台。在一个界面中接入并驱动 Claude Code、CodeBuddy、Kilo Code、Devin 等编码 Agent，实现多会话并发、流式对话、文件浏览编辑、终端交互与定时任务调度。

[English Documentation](README.md)

## 功能特性

- **多 Agent 接入**：内置 Claude Code、CodeBuddy、Kilo Code、Devin 等 ACP Agent，支持在设置页动态添加自定义 Agent 配置
- **会话管理**：创建 / 恢复 / 关闭 / 删除会话，每个 Agent 共享一条 ACP 连接（多路复用），支持同时并发多个会话
- **流式对话**：基于 SSE 的实时流式输出，展示 Agent 的思考、工具调用与最终回复
- **文件浏览与编辑**：在会话工作区内浏览目录、查看与编辑文件（集成 CodeMirror，支持多语言语法高亮）
- **终端交互**：基于 WebSocket 的 xterm 终端，可直接操作会话工作区
- **定时任务**：支持 cron 表达式调度，自动创建会话并发送 prompt，可查看历史执行记录
- **连接健康检查与自动重连**：后台定期检测各 Agent 连接状态，断线自动重连；侧边栏实时展示连接状态
- **用户认证**：JWT 鉴权，支持注册 / 登录 / 密码修改 / 个人资料
- **主题切换**：内置亮色 / 暗色主题
- **国际化**：支持中文和英文界面，在设置页可切换语言
- **单端口部署**：生产模式下前端构建产物由后端直接服务，前后端同一端口；同时支持 Docker 化部署
- **桌面客户端**：使用 [Pake](https://github.com/tw93/Pake)（基于 Tauri）封装为原生桌面应用，轻量高效，启动时自动拉起后端服务

## 技术栈

| 层 | 技术 |
|------|------|
| 后端 | Go 1.25 · Gin · GORM · SQLite · JWT |
| 前端 | React 18 · TypeScript · Vite · CodeMirror · xterm.js |
| 协议 | Agent Client Protocol (ACP) |

## 项目结构

```
NexusAgent/
├── cmd/server/          # 程序入口
├── internal/
│   ├── acp/             # ACP 协议封装：连接、客户端、会话管理、健康检查
│   ├── agent/           # Agent 注册表与路由层
│   ├── config/          # 配置加载与校验
│   ├── database/        # 数据库连接
│   ├── handlers/        # HTTP 处理器（会话、Agent、文件、终端、定时任务等）
│   ├── middleware/      # 中间件（JWT 鉴权）
│   ├── models/          # 数据模型
│   ├── repository/      # 数据访问层
│   ├── router/          # 路由注册与静态文件服务
│   └── services/        # 业务服务（认证、JWT、调度器）
├── web/                 # 前端源码（React + Vite）
├── config.yaml          # 默认配置文件
├── Dockerfile           # 多阶段构建（前端 + 后端）
├── docker-compose.yml   # 容器编排
└── Makefile             # 常用命令快捷方式
```

## 快速开始

### 环境要求

- Go >= 1.25
- Node.js >= 20
- 各 Agent 所需的 API Key（如 Claude Code 需要 `ANTHROPIC_API_KEY`）

### 本地开发

```bash
# 一键启动前后端开发服务器（后端 :8080，前端 :3000）
make dev
```

启动后访问 http://localhost:3000 即可使用。首次使用需注册账号。

如需单独启动：

```bash
make backend    # 仅启动后端 http://localhost:8080
make frontend   # 仅启动前端 http://localhost:3000
```

### 单端口运行（生产模式）

```bash
# 构建前端 + 后端，以 release 模式启动（前端 + API 同端口）
make run
```

访问 http://localhost:8080。

### Docker 部署

```bash
# 构建镜像并前台启动
make docker-up

# 或后台启动
make docker-up-d
```

如需配置 `ANTHROPIC_API_KEY` 等环境变量，在启动前设置：

```bash
ANTHROPIC_API_KEY=sk-xxx make docker-up-d
```

## 配置说明

配置文件为 `config.yaml`，也可通过环境变量覆盖：

| 配置项 | 环境变量 | 说明 |
|--------|---------|------|
| `server.port` | `SERVER_PORT` | 服务端口，默认 `8080` |
| `server.mode` | `SERVER_MODE` | `debug` / `release`，release 为单端口模式 |
| `server.web_dist` | `WEB_DIST` | 前端构建产物目录，默认 `./web/dist` |
| `database.path` | `DATABASE_PATH` | SQLite 数据库路径，默认 `./data/nexus.db` |
| `jwt.secret` | `JWT_SECRET` | JWT 签名密钥（生产环境务必修改） |
| `agents.workspace.session_dir` | `AGENTS_WORKSPACE_SESSION_DIR` | 会话工作区根目录 |

Agent 的连接命令、参数、API Key 等可在前端「设置」页面动态管理，修改后实时生效。

## 常用命令

| 命令 | 说明 |
|------|------|
| `make dev` | 一键启动前后端开发服务器 |
| `make run` | 单端口启动（release 模式） |
| `make build` | 构建前端 + 后端 |
| `make pake` | 仅打包 Pake 桌面客户端 |
| `make desktop` | 构建完整桌面应用（后端 + Pake 客户端） |
| `make test` | 运行后端全部测试 |
| `make clean` | 清理构建产物 |
| `make docker-up` | 构建 Docker 镜像并前台启动 |
| `make docker-down` | 停止并清理 Docker 容器 |
| `make docker-logs` | 查看 Docker 容器日志 |

## 许可证

私有项目，保留所有权利。
