import { useRef, useEffect } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { X } from 'lucide-react'
import styles from './Terminal.module.css'

interface TerminalProps {
  sessionId: number
  /** 关闭终端时回调 */
  onClose: () => void
}

// 构建 WebSocket URL（从 access_token 和 baseURL 推导）
function buildTerminalURL(sessionId: number): string {
  const baseURL = import.meta.env.VITE_API_BASE || '/api/v1'
  const token = localStorage.getItem('access_token') || ''

  // 将 http(s) URL 转换为 ws(s)
  let wsBase: string
  if (baseURL.startsWith('http://')) {
    wsBase = 'ws://' + baseURL.slice(7)
  } else if (baseURL.startsWith('https://')) {
    wsBase = 'wss://' + baseURL.slice(8)
  } else {
    // 相对路径，从 window.location 推导
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    wsBase = `${proto}//${window.location.host}${baseURL}`
  }

  return `${wsBase}/sessions/${sessionId}/terminal?token=${encodeURIComponent(token)}`
}

export default function TerminalPanel({ sessionId, onClose }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<XTerm | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const fitRef = useRef<FitAddon | null>(null)

  useEffect(() => {
    if (!containerRef.current) return

    // 创建 xterm 实例
    const term = new XTerm({
      fontSize: 13,
      fontFamily: "'Monaco', 'Menlo', 'Courier New', monospace",
      cursorBlink: true,
      theme: {
        background: '#1e1e2e',
        foreground: '#cdd6f4',
        cursor: '#f5e0dc',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.loadAddon(new WebLinksAddon())
    term.open(containerRef.current)
    fit.fit()
    termRef.current = term
    fitRef.current = fit

    term.writeln('\x1b[36m正在连接终端...\x1b[0m')

    // 连接 WebSocket
    const wsURL = buildTerminalURL(sessionId)
    let ws: WebSocket
    try {
      ws = new WebSocket(wsURL)
    } catch (err) {
      term.writeln(`\x1b[31m无法创建 WebSocket 连接: ${err}\x1b[0m`)
      term.writeln('\x1b[33m请确认后端服务已启动\x1b[0m')
      return
    }
    wsRef.current = ws

    ws.binaryType = 'arraybuffer'

    // 连接超时检测：5 秒未连接则提示
    let connected = false
    const connectTimeout = setTimeout(() => {
      if (!connected) {
        term.clear()
        term.writeln('\x1b[31m连接超时，请检查：\x1b[0m')
        term.writeln('  1. 后端服务是否已启动 (默认 :8080)')
        term.writeln('  2. 会话工作目录是否存在')
        term.writeln('  3. 认证令牌是否有效')
        term.writeln('')
        term.writeln('\x1b[33m关闭终端面板后重新打开可重试\x1b[0m')
      }
    }, 5000)

    ws.onopen = () => {
      connected = true
      clearTimeout(connectTimeout)
      term.clear()
      // 发送初始 resize
      sendResize(term, ws, fit)
    }

    ws.onmessage = (event) => {
      const data = event.data instanceof ArrayBuffer
        ? new Uint8Array(event.data)
        : event.data
      term.write(data)
    }

    ws.onerror = () => {
      if (!connected) {
        clearTimeout(connectTimeout)
        term.clear()
        term.writeln('\x1b[31m终端连接失败\x1b[0m')
        term.writeln('\x1b[33m可能原因：后端未启动 / WebSocket 代理未配置 / 认证失败\x1b[0m')
      }
    }

    ws.onclose = () => {
      clearTimeout(connectTimeout)
      if (connected) {
        term.writeln('\r\n\x1b[33m终端已断开\x1b[0m')
      }
    }

    // 终端输入 -> WebSocket
    const inputDisposable = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data)
      }
    })

    // resize 处理
    const resizeDisposable = term.onResize(() => {
      sendResize(term, ws, fit)
    })

    // 容器尺寸变化（Tab 切换 display:none->flex、面板分栏拖动、窗口 resize）
    // 时重新 fit，避免终端尺寸与容器不符导致留白/错位。
    const resizeObserver = new ResizeObserver(() => {
      try { fit.fit() } catch { /* 容器未渲染好时忽略 */ }
    })
    resizeObserver.observe(containerRef.current)

    // 聚焦终端
    term.focus()

    return () => {
      clearTimeout(connectTimeout)
      inputDisposable.dispose()
      resizeDisposable.dispose()
      resizeObserver.disconnect()
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close()
      }
      term.dispose()
      termRef.current = null
      wsRef.current = null
    }
  }, [sessionId])

  // 面板尺寸变化时重新 fit
  useEffect(() => {
    if (!fitRef.current || !termRef.current) return
    const timer = setTimeout(() => {
      try {
        fitRef.current?.fit()
        termRef.current?.focus()
      } catch {
        // 忽略
      }
    }, 50)
    return () => clearTimeout(timer)
  }, [])

  function handleClose() {
    wsRef.current?.close()
    termRef.current?.dispose()
    onClose()
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <span className={styles.title}>终端</span>
        <button className={styles.closeBtn} onClick={handleClose} type="button" title="关闭终端">
          <X size={16} />
        </button>
      </div>
      <div ref={containerRef} className={styles.terminal} />
    </div>
  )
}

// 发送 resize 控制消息：0x01 + cols(2) + rows(2)
function sendResize(term: XTerm, ws: WebSocket, fit: FitAddon) {
  if (ws.readyState !== WebSocket.OPEN) return
  const cols = term.cols
  const rows = term.rows
  const buf = new Uint8Array(5)
  buf[0] = 0x01
  buf[1] = (cols >> 8) & 0xff
  buf[2] = cols & 0xff
  buf[3] = (rows >> 8) & 0xff
  buf[4] = rows & 0xff
  ws.send(buf)
  // fit 用于确保渲染正确
  try { fit.fit() } catch { /* 忽略 */ }
}
