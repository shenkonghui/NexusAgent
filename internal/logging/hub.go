package logging

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"nexusagent/internal/models"
)

// defaultBufferSize 是每个订阅者 channel 的缓冲大小。
// 与 msgBroadcaster 一致，缓冲满时丢弃，避免慢消费者阻塞日志生产。
const defaultBufferSize = 256

// defaultRingCapacity 是内存环形缓冲保留的历史日志条数上限。
const defaultRingCapacity = 500

// LogHub 维护一份内存中的最近日志（环形缓冲）以及一组实时订阅者。
// 它是自定义 slog Handler 与 SSE 日志端点之间的桥梁：
//   - slog Handler 在每条日志产生时调用 Append；
//   - SSE 端点通过 Subscribe 获取历史快照 + 实时 channel。
//
// 设计参考 internal/acp/broadcaster.go 的 msgBroadcaster。
type LogHub struct {
	ringCapacity int

	mu          sync.RWMutex
	ring        []models.LogEntry // 环形缓冲，按时间顺序存储
	ringStart   int               // ring[ringStart] 是最旧条目的下标
	ringLen     int               // 当前已存条数（<= ringCapacity）
	subs        []chan models.LogEntry
	closed      bool

	// seq 是单调递增的日志序号，0 起步。用原子操作避免与 mu 抢锁。
	// 注意：seq 的分配在 Append 入口完成，确保即便丢弃也保持递增。
	seq int64
}

// NewLogHub 创建日志中心。ringCapacity<=0 时使用默认值。
func NewLogHub(ringCapacity int) *LogHub {
	if ringCapacity <= 0 {
		ringCapacity = defaultRingCapacity
	}
	return &LogHub{
		ringCapacity: ringCapacity,
		ring:         make([]models.LogEntry, ringCapacity),
	}
}

// Append 写入一条日志：分配 seq、写入环形缓冲、非阻塞广播给订阅者。
// 对缓冲满的慢订阅者丢弃日志（写 stderr 记录，避免 slog 自循环）。
func (h *LogHub) Append(entry models.LogEntry) {
	entry.Seq = atomic.AddInt64(&h.seq, 1)

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	// 写入环形缓冲
	if h.ringLen < h.ringCapacity {
		idx := (h.ringStart + h.ringLen) % h.ringCapacity
		h.ring[idx] = entry
		h.ringLen++
	} else {
		// 缓冲已满，覆盖最旧条目
		h.ring[h.ringStart] = entry
		h.ringStart = (h.ringStart + 1) % h.ringCapacity
	}
	// 快照订阅者 channel 列表，释放锁后再发送
	subs := make([]chan models.LogEntry, len(h.subs))
	copy(subs, h.subs)
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
			// 慢消费者：直接写 stderr，不能用 slog（会导致递归广播）
			fmt.Fprintf(os.Stderr, "[logHub] 订阅者 buffer 满，丢弃日志 seq=%d\n", entry.Seq)
		}
	}
}

// Subscribe 注册一个新订阅者，返回其专属 channel 以及订阅时刻的历史快照。
// 调用方应先处理历史快照（按 seq 升序），再消费 channel，以避免遗漏或重复。
// 返回的 channel 在 Close 后会被关闭；调用方无需显式取消订阅。
func (h *LogHub) Subscribe(bufSize int) (<-chan models.LogEntry, []models.LogEntry) {
	if bufSize <= 0 {
		bufSize = defaultBufferSize
	}
	ch := make(chan models.LogEntry, bufSize)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch, nil
	}
	h.subs = append(h.subs, ch)
	snapshot := h.snapshotLocked()
	h.mu.Unlock()
	return ch, snapshot
}

// Unsubscribe 移除并关闭指定订阅者 channel。允许多次调用（幂等）。
func (h *LogHub) Unsubscribe(ch <-chan models.LogEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, c := range h.subs {
		if c == ch {
			h.subs = append(h.subs[:i], h.subs[i+1:]...)
			close(c)
			break
		}
	}
}

// Recent 返回最近 n 条日志（按时间升序）。n<=0 或超过已有条数时返回全部。
func (h *LogHub) Recent(n int) []models.LogEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.snapshotLockedN(n)
}

// Close 关闭所有订阅者 channel，并拒绝后续 Append。
// 用于进程关闭时清理资源。
func (h *LogHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, ch := range h.subs {
		close(ch)
	}
	h.subs = nil
}

// snapshotLocked 返回当前环形缓冲的完整快照（调用方持读锁）。
func (h *LogHub) snapshotLocked() []models.LogEntry {
	return h.snapshotLockedN(-1)
}

// snapshotLockedN 返回最近 n 条日志（n<=0 表示全部）。调用方持读锁。
// 返回的切片是拷贝，调用方可安全持有。
func (h *LogHub) snapshotLockedN(n int) []models.LogEntry {
	if h.ringLen == 0 {
		return nil
	}
	count := h.ringLen
	start := h.ringStart
	if n > 0 && n < count {
		// 只取最近 n 条：跳过前面 (count-n) 条
		skip := count - n
		start = (h.ringStart + skip) % h.ringCapacity
		count = n
	}
	out := make([]models.LogEntry, count)
	for i := 0; i < count; i++ {
		idx := (start + i) % h.ringCapacity
		out[i] = h.ring[idx]
	}
	return out
}

// LastSeq 返回当前已分配的最大 seq（用于测试与调试）。
func (h *LogHub) LastSeq() int64 {
	return atomic.LoadInt64(&h.seq)
}
