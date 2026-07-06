// kb_index_worker.go 实现 knowledge.extract.done 事件的消费者（index-worker）。
//
// 对应《Bellkeeper 1.0 重构与架构演进规划》§2.1.2：
//   extract-worker 入库 → 发 knowledge.extract.done → 本 worker 消费 →
//   对刚写入的文件调 KnowledgeIndexService.IndexFile 刷新 Meili 索引。
//
// 取代原 KnowledgeIndexService 全量/增量定时扫描对单文件入库的延迟索引：
// 入库即触发单文件索引，近实时可见。

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/singll/bellkeeper/internal/eventbus"
	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// KBIndexWorker 消费 knowledge.extract.done 事件，刷新 Meili 索引。
type KBIndexWorker struct {
	bus            *eventbus.Client
	indexService   *KnowledgeIndexService
	maxRetry       int

	sub    *nats.Subscription
	stopCh chan struct{}
	wg     sync.WaitGroup
	running bool
	mu     sync.Mutex
}

// NewKBIndexWorker 构造 index-worker。
func NewKBIndexWorker(bus *eventbus.Client, indexService *KnowledgeIndexService, maxRetry int) *KBIndexWorker {
	return &KBIndexWorker{
		bus:          bus,
		indexService: indexService,
		maxRetry:     maxRetry,
		stopCh:       make(chan struct{}),
	}
}

// Start 订阅 knowledge.extract.done 并启动消费循环。
func (w *KBIndexWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	if w.bus == nil {
		middleware.GetLogger().Info("kb index worker disabled (eventbus not configured)")
		return nil
	}

	sub, err := w.bus.Subscribe("knowledge.extract.done", "bellkeeper-index-worker")
	if err != nil {
		return fmt.Errorf("kb index worker subscribe: %w", err)
	}
	w.sub = sub

	w.wg.Add(1)
	go w.consumeLoop(ctx)
	middleware.GetLogger().Info("kb index worker started")
	return nil
}

// Stop 优雅停止。
func (w *KBIndexWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()
	middleware.GetLogger().Info("kb index worker stopped")
}

func (w *KBIndexWorker) consumeLoop(ctx context.Context) {
	defer w.wg.Done()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			msgs, err := w.sub.Fetch(10, nats.MaxWait(5*time.Second))
			if err != nil {
				if err == nats.ErrTimeout {
					continue
				}
				middleware.GetLogger().Warn("kb index worker fetch error", zap.Error(err))
				time.Sleep(time.Second)
				continue
			}
			for _, msg := range msgs {
				w.processMessage(ctx, msg)
			}
		}
	}
}

func (w *KBIndexWorker) processMessage(ctx context.Context, msg *nats.Msg) {
	ev, err := eventbus.UnmarshalEvent(msg.Data)
	if err != nil {
		middleware.GetLogger().Error("kb index worker: unmarshal event envelope", zap.Error(err))
		_ = msg.Nak()
		return
	}
	var payload KBExtractDonePayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		middleware.GetLogger().Error("kb index worker: unmarshal extract.done payload", zap.Error(err))
		_ = msg.Nak()
		return
	}

	if payload.FilePath == "" {
		// 无文件路径（duplicate/skipped），无需索引，直接 Ack。
		_ = msg.Ack()
		return
	}

	if err := w.indexService.IndexFile(ctx, payload.FilePath); err != nil {
		metadata, _ := msg.Metadata()
		deliveryCount := 1
		if metadata != nil {
			deliveryCount = int(metadata.NumDelivered)
		}
		middleware.GetLogger().Error("kb index worker: index file failed",
			zap.String("file_path", payload.FilePath),
			zap.Error(err))
		if deliveryCount <= w.maxRetry {
			delay := time.Duration(deliveryCount) * 10 * time.Second
			if delay > 2*time.Minute {
				delay = 2 * time.Minute
			}
			_ = msg.NakWithDelay(delay)
			return
		}
		// 超重试：Ack 放弃（Meili 增量扫描会兜底补索引）。
		_ = msg.Ack()
		return
	}

	_ = msg.Ack()
	middleware.GetLogger().Info("kb index worker: indexed",
		zap.Uint("job_id", payload.JobID),
		zap.String("file_path", payload.FilePath))
}