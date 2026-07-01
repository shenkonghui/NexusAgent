import type { Message } from '../types'
import { getBaseURL, getAuthHeaders, clearTokensAndRedirect } from './client'

const CONNECT_TIMEOUT_MS = 30_000
const IDLE_TIMEOUT_MS = 120_000

export interface StreamPromptOptions {
  signal?: AbortSignal
  onActivity?: () => void
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
): boolean {
  for (const event of events) {
    for (const line of event.split('\n')) {
      if (!line.startsWith('data: ')) continue
      const data = line.slice(6)
      if (data === '[DONE]') return true
      try {
        onMessage(JSON.parse(data) as Message)
        onActivity?.()
      } catch {
        // 忽略无法解析的行
      }
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
      const { done, value } = await readWithTimeout(reader, IDLE_TIMEOUT_MS, signal)
      if (done) break

      if (value?.length) options?.onActivity?.()
      buffer += decoder.decode(value, { stream: true })

      const events = buffer.split('\n\n')
      buffer = events.pop() || ''

      if (parseSSEEvents(events, onMessage, options?.onActivity)) {
        onDone()
        return
      }
    }

    if (buffer.trim() && parseSSEEvents([buffer], onMessage, options?.onActivity)) {
      onDone()
      return
    }

    onDone()
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      if (signal?.aborted) {
        onError(new Error('请求已取消'))
      } else {
        onError(new Error('连接超时，Agent 无响应'))
      }
    } else if (err instanceof TypeError && err.message.includes('fetch')) {
      onError(new Error('网络连接失败，可能是 agent 端超时或无响应'))
    } else {
      onError(err instanceof Error ? err : new Error('未知错误'))
    }
  }
}
