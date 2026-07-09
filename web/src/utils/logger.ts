import type { LogLevel, LogEntry } from '../types'

// 前端日志查看器的内存环形缓冲容量上限。
// 超过后丢弃最旧条目，避免长时间运行占用过多内存。
const MAX_ENTRIES = 500

type Listener = (entry: LogEntry) => void

/**
 * 前端日志单例。在内存中维护最近 MAX_ENTRIES 条日志，
 * 并通过发布-订阅通知订阅者（LogPanel）实时刷新。
 *
 * 不依赖任何业务模块（不 import client/apiFetch），避免循环依赖。
 * 各业务模块（api/client.ts、api/sse.ts、main.tsx 的全局错误捕获）
 * 通过 logger.xxx(source, message) 写入日志。
 */
class Logger {
  private entries: LogEntry[] = []
  private seq = 0
  private listeners = new Set<Listener>()

  /** 写入一条日志并通知订阅者。 */
  private push(level: LogLevel, source: string, message: string): void {
    this.seq += 1
    const entry: LogEntry = {
      seq: this.seq,
      timestamp: new Date().toISOString(),
      level,
      source,
      message,
    }
    this.entries.push(entry)
    if (this.entries.length > MAX_ENTRIES) {
      this.entries.splice(0, this.entries.length - MAX_ENTRIES)
    }
    // 同步通知订阅者。即便抛错也不应影响调用方业务，故吞掉异常。
    this.listeners.forEach((fn) => {
      try {
        fn(entry)
      } catch {
        /* 忽略订阅者异常 */
      }
    })
  }

  debug(source: string, message: string): void {
    this.push('debug', source, message)
  }

  info(source: string, message: string): void {
    this.push('info', source, message)
  }

  warn(source: string, message: string): void {
    this.push('warn', source, message)
  }

  error(source: string, message: string): void {
    this.push('error', source, message)
  }

  /** 订阅新日志事件，返回取消订阅函数。 */
  subscribe(fn: Listener): () => void {
    this.listeners.add(fn)
    return () => this.listeners.delete(fn)
  }

  /** 返回当前缓冲中的所有日志（按时间升序）的拷贝。 */
  getAll(): LogEntry[] {
    return this.entries.slice()
  }

  /** 清空所有日志。 */
  clear(): void {
    this.entries = []
    // seq 不重置，避免与仍持有旧引用的订阅者产生 seq 冲突的错觉
  }
}

// 导出单例。整个应用共享同一份前端日志缓冲。
export const logger = new Logger()
