# ~/.openNexus 数据目录统一 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将数据库、临时会话工作区、配置文件默认统一到 `~/.openNexus/`，使本地启动与 Mac/Electron 安装版数据互通。

**Architecture:** 后端解析配置路径顺序为 `CONFIG_PATH` → `~/.openNexus/config.yaml` → `./config.yaml`；`Validate` 将空的 `database.path` 默认设为 `~/.openNexus/opennexus.db`；桌面启动器 `--data-dir` 改为 `$HOME/.openNexus`。Docker 仍用环境变量，不改。

**Tech Stack:** Go、config.yaml、shell 启动脚本、Electron main.cjs

## Global Constraints

- 配置优先序：`CONFIG_PATH` → `~/.openNexus/config.yaml` → `./config.yaml`
- 默认数据根：`~/.openNexus/`（opennexus.db、session/）
- 不做旧数据自动迁移
- Docker 路径保持环境变量覆盖
- 不主动 commit（除非用户要求）

---

### Task 1: 配置路径解析与数据库默认路径

**Files:**
- Modify: `cmd/server/main.go`（配置路径解析）
- Modify: `internal/config/config.go`（database 默认）
- Modify: `internal/config/config_test.go`
- Modify: `config.yaml`（注释与默认 path）

**Interfaces:**
- Produces: `resolveConfigPath() string`（可放在 main 或 config 包）
- Produces: `Validate` 后 `Database.Path` 默认为 `~/.openNexus/opennexus.db`

- [ ] **Step 1: 写失败测试（config 默认 database path）**

在 `internal/config/config_test.go` 增加：

```go
func TestValidate_DatabasePath_Default(t *testing.T) {
	cfg := &Config{
		JWT:    JWTConfig{Secret: strings.Repeat("x", 32)},
		Agents: AgentsConfig{Workspace: WorkspaceConfig{DefaultMode: "temporary"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, ".openNexus", "opennexus.db")
	if cfg.Database.Path != expected {
		t.Errorf("Database.Path = %q, 期望 %q", cfg.Database.Path, expected)
	}
}
```

- [ ] **Step 2: 实现 Validate 默认 database.path**

在 `Validate` 中，若 `c.Database.Path == ""`：

```go
home, err := os.UserHomeDir()
// ...
c.Database.Path = filepath.Join(home, ".openNexus", "opennexus.db")
```

并将 `config.yaml` 中 `database.path` 改为空或注释说明默认 `~/.openNexus/opennexus.db`（若保留显式值，开发时仍会覆盖默认——推荐改为注释掉 path 或写 `~/.openNexus/opennexus.db` 并在 Load 后 expandPath）。

推荐：`config.yaml` 写 `path: ~/.openNexus/opennexus.db`，在 `Validate` 或 `applyEnv` 后对 database.path 做 `expandPath`。

- [ ] **Step 3: 实现 resolveConfigPath**

在 `cmd/server/main.go`：

```go
func resolveConfigPath() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".openNexus", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "config.yaml"
}
```

用 `resolveConfigPath()` 替换现有 cfgPath 逻辑。

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/config/ ./cmd/server/ -count=1`
Expected: PASS

---

### Task 2: 桌面启动器改用 ~/.openNexus

**Files:**
- Modify: `scripts/package-darwin.sh`
- Modify: `scripts/package-linux.sh`
- Modify: `electron/main.cjs`
- Modify: `electron/README.md`、`README.zh-CN.md`、`README.md`（路径说明）

- [ ] **Step 1: 改 darwin/linux DATA_DIR**

```bash
DATA_DIR="$HOME/.openNexus"
```

启动器：仅当 `~/.openNexus/config.yaml` 不存在时，再 `export CONFIG_PATH` 指向 bundle/旁路默认配置；若用户配置存在则不设 `CONFIG_PATH`。

- [ ] **Step 2: 改 electron userDataDir**

```js
function userDataDir() {
  return path.join(os.homedir(), '.openNexus')
}
```

`startBackend` 中：若 `~/.openNexus/config.yaml` 存在则不设 `CONFIG_PATH`，否则设为 bundle/项目 config。

- [ ] **Step 3: 更新 README 中 database 默认路径说明**

---

### Task 3: 验证

- [ ] **Step 1:** `go test ./internal/config/ -count=1`
- [ ] **Step 2:** 确认 `config.yaml` / README / 启动脚本路径一致
