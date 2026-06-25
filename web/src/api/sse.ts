import type { Message } from '../types'
import { getBaseURL, getAuthHeaders, clearTokensAndRedirect } from './client'

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
    msg.includes('连接超时') ||
    msg.includes('网络超时') ||
    msg.includes('network error') ||
    msg.includes('failed to fetch')
  )
}

// SSE 流式 prompt 发送
export async function streamPrompt(
  sessionId: number,
  prompt: string,
  onMessage: (msg: Message) => void,
  onDone: () => void,
  onError: (err: Error) => void,
): Promise<void> {
  const controller = new AbortController()

  try {
    const resp = await fetch(`${getBaseURL()}/sessions/${sessionId}/prompt`, {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify({ prompt }),
      signal: controller.signal,
    })

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

    const reader = resp.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })

      // 按 \n\n 分割 SSE 事件
      const events = buffer.split('\n\n')
      buffer = events.pop() || ''

      for (const event of events) {
        const lines = event.split('\n')
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6)
            if (data === '[DONE]') {
              onDone()
              return
            }
            try {
              const msg: Message = JSON.parse(data)
              onMessage(msg)
            } catch {
              // 忽略无法解析的行
            }
          }
        }
      }
    }

    // 处理缓冲区剩余数据
    if (buffer.trim()) {
      const lines = buffer.split('\n')
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6)
          if (data === '[DONE]') {
            onDone()
            return
          }
          try {
            const msg: Message = JSON.parse(data)
            onMessage(msg)
          } catch {
            // 忽略
          }
        }
      }
    }

    onDone()
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      onError(new Error('请求已取消'))
    } else if (err instanceof TypeError && err.message.includes('fetch')) {
      // 浏览器 fetch 网络错误（连接中断、DNS 失败等）
      onError(new Error('网络连接失败，可能是 agent 端超时或无响应'))
    } else {
      onError(err instanceof Error ? err : new Error('未知错误'))
    }
  }
}
