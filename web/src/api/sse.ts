import type { Message } from '../types'
import { getBaseURL, getAuthHeaders, clearTokensAndRedirect } from './client'
import { logger } from '../utils/logger'

const CONNECT_TIMEOUT_MS = 30_000
const IDLE_TIMEOUT_MS = 120_000

export interface StreamPromptOptions {
  signal?: AbortSignal
  onActivity?: () => void
  onSeq?: (seq: number) => void
  /** 返回 true 时暂停空闲超时（例如等待用户审批权限） */
  shouldPauseIdleTimeout?: () => boolean
}

const PAUSED_IDLE_TIMEOUT_MS = 24 * 60 * 60 * 1000

function idleTimeoutMs(options?: StreamPromptOptions): number {
  if (options?.shouldPauseIdleTimeout?.()) return PAUSED_IDLE_TIMEOUT_MS
  return IDLE_TIMEOUT_MS
}

// 判断错误是否为超时/agent 端无响应
export function isTimeoutError(err: Error): boolean {
  const msg = err.message.toLowerCase()
  return (
    msg.includes('timeout') ||
    msg.includes('timed out') ||
    msg.includes('deadline exceeded') ||
    msg.includes('deadline_exceeded') ||
    msg.includes('context deadline') ||
    msg.includes('agent 超时') ||
    msg.includes('agent无响应') ||
    msg.includes('agent 响应超时') ||
    msg.includes('连接超时') ||
    msg.includes('网络超时') ||
    msg.includes('network error') ||
    msg.includes('failed to fetch')
  )
}

// 判断错误是否为"会话不在活跃状态"——agent 进程退出/重连失败等使会话进入 error/closed。
// 此类错误下挂起权限的接收方已失效，调用方应清除死权限栏而非保留。
export function isSessionInactiveError(errOrMsg: Error | string): boolean {
  const msg = (typeof errOrMsg === 'string' ? errOrMsg : errOrMsg.message).toLowerCase()
  return (
    msg.includes('不在活跃状态') ||
    msg.includes('not active') ||
    msg.includes('session_not_active')
  )
}

function readWithTimeout(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  timeoutMs: number,
  signal?: AbortSignal,
): Promise<ReadableStreamReadResult<Uint8Array>> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('Agent 响应超时')), timeoutMs)
    const onAbort = () => {
      clearTimeout(timer)
      reject(new DOMException('Aborted', 'AbortError'))
    }
    signal?.addEventListener('abort', onAbort, { once: true })
    reader.read().then(
      (result) => {
        clearTimeout(timer)
        signal?.removeEventListener('abort', onAbort)
        resolve(result)
      },
      (err) => {
        clearTimeout(timer)
        signal?.removeEventListener('abort', onAbort)
        reject(err)
      },
    )
  })
}

function parseSSEEvents(
  events: string[],
  onMessage: (msg: Message) => void,
  onActivity?: () => void,
  onSeq?: (seq: number) => void,
): boolean {
  for (const event of events) {
    // 一个 SSE event 块可能同时含 id: 和 data: 行
    let eventSeq: number | null = null
    const dataLines: string[] = []
    for (const line of event.split('\n')) {
      if (line.startsWith('id: ')) {
        const n = Number(line.slice(4))
        if (!Number.isNaN(n)) eventSeq = n
      } else if (line.startsWith('data: ')) {
        dataLines.push(line.slice(6))
      }
    }
    if (dataLines.length === 0) continue
    const data = dataLines.join('')
    if (data === '[DONE]') return true
    try {
      const msg = JSON.parse(data) as Message
      // 优先用 SSE id: 行的 sequence，回退到消息体自身的 sequence 字段
      if (eventSeq !== null) msg.sequence = eventSeq
      onMessage(msg)
      if (msg.sequence > 0) onSeq?.(msg.sequence)
      onActivity?.()
    } catch {
      // 忽略无法解析的行
    }
  }
  return false
}

// SSE 流式 prompt 发送
export async function streamPrompt(
  sessionId: number,
  prompt: string,
  onMessage: (msg: Message) => void,
  onDone: () => void,
  onError: (err: Error) => void,
  options?: StreamPromptOptions,
): Promise<void> {
  const signal = options?.signal

  try {
    const connectController = new AbortController()
    const onExternalAbort = () => connectController.abort()
    signal?.addEventListener('abort', onExternalAbort, { once: true })
    const connectTimeout = setTimeout(() => connectController.abort(), CONNECT_TIMEOUT_MS)

    let resp: Response
    try {
      resp = await fetch(`${getBaseURL()}/sessions/${sessionId}/prompt`, {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({ prompt }),
        signal: connectController.signal,
      })
    } finally {
      clearTimeout(connectTimeout)
      signal?.removeEventListener('abort', onExternalAbort)
    }

    if (!resp.ok) {
      if (resp.status === 401) {
        clearTokensAndRedirect()
        onError(new Error('认证已过期，请重新登录'))
        return
      }
      const errBody = await resp.json().catch(() => null)
      const msg = errBody?.error?.message || `请求失败 (${resp.status})`
      onError(new Error(msg))
      return
    }

    if (!resp.body) {
      onError(new Error('响应体为空'))
      return
    }

    options?.onActivity?.()

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await readWithTimeout(reader, idleTimeoutMs(options), signal)
      if (done) break

      if (value?.length) options?.onActivity?.()
      buffer += decoder.decode(value, { stream: true })

      const events = buffer.split('\n\n')
      buffer = events.pop() || ''

      if (parseSSEEvents(events, onMessage, options?.onActivity, options?.onSeq)) {
        onDone()
        return
      }
    }

    if (buffer.trim() && parseSSEEvents([buffer], onMessage, options?.onActivity, options?.onSeq)) {
      onDone()
      return
    }

    onDone()
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      if (signal?.aborted) {
        onError(new Error('请求已取消'))
      } else {
        logger.warn('sse', 'SSE 连接超时，Agent 无响应')
        onError(new Error('连接超时，Agent 无响应'))
      }
    } else if (err instanceof TypeError && err.message.includes('fetch')) {
      logger.warn('sse', 'SSE 网络连接失败')
      onError(new Error('网络连接失败，可能是 agent 端超时或无响应'))
    } else {
      logger.error('sse', `SSE 异常: ${err instanceof Error ? err.message : String(err)}`)
      onError(err instanceof Error ? err : new Error('未知错误'))
    }
  }
}

// 订阅会话当前进行中的 prompt 流（断点续传），不发起新 prompt。
// lastSeq 为客户端最后收到的 message sequence，服务端据此补齐遗漏消息并续接实时流。
// 若会话当前无活跃 prompt，服务端补齐遗漏消息后立即返回 [DONE]。
export async function subscribeStream(
  sessionId: number,
  lastSeq: number,
  onMessage: (msg: Message) => void,
  onDone: () => void,
  onError: (err: Error) => void,
  options?: StreamPromptOptions,
): Promise<void> {
  const signal = options?.signal
  try {
    const connectController = new AbortController()
    const onExternalAbort = () => connectController.abort()
    signal?.addEventListener('abort', onExternalAbort, { once: true })
    const connectTimeout = setTimeout(() => connectController.abort(), CONNECT_TIMEOUT_MS)

    const headers = getAuthHeaders()
    if (lastSeq > 0) headers['Last-Event-ID'] = String(lastSeq)

    let resp: Response
    try {
      resp = await fetch(`${getBaseURL()}/sessions/${sessionId}/stream`, {
        method: 'GET',
        headers,
        signal: connectController.signal,
      })
    } finally {
      clearTimeout(connectTimeout)
      signal?.removeEventListener('abort', onExternalAbort)
    }

    if (!resp.ok) {
      if (resp.status === 401) {
        clearTokensAndRedirect()
        onError(new Error('认证已过期，请重新登录'))
        return
      }
      const errBody = await resp.json().catch(() => null)
      const msg = errBody?.error?.message || `请求失败 (${resp.status})`
      onError(new Error(msg))
      return
    }

    if (!resp.body) {
      onError(new Error('响应体为空'))
      return
    }

    options?.onActivity?.()

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await readWithTimeout(reader, idleTimeoutMs(options), signal)
      if (done) break
      if (value?.length) options?.onActivity?.()
      buffer += decoder.decode(value, { stream: true })

      const events = buffer.split('\n\n')
      buffer = events.pop() || ''

      if (parseSSEEvents(events, onMessage, options?.onActivity, options?.onSeq)) {
        onDone()
        return
      }
    }

    if (buffer.trim() && parseSSEEvents([buffer], onMessage, options?.onActivity, options?.onSeq)) {
      onDone()
      return
    }

    onDone()
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      if (signal?.aborted) {
        onError(new Error('请求已取消'))
      } else {
        logger.warn('sse', 'SSE 连接超时，Agent 无响应')
        onError(new Error('连接超时，Agent 无响应'))
      }
    } else if (err instanceof TypeError && err.message.includes('fetch')) {
      logger.warn('sse', 'SSE 网络连接失败')
      onError(new Error('网络连接失败，可能是 agent 端超时或无响应'))
    } else {
      logger.error('sse', `SSE 异常: ${err instanceof Error ? err.message : String(err)}`)
      onError(err instanceof Error ? err : new Error('未知错误'))
    }
  }
}

// 恢复中断的任务并流式接收输出（ResumeSession + 重发原 prompt）。
// 端点 POST /running-tasks/:taskId/resume 返回与 /prompt 相同格式的 SSE 流。
export async function streamResumeTask(
  taskId: number,
  onMessage: (msg: Message) => void,
  onDone: () => void,
  onError: (err: Error) => void,
  options?: StreamPromptOptions,
): Promise<void> {
  const signal = options?.signal
  try {
    const connectController = new AbortController()
    const onExternalAbort = () => connectController.abort()
    signal?.addEventListener('abort', onExternalAbort, { once: true })
    const connectTimeout = setTimeout(() => connectController.abort(), CONNECT_TIMEOUT_MS)

    let resp: Response
    try {
      resp = await fetch(`${getBaseURL()}/running-tasks/${taskId}/resume`, {
        method: 'POST',
        headers: getAuthHeaders(),
        signal: connectController.signal,
      })
    } finally {
      clearTimeout(connectTimeout)
      signal?.removeEventListener('abort', onExternalAbort)
    }

    if (!resp.ok) {
      if (resp.status === 401) {
        clearTokensAndRedirect()
        onError(new Error('认证已过期，请重新登录'))
        return
      }
      const errBody = await resp.json().catch(() => null)
      const msg = errBody?.error?.message || `请求失败 (${resp.status})`
      onError(new Error(msg))
      return
    }

    if (!resp.body) {
      onError(new Error('响应体为空'))
      return
    }

    options?.onActivity?.()

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await readWithTimeout(reader, idleTimeoutMs(options), signal)
      if (done) break
      if (value?.length) options?.onActivity?.()
      buffer += decoder.decode(value, { stream: true })

      const events = buffer.split('\n\n')
      buffer = events.pop() || ''

      if (parseSSEEvents(events, onMessage, options?.onActivity, options?.onSeq)) {
        onDone()
        return
      }
    }

    if (buffer.trim() && parseSSEEvents([buffer], onMessage, options?.onActivity, options?.onSeq)) {
      onDone()
      return
    }

    onDone()
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      if (signal?.aborted) {
        onError(new Error('请求已取消'))
      } else {
        logger.warn('sse', 'SSE 连接超时，Agent 无响应')
        onError(new Error('连接超时，Agent 无响应'))
      }
    } else if (err instanceof TypeError && err.message.includes('fetch')) {
      logger.warn('sse', 'SSE 网络连接失败')
      onError(new Error('网络连接失败，可能是 agent 端超时或无响应'))
    } else {
      logger.error('sse', `SSE 异常: ${err instanceof Error ? err.message : String(err)}`)
      onError(err instanceof Error ? err : new Error('未知错误'))
    }
  }
}
