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
}

func NewNoteClassifyWorker(classifier *NoteClassifier) *NoteClassifyWorker {
	return &NoteClassifyWorker{
		classifier: classifier,
		stopCh:     make(chan struct{}),
	}
}

func (w *NoteClassifyWorker) Start() {
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
}

func (w *NoteClassifyWorker) Stop() {
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
