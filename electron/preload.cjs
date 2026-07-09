/*
 * NexusAgent 预加载脚本
 *
 * 当前前端完全通过 HTTP/SSE/WebSocket 与后端交互,不需要 Node 能力。
 * 这里仅暴露最小运行时信息(platform / versions),为未来扩展(如原生文件对话框、
 * 系统托盘、自动更新)预留一个 contextBridge 入口。
 *
 * 安全:contextIsolation=true,渲染进程拿不到 require/process。
 */

const { contextBridge } = require('electron')

contextBridge.exposeInMainWorld('nexusagent', {
  platform: process.platform,
  versions: {
    electron: process.versions.electron,
    node: process.versions.node,
    chrome: process.versions.chrome,
  },
  isElectron: true,
})
