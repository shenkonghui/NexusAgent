# 统一数据目录至 ~/.nextAgent

## 背景

当前本地开发、Docker、Mac/Electron 安装版的数据路径不一致：

| 用途 | 本地开发 | Mac/Electron 安装版 |
|------|----------|---------------------|
| 数据库 | `./data/nexus.db` | `~/Library/Application Support/NexusAgent/nexus.db` |
| 临时会话工作区 | `~/.nextAgent/session` | Application Support 下的 `session/` |
| 配置文件 | 项目根 / App bundle 内 `config.yaml` | 同左（`CONFIG_PATH`） |

目标：数据库、临时会话工作区、配置文件统一落在 `~/.nextAgent/`，使直接启动与安装到 Mac 后数据互通。

## 目标目录结构

```
~/.nextAgent/
├── config.yaml   # 可选用户配置
├── nexus.db      # SQLite 数据库
└── session/      # temporary 模式会话工作区
```

## 配置加载顺序

采用「优先用户目录、可回退」策略：

1. 若设置了环境变量 `CONFIG_PATH`，使用该路径
2. 否则若存在 `~/.nextAgent/config.yaml`，使用该文件
3. 否则回退到当前工作目录下的 `config.yaml`

不自动从内置/仓库复制配置到用户目录；用户可自行放置。

## 默认路径

| 配置项 | 默认值 | 可覆盖方式 |
|--------|--------|------------|
| `database.path` | `~/.nextAgent/nexus.db` | `DATABASE_PATH`、`--data-dir` |
| `agents.workspace.session_dir` | `~/.nextAgent/session` | `AGENTS_WORKSPACE_SESSION_DIR`、`--data-dir` |
| 配置文件 | 见上方加载顺序 | `CONFIG_PATH` |

`--data-dir <dir>` 行为保持不变：将数据库设为 `<dir>/nexus.db`，会话目录设为 `<dir>/session`。

## 桌面启动脚本

Mac（Pake）、Electron、Linux 启动器中的数据目录统一改为：

```
$HOME/.nextAgent
```

不再使用：

- macOS：`~/Library/Application Support/NexusAgent`
- Linux：`${XDG_DATA_HOME:-~/.local/share}/NexusAgent`

启动时仍可传入 `--data-dir $HOME/.nextAgent`；`CONFIG_PATH` 可继续指向 App bundle 内默认配置（作为回退源）。若用户已在 `~/.nextAgent/config.yaml` 放置配置，则按加载顺序优先使用用户配置。

说明：若启动器同时设置了 `CONFIG_PATH`（指向 bundle）与用户目录配置，按「`CONFIG_PATH` 优先」规则，用户目录配置不会生效。为真正优先用户配置，启动器应：

- **不**再强制设置 `CONFIG_PATH` 指向 bundle；或
- 仅在用户目录无配置时设置 `CONFIG_PATH` 为 bundle 内配置

推荐：启动器只传 `--data-dir $HOME/.nextAgent`，由后端按加载顺序解析配置；bundle / 安装包旁的 `config.yaml` 作为 cwd 回退或由启动器在「用户配置不存在」时再设 `CONFIG_PATH`。

## Docker

容器环境不强制使用宿主 `~/.nextAgent`。继续通过环境变量显式指定容器内路径（现有 `DATABASE_PATH=/app/data/nexus.db`、`AGENTS_WORKSPACE_SESSION_DIR=/app/data/session`），与本次统一无关。

## 不做的范围

- 不自动迁移已有 `./data`、Application Support 等旧数据
- 不修改 Skills / Commands / Rules 的用户目录（仍为 `~/.claude/*`、`~/.cursor/*`）
- 不改变 Agent 二进制缓存路径（`~/.nexusagent/binaries/`）

## 涉及改动（实现指引）

1. `cmd/server/main.go`：实现配置路径解析顺序；默认不再依赖仅 cwd 相对路径
2. `internal/config/config.go`：`Validate` 中 database 默认改为 `~/.nextAgent/nexus.db`（若为空）
3. `config.yaml`：更新注释与默认 `database.path` 示例
4. `scripts/package-darwin.sh`、`scripts/package-linux.sh`、`electron/main.cjs`：数据目录改为 `$HOME/.nextAgent`，调整 `CONFIG_PATH` 与用户配置的优先级
5. README / electron README：同步路径说明
6. 相关单元测试：更新期望路径

## 验收标准

- 本地 `make run` / `go run ./cmd/server` 默认写入 `~/.nextAgent/nexus.db` 与 `~/.nextAgent/session`
- 存在 `~/.nextAgent/config.yaml` 时优先加载；否则可用 `CONFIG_PATH` 或 `./config.yaml`
- Mac/Electron 安装版与本地开发使用同一数据根目录
- Docker 行为不变（仍靠环境变量）
