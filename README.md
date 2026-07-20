# openNexus

A multi-Agent orchestration and conversation platform based on the [Agent Client Protocol (ACP)](https://github.com/coder/acp-go-sdk). Connect and drive coding agents like Claude Code, CodeBuddy, Kilo Code and Devin from a single interface with multi-session concurrency, streaming conversations, file editing, terminal interaction, and scheduled task automation.

[🇨🇳 中文文档](README.zh-CN.md)

## Features

- **Multi-Agent Access**: Built-in support for Claude Code, CodeBuddy, Kilo Code, Devin and more ACP agents. Add custom agent configurations dynamically via the Settings page.
- **Session Management**: Create / resume / close / delete sessions. Multiple sessions share a single ACP connection per agent type (multiplexed) for concurrent usage.
- **Streaming Conversations**: Real-time SSE streaming output showing Agent thinking, tool calls, and final responses.
- **File Browsing & Editing**: Browse directories, view and edit files within the session workspace (CodeMirror with multi-language syntax highlighting).
- **Terminal**: WebSocket-based xterm terminal for direct session workspace interaction.
- **Scheduled Tasks**: Cron-driven task scheduling with automatic session creation and prompt execution. View execution history.
- **Notes**: Quick capture with `#tag` parsing, tag filtering, Markdown rendering, and optional Agent-based auto-classification.
- **Prompt Input Enhancements**: `/` completes commands, skills, and modes; `@` provides hierarchical references to commands, skills, workspace files, and notes (browse by tag).
- **Skills & Commands Discovery**: Scans `SKILL.md` and slash command files under workspace and user directories for autocomplete.
- **Health Check & Auto-Reconnect**: Background agent connection health monitoring with automatic reconnection on failure. Real-time status badges in the sidebar.
- **User Authentication**: JWT-based auth with registration, login, password change, and profile management.
- **Theme Toggle**: Light and dark theme support.
- **Internationalization**: Chinese and English UI. Switch language in the Settings page.
- **Single-Port Deployment**: Production mode serves the frontend build directly from the backend. Docker support included.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25 · Gin · GORM · SQLite · JWT |
| Frontend | React 18 · TypeScript · Vite · CodeMirror · xterm.js · react-markdown |
| Protocol | Agent Client Protocol (ACP) |

## Project Structure

```
openNexus/
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
| `database.path` | `DATABASE_PATH` | SQLite database path (default: `~/.openNexus/opennexus.db`) |
| `jwt.secret` | `JWT_SECRET` | JWT signing secret (change in production!) |
| `agents.workspace.session_dir` | `AGENTS_WORKSPACE_SESSION_DIR` | Session workspace root (default: `~/.openNexus/session`) |

Config file lookup: `CONFIG_PATH` → `~/.openNexus/config.yaml` → `./config.yaml`. Database and session data default to `~/.openNexus/`.

Agent commands, arguments, and API keys can be managed dynamically in the Settings page — changes take effect immediately.

## Data Migration (Automatic)

On startup, openNexus automatically migrates data from legacy directories left by previous versions, so existing users can upgrade without data loss. The migration runs once before config loading and is **idempotent** — re-running has no effect.

| Legacy directory | Migrated to | Contents |
|------------------|-------------|----------|
| `~/.nextAgent` | `~/.openNexus` | Database, session workspaces, config, ACP debug logs |
| `~/.nexusagent/binaries` | `~/.openNexus/binaries` | Downloaded agent binaries and `versions.json` |
| `~/.openNexus/nexus.db` | `~/.openNexus/opennexus.db` | Renamed in place (within the data dir) |

**Migration policy (target-first):** if `~/.openNexus` already exists and is non-empty, the main-directory migration is skipped to avoid overwriting existing data (the legacy directory is preserved as-is, and a log line points you to it). The binary cache is still merged entry-by-entry (target entries are kept). When both `nexus.db` and `opennexus.db` exist, `opennexus.db` wins and the old file is removed.

Migration errors are **non-fatal** — they are logged as warnings and startup continues (consistent with existing recover-on-startup logic like `RestoreBinarySymlinks` / `RecoverActiveSessions`).

**Skip the migration** (e.g. for Docker / CI where data is managed externally):

```bash
SKIP_DATA_MIGRATION=1 ./opennexus
```

> **Manual recovery:** if a fresh start created an empty `~/.openNexus` before the migration could run, the auto-migration will skip it. You can recover by stopping the server, replacing `~/.openNexus/opennexus.db` with your `~/.nextAgent/nexus.db`, and moving `~/.nextAgent/session/*` into `~/.openNexus/session/`. The original legacy directory is never deleted by the migration.

## Agent Integration

### Enabling an Agent

1. Open **Settings → Agent** and enable the target agent (Claude Code is enabled by default on first launch; other agents from the [ACP Registry](https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json) are synced but disabled)
2. Configure required environment variables (e.g. `ANTHROPIC_API_KEY` for Claude Code)
3. The backend registers the agent immediately and completes connection **asynchronously in the background**

### Background Authentication

After enabling an agent, openNexus automatically performs these steps in the background (`PreconnectAllAsync` + health-check reconnect), with no manual action in the UI:

1. **Start subprocess**: run the configured `npx` / `uvx` or binary distribution command
2. **ACP handshake**: call `initialize` to negotiate capabilities
3. **ACP authentication**: only `env_var` methods (API key injected via `api_key_env`) are auto-authenticated; `agent` / `terminal` interactive login is not attempted in the background
4. **Config probe**: cache available models, modes, and commands
5. **Health check**: poll connection status every 30 seconds and auto-reconnect on failure

Connection status (connected / connecting / disconnected) is shown in the sidebar. Check backend logs on failure (agent stderr is forwarded to the server console).

### Distribution Types & Binaries

| Type | Launch | Prerequisites |
|------|--------|---------------|
| `npx` | `npm exec --include=optional --yes <package>` | Node.js / npm (included in Docker image) |
| `uvx` | `uvx <package>` | [uv](https://github.com/astral-sh/uv) installed on host |
| `binary` | Download platform archive from Registry | Auto-downloaded to `~/.openNexus/binaries/<agent>-<version>/` on first enable |

**Binary distribution notes:**

- Downloads match the current OS/arch (e.g. `darwin-aarch64`, `linux-x86_64`); connection fails if Registry has no entry for your platform
- Ensure the binary is executable; check logs for `安装 binary agent 失败` on download/extract errors
- In Docker, binary cache lives at `~/.openNexus/binaries/` inside the container — mount this path to avoid re-downloads
- Alpine containers use musl libc; some glibc-built binaries may not run — prefer host deployment or npx distribution

**Verify before enabling:**

```bash
# npx example (Claude Code)
npm exec --include=optional --yes @agentclientprotocol/claude-agent-acp@latest -- --help

# After enabling: click "Fetch Config" in Settings, or confirm sidebar shows "connected"
```

Workspace directory policy:

- **temporary**: Cleaned up only when the entire workspace is deleted; deleting a single session does not remove the shared directory; missing dirs are recreated on session resume
- **persistent**: Directory must exist beforehand; cleanup happens when the workspace is deleted

## Prompt Input

The chat input supports two completion modes (↑↓ select, Enter confirm, Esc go back or close):

| Trigger | Description |
|---------|-------------|
| `/` | Flat list of commands, skills, and modes |
| `@` | Hierarchical picker: choose type first (Command / Skill / File / Note), then pick an item |

`@` navigation:

1. **Command / Skill**: Insert `/name` (backend expands local command / skill file content)
2. **File**: Browse the session workspace; enter subdirectories; insert `@/absolute/path` for files
3. **Note**: Pick a tag, then a note; insert `@note:{id}`

The Notes page (`/notes`) supports quick capture, tag filtering, Markdown preview, and inline editing.

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make dev` | Start frontend + backend dev servers |
| `make run` | Single-port production mode |
| `make build` | Build frontend + backend |
| `make pake` | Build Pake desktop client only |
| `make desktop` | Build macOS desktop app (Apple Silicon) |
| `make desktop-linux` | Build Linux amd64 desktop app |
| `make desktop-windows` | Build Windows amd64 desktop app |
| `make test` | Run all backend tests |
| `make clean` | Clean build artifacts |
| `make docker-up` | Build Docker image and start |
| `make docker-down` | Stop and clean Docker containers |
| `make docker-logs` | View Docker container logs |

## Release Builds

Pushing a `v*` tag (e.g. `v1.0.0`) triggers GitHub Actions to build and publish a Release with:

| Platform | Desktop artifact | CLI artifact |
|----------|------------------|--------------|
| macOS Apple Silicon | `opennexus-darwin-desktop.tar.gz` | `opennexus-darwin-arm64.tar.gz` |
| Linux x86_64 | `opennexus-linux-desktop.tar.gz` | `opennexus-linux-amd64.tar.gz` |
| Windows x86_64 | `opennexus-windows-desktop.zip` | `opennexus-windows-amd64.zip` |

Local desktop builds require Rust, pnpm, and `pake-cli@3.13.0` — see `scripts/build-pake.sh`.

## License

Private project. All rights reserved.
