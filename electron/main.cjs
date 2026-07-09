/*
 * NexusAgent Electron 主进程
 *
 * 职责与现有 Pake 方案一致:
 *   1. 探测一个空闲端口(避免固定 8080 的冲突,支持多开)
 *   2. 拉起 Go 后端二进制(release 模式,单端口同时服务 API + 前端)
 *   3. 轮询 /health 直到后端就绪
 *   4. 打开 BrowserWindow 加载 http://127.0.0.1:<port>
 *   5. 应用退出时回收后端进程
 *
 * 前端与后端均无改动:前端全部使用相对路径 /api/v1,同源访问后端。
 */

const { app, BrowserWindow, shell, dialog } = require('electron')
const { spawn, execSync } = require('child_process')
const net = require('net')
const http = require('http')
const path = require('path')
const fs = require('fs')
const os = require('os')

let backend = null
let mainWindow = null
let backendCrashed = false
const LOG_TAG = '[nexusagent-electron]'

// 探测一个空闲端口传给后端(SERVER_PORT 环境变量,后端 config.go 已支持)。
function pickFreePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer()
    srv.unref()
    srv.on('error', reject)
    srv.listen(0, '127.0.0.1', () => {
      const port = srv.address().port
      srv.close(() => resolve(port))
    })
  })
}

// 数据目录(复用 Pake launcher.sh / launch.ps1 的约定,与 Pake 版数据互通)。
function userDataDir() {
  const home = os.homedir()
  switch (process.platform) {
    case 'win32':
      return path.join(process.env.LOCALAPPDATA || home, 'NexusAgent')
    case 'darwin':
      return path.join(home, 'Library', 'Application Support', 'NexusAgent')
    default:
      return path.join(process.env.XDG_DATA_HOME || path.join(home, '.local', 'share'), 'NexusAgent')
  }
}

// 定位 Go 后端二进制:打包后位于 resources/backend,开发模式为项目根编译产物。
function backendBinPath() {
  const binName = process.platform === 'win32' ? 'nexusagent.exe' : 'nexusagent'
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'backend', binName)
  }
  return path.join(__dirname, '..', binName)
}

// 定位 config.yaml:后端 main.go 优先读 CONFIG_PATH 环境变量,否则用相对路径
// (依赖 cwd)。这里统一用 CONFIG_PATH 指向绝对路径,避免 cwd 不确定导致加载失败。
function configPath() {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'config.yaml')
  }
  return path.join(__dirname, '..', 'config.yaml')
}

// 后端工作目录:开发模式用项目根(便于相对路径/日志定位),打包用 resources 目录。
function backendCwd() {
  return path.dirname(configPath())
}

// 拉起 Go 后端。将 stdout/stderr 落盘到数据目录 launcher.log,便于排障。
function startBackend(port) {
  const dataDir = userDataDir()
  fs.mkdirSync(dataDir, { recursive: true })

  const binPath = backendBinPath()
  if (!fs.existsSync(binPath)) {
    throw new Error(`后端二进制不存在: ${binPath}\n请先执行 \`make build\` (开发) 或重新打包。`)
  }

  const cfg = configPath()
  const env = {
    ...process.env,
    SERVER_PORT: String(port),
    SERVER_MODE: 'release',
    CONFIG_PATH: cfg,
    WEB_DIST: app.isPackaged
      ? path.join(process.resourcesPath, 'web')
      : path.join(__dirname, '..', 'web', 'dist'),
  }

  const logFile = path.join(dataDir, 'launcher.log')
  const logStream = fs.createWriteStream(logFile, { flags: 'a' })
  logStream.write(`\n${LOG_TAG} starting backend at ${new Date().toISOString()} port=${port} config=${cfg}\n`)

  backend = spawn(binPath, ['--data-dir', dataDir], {
    env,
    cwd: backendCwd(),
    stdio: ['ignore', 'pipe', 'pipe'],
  })

  backend.stdout.on('data', (d) => logStream.write(d))
  backend.stderr.on('data', (d) => logStream.write(d))
  backend.on('exit', (code, signal) => {
    console.log(`${LOG_TAG} backend exited code=${code} signal=${signal}`)
    logStream.write(`${LOG_TAG} backend exited code=${code} signal=${signal}\n`)
    // 后端非正常退出且窗口还在,标记崩溃以便阻止窗口反复 loadURL
    if (code !== 0 && code !== null) backendCrashed = true
    backend = null
  })

  return dataDir
}

// 轮询 /health(沿用 Pake launcher 的 ~30s 超时逻辑)。
function waitReady(port, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs
  return new Promise((resolve, reject) => {
    const tick = () => {
      if (backendCrashed) return reject(new Error('后端启动后立即退出,请查看 launcher.log'))
      if (Date.now() > deadline) return reject(new Error('等待后端就绪超时(30s)'))
      const req = http.get(`http://127.0.0.1:${port}/health`, (res) => {
        res.resume()
        if (res.statusCode === 200) resolve()
        else setTimeout(tick, 500)
      })
      req.on('error', () => setTimeout(tick, 500))
      req.setTimeout(2000, () => {
        req.destroy()
        setTimeout(tick, 500)
      })
    }
    tick()
  })
}

// 同步杀死后端进程树。Windows 下 SIGTERM 无效,需用 taskkill。
function killBackend() {
  if (!backend || backend.killed) return
  try {
    if (process.platform === 'win32') {
      // /t 杀掉子进程树(npx 拉起的 agent 子进程也要回收)
      spawn('taskkill', ['/pid', String(backend.pid), '/f', '/t'])
    } else {
      backend.kill('SIGTERM')
      // 兜底:3s 后仍未退出则 SIGKILL
      const pid = backend.pid
      setTimeout(() => {
        try {
          if (pid) process.kill(pid, 0) // 仍存活则抛错进入 catch
          process.kill(pid, 'SIGKILL')
        } catch {
          /* 已退出 */
        }
      }, 3000)
    }
  } catch (e) {
    console.error(`${LOG_TAG} kill backend failed:`, e)
  }
}

async function bootstrap() {
  let port
  try {
    port = await pickFreePort()
  } catch (e) {
    return fatal(`无法获取空闲端口: ${e.message}`)
  }

  let dataDir
  try {
    dataDir = startBackend(port)
  } catch (e) {
    return fatal(e.message)
  }

  try {
    await waitReady(port)
  } catch (e) {
    return fatal(`${e.message}\n\n日志: ${path.join(dataDir, 'launcher.log')}`)
  }

  mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 900,
    minHeight: 600,
    title: 'NexusAgent',
    icon: path.join(__dirname, 'icon.png'),
    show: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false, // preload.cjs 需用 node require
    },
  })

  // 创建后再显示,避免白屏
  mainWindow.once('ready-to-show', () => mainWindow.show())

  // 外链(http(s))用系统浏览器打开,避免在应用内跳走丢失会话
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:\/\//i.test(url)) {
      shell.openExternal(url)
      return { action: 'deny' }
    }
    return { action: 'allow' }
  })

  await mainWindow.loadURL(`http://127.0.0.1:${port}`)
}

// 弹出致命错误对话框并退出应用。
function fatal(message) {
  console.error(`${LOG_TAG} FATAL: ${message}`)
  if (app.isReady()) {
    dialog.showErrorBox('NexusAgent 启动失败', message)
  }
  app.quit()
}

// ---- 生命周期 ----
app.whenReady().then(bootstrap).catch((e) => fatal(e && e.message ? e.message : String(e)))

app.on('window-all-closed', () => {
  // 非单实例场景直接退出;macOS 由用户从 Dock 退出
  killBackend()
  if (process.platform !== 'darwin') app.quit()
})

app.on('before-quit', () => killBackend())
app.on('will-quit', () => killBackend())

// 防止多实例导致后端冲突(可选,提升体验)
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
} else {
  app.on('second-instance', () => {
    if (mainWindow) {
      if (mainWindow.isMinimized()) mainWindow.restore()
      mainWindow.focus()
    }
  })
}
