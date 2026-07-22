package services

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	NoteClassifyTickInterval = 1 * time.Minute
	NoteClassifyBatchSize    = 20
)

// NoteClassifyWorker 周期性处理待分类笔记队列。
type NoteClassifyWorker struct {
	classifier *NoteClassifier
	stopCh     chan struct{}
	wg         sync.WaitGroup
	// startOnce / stopOnce 保证 Start/Stop 幂等，参照 SchedulerService 的既定模式。
	// 多次 Start 会派生多余 goroutine；多次 Stop 会触发 close-of-closed-channel panic。
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewNoteClassifyWorker(classifier *NoteClassifier) *NoteClassifyWorker {
	return &NoteClassifyWorker{
		classifier: classifier,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动笔记分类 worker。幂等：多次调用仅启动一次。
func (w *NoteClassifyWorker) Start() {
	w.startOnce.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			ticker := time.NewTicker(NoteClassifyTickInterval)
			defer ticker.Stop()
			for {
				select {
				case <-w.stopCh:
					return
				case <-ticker.C:
					w.process(context.Background())
				}
			}
		}()
		log.Printf("笔记分类队列已启动，每分钟扫描待分类笔记")
	})
}

// Stop 停止笔记分类 worker。幂等：多次调用安全（sync.Once 防止 close-of-closed-channel panic）。
func (w *NoteClassifyWorker) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		done := make(chan struct{})
		go func() {
			w.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			log.Printf("笔记分类 worker 退出超时，继续关闭")
		}
	})
}

func (w *NoteClassifyWorker) process(ctx context.Context) {
	if w.classifier == nil {
		return
	}
	n, err := w.classifier.ProcessPending(ctx, NoteClassifyBatchSize)
	if err != nil {
		log.Printf("笔记分类队列处理失败: %v", err)
		return
	}
	if n > 0 {
		log.Printf("笔记分类队列本轮完成 %d 条", n)
	}
}
