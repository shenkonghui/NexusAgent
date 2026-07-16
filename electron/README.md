# NexusAgent Electron 客户端

NexusAgent 的 Electron 桌面壳,与现有 Pake(Tauri)客户端**并存**,可自由选择使用哪一种。

## 工作原理

```
Electron 主进程(main.cjs)
   │  spawn ./nexusagent --data-dir <userDataDir>   (release 模式)
   │  SERVER_PORT=<随机空闲端口> SERVER_MODE=release WEB_DIST=<web/dist>
   │
   ▼
Go 后端(单端口,同时服务 API + 前端静态文件)
   │  BrowserWindow.loadURL('http://127.0.0.1:<port>')
   ▼
React SPA(前端全部用相对路径 /api/v1,同源访问后端)
```

前端、后端**均无改动**:前端已用相对路径 `/api/v1`,SSE 与 WebSocket(终端)都基于 `window.location.host` 推导,Electron 加载后端同源页面即自动打通。

## 前置条件

- Go >= 1.25(编译后端)
- Node.js >= 20(前端构建 + Electron)
- 已执行过根目录的 `make build`,产出:
  - 根目录 `nexusagent`(Go 二进制)
  - `web/dist/`(前端构建产物)

## 常用命令(在项目根目录执行)

```bash
# 开发:先 make build 出后端+前端,再拉起 Electron 窗口
make electron-dev

# 打包当前平台桌面应用(macOS 产出 dmg)
make electron-dist

# 一键安装到本机 /Applications(macOS),含打包
make electron-install

# 启动已安装的应用
make electron-run

# 卸载
make electron-uninstall
```

## 数据目录与日志

本地开发与桌面安装版共用同一数据根目录：

| 平台 | 数据目录 |
|------|---------|
| 全部 | `~/.nextAgent` |

目录内容：`nexus.db`（数据库）、`session/`（临时会话工作区）、可选 `config.yaml`。

配置加载顺序：`CONFIG_PATH` → `~/.nextAgent/config.yaml` → 安装包/项目旁的 `config.yaml`。

后端日志写入 `~/.nextAgent/launcher.log`，启动失败时可在此排查。

## 与 Pake 版的差异

| 项 | Electron | Pake(Tauri) |
|----|---------|-------------|
| 渲染层 | 自带 Chromium | 系统 WebView |
| 安装体积 | ~90–130MB | ~15–20MB |
| 渲染一致性 | 三端一致 | 受系统 WebView 影响 |
| 打包工具 | electron-builder | pake-cli |
| 端口 | 动态空闲端口 | 固定 8080 |

## 手动构建(不走 Makefile)

```bash
cd electron
npm install
npm start        # 开发运行
npm run dist     # 打包到 electron/dist/
```

打包前需确保根目录已 `make build`(产出 `nexusagent` 二进制与 `web/dist`)。
