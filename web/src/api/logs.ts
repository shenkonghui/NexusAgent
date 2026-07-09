import type { LogEntry } from '../types'
import { getBaseURL, getAuthHeaders } from './client'

export interface StreamBackendLogsOptions {
  /** 收到一条日志时回调。 */
  onLog: (entry: LogEntry) => void
  /** 连接出错时回调（例如认证失败、网络断开）。 */
  onError?: (err: Error) => void
  /** 可选 AbortSignal，用于面板关闭/切走时断开连接。 */
  signal?: AbortSignal
  /** 最低日志等级过滤（透传给后端 ?level=）。 */
  level?: string
  /** 起始 seq（> since 的历史日志才会被推送）。 */
  since?: number
}

/**
 * 订阅后端日志 SSE 流：GET /api/v1/logs/stream。
 *
 * 用 fetch + ReadableStream 而非 EventSource，以便通过 Authorization Header
 * 携带 JWT（复用 getAuthHeaders）。SSE 块解析逻辑与 api/sse.ts 一致：
 *   id: <seq>\ndata: <json>\n\n
 *
 * 返回一个断开函数；调用方也可通过 AbortSignal 主动断开。
 */
export function streamBackendLogs(opts: StreamBackendLogsOptions): () => void {
  const { onLog, onError, signal, level, since } = opts
  // 内部 controller：既能被外部 signal 触发，也能被返回的 cleanup 触发
  const controller = new AbortController()
  const onExternalAbort = () => controller.abort()
  signal?.addEventListener('abort', onExternalAbort, { once: true })

  const cleanup = () => {
    signal?.removeEventListener('abort', onExternalAbort)
    controller.abort()
  }

  const params = new URLSearchParams()
  if (level) params.set('level', level)
  if (since !== undefined && since > 0) params.set('since', String(since))
  const query = params.toString() ? `?${params.toString()}` : ''

  ;(async () => {
    try {
      const resp = await fetch(`${getBaseURL()}/logs/stream${query}`, {
        method: 'GET',
        headers: { ...getAuthHeaders(), Accept: 'text/event-stream' },
        signal: controller.signal,
      })
      if (!resp.ok || !resp.body) {
        onError?.(new Error(`日志流连接失败 (${resp.status})`))
        return
      }

      const reader = resp.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })

        // 按 SSE 事件分隔符切分，最后一块可能不完整，保留在 buffer
        const events = buffer.split('\n\n')
        buffer = events.pop() || ''

        for (const event of events) {
          const dataLines: string[] = []
          for (const line of event.split('\n')) {
            if (line.startsWith('data: ')) {
              dataLines.push(line.slice(6))
            }
          }
          if (dataLines.length === 0) continue
          const data = dataLines.join('')
          if (data === '[DONE]') return
          try {
            const entry = JSON.parse(data) as LogEntry
            onLog(entry)
          } catch {
            /* 忽略无法解析的行 */
          }
        }
      }
    } catch (err) {
      // 主动 abort 不算错误
      if (controller.signal.aborted) return
      onError?.(err instanceof Error ? err : new Error('日志流读取失败'))
    }
  })()

  return cleanup
}
