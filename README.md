# NexusAgent

A multi-Agent orchestration and conversation platform based on the [Agent Client Protocol (ACP)](https://github.com/coder/acp-go-sdk). Connect and drive coding agents like Claude Code, CodeBuddy, Kilo Code and Devin from a single interface with multi-session concurrency, streaming conversations, file editing, terminal interaction, and scheduled task automation.

[🇨🇳 中文文档](README.zh-CN.md)

## Features

- **Multi-Agent Access**: Built-in support for Claude Code, CodeBuddy, Kilo Code, Devin and more ACP agents. Add custom agent configurations dynamically via the Settings page.
- **Session Management**: Create / resume / close / delete sessions. Multiple sessions share a single ACP connection per agent type (multiplexed) for concurrent usage.
- **Streaming Conversations**: Real-time SSE streaming output showing Agent thinking, tool calls, and final responses.
- **File Browsing & Editing**: Browse directories, view and edit files within the session workspace (CodeMirror with multi-language syntax highlighting).
- **Terminal**: WebSocket-based xterm terminal for direct session workspace interaction.
- **Scheduled Tasks**: Cron-driven task scheduling with automatic session creation and prompt execution. View execution history.
- **Health Check & Auto-Reconnect**: Background agent connection health monitoring with automatic reconnection on failure. Real-time status badges in the sidebar.
- **User Authentication**: JWT-based auth with registration, login, password change, and profile management.
- **Theme Toggle**: Light and dark theme support.
- **Internationalization**: Chinese and English UI. Switch language in the Settings page.
- **Single-Port Deployment**: Production mode serves the frontend build directly from the backend. Docker support included.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25 · Gin · GORM · SQLite · JWT |
| Frontend | React 18 · TypeScript · Vite · CodeMirror · xterm.js |
| Protocol | Agent Client Protocol (ACP) |

## Project Structure

```
NexusAgent/
├── cmd/server/          # Entry point
├── internal/
│   ├── acp/             # ACP protocol: connection, client, session, health check
│   ├── agent/           # Agent registry and router
│   ├── config/          # Config loading and validation
│   ├── database/        # DB connection
│   ├── handlers/        # HTTP handlers (sessions, agents, files, terminal, tasks)
│   ├── middleware/      # JWT auth middleware
│   ├── models/          # Data models
│   ├── repository/      # Data access layer
│   ├── router/          # Route registration and static file serving
│   └── services/        # Business services (auth, JWT, scheduler)
├── web/                 # Frontend (React + Vite)
├── config.yaml          # Default configuration
├── Dockerfile           # Multi-stage build (frontend + backend)
├── docker-compose.yml   # Container orchestration
└── Makefile             # Common command shortcuts
```

## Quick Start

### Prerequisites

- Go >= 1.25
- Node.js >= 20
- API keys for the agents you want to use (e.g., `ANTHROPIC_API_KEY` for Claude Code)

### Local Development

```bash
# Start both frontend and backend dev servers
make dev
```

Visit http://localhost:3000. Register an account to get started.

### Production Mode (Single Port)

```bash
# Build frontend + backend, run in release mode
make run
```

Visit http://localhost:8080.

### Docker Deployment

```bash
# Build image and start
make docker-up

# Or run in background
make docker-up-d
```

Set environment variables like `ANTHROPIC_API_KEY` before starting:

```bash
ANTHROPIC_API_KEY=sk-xxx make docker-up-d
```

## Configuration

The configuration file is `config.yaml`. Environment variable overrides:

| Config | Env Var | Description |
|--------|---------|-------------|
| `server.port` | `SERVER_PORT` | Server port (default: `8080`) |
| `server.mode` | `SERVER_MODE` | `debug` / `release` |
| `server.web_dist` | `WEB_DIST` | Frontend build directory (default: `./web/dist`) |
| `database.path` | `DATABASE_PATH` | SQLite database path (default: `./data/nexus.db`) |
| `jwt.secret` | `JWT_SECRET` | JWT signing secret (change in production!) |

Agent commands, arguments, and API keys can be managed dynamically in the Settings page — changes take effect immediately.

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make dev` | Start frontend + backend dev servers |
| `make run` | Single-port production mode |
| `make build` | Build frontend + backend |
| `make test` | Run all backend tests |
| `make clean` | Clean build artifacts |
| `make docker-up` | Build Docker image and start |
| `make docker-down` | Stop and clean Docker containers |
| `make docker-logs` | View Docker container logs |

## License

Private project. All rights reserved.
