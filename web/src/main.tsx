import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './styles/global.css'
import './i18n'
import { logger } from './utils/logger'

// 全局错误捕获：未处理的运行时异常与 Promise rejection 写入前端日志查看器，
// 便于在左下角日志面板中排查。在 React render 之前安装，覆盖初始化阶段错误。
window.addEventListener('error', (e) => {
  const detail = e.error?.stack || e.message
  logger.error('runtime', `${e.message}${detail && detail !== e.message ? `\n${detail}` : ''}`)
})
window.addEventListener('unhandledrejection', (e) => {
  const reason = e.reason
  const msg = reason instanceof Error ? reason.message : String(reason)
  const stack = reason instanceof Error ? reason.stack : ''
  logger.error('promise', `${msg}${stack ? `\n${stack}` : ''}`)
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
