// kb_extract_worker.go 实现 knowledge.crawl.done 事件的消费者（extract-worker）。
//
// 对应《Bellkeeper 1.0 重构与架构演进规划》§2.1.2：
//   crawl worker 抓取+提取 → 发 knowledge.crawl.done → 本 worker 消费 →
//   调 IngestURL 入库（写文件 + article_tags DB）→ 发 knowledge.extract.done →
//   index-worker 刷新 Meili。
//
// DB 表 crawl_jobs.status 为状态真相源：本 worker 入库成功后把 crawled → success/skipped。
// 至少一次投递：失败 NakWithDelay 重试，超 maxRetries 后 Ack 落 DB 为 failed 供人工排查。

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
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// KBExtractWorker 消费 knowledge.crawl.done 事件，执行入库。
type KBExtractWorker struct {
	bus          *eventbus.Client
	ingestion    *FileIngestionService
	jobRepo      *repository.CrawlJobRepository
	eventPublisher *KBEventPublisher
	maxRetry     int

	sub    *nats.Subscription
	stopCh chan struct{}
	wg     sync.WaitGroup
	running bool
	mu     sync.Mutex
}

// NewKBExtractWorker 构造 extract-worker。
func NewKBExtractWorker(
	bus *eventbus.Client,
	ingestion *FileIngestionService,
	jobRepo *repository.CrawlJobRepository,
	eventPublisher *KBEventPublisher,
	maxRetry int,
) *KBExtractWorker {
	return &KBExtractWorker{
		bus:            bus,
		ingestion:      ingestion,
		jobRepo:        jobRepo,
		eventPublisher: eventPublisher,
		maxRetry:       maxRetry,
		stopCh:         make(chan struct{}),
	}
}

// Start 订阅 knowledge.crawl.done 并启动消费循环。
func (w *KBExtractWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	if w.bus == nil {
		middleware.GetLogger().Info("kb extract worker disabled (eventbus not configured)")
		return nil
	}

	sub, err := w.bus.Subscribe("knowledge.crawl.done", "bellkeeper-extract-worker")
	if err != nil {
		return fmt.Errorf("kb extract worker subscribe: %w", err)
	}
	w.sub = sub

	w.wg.Add(1)
	go w.consumeLoop(ctx)
	middleware.GetLogger().Info("kb extract worker started")
	return nil
}

// Stop 优雅停止。
func (w *KBExtractWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()
	middleware.GetLogger().Info("kb extract worker stopped")
}

func (w *KBExtractWorker) consumeLoop(ctx context.Context) {
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
				middleware.GetLogger().Warn("kb extract worker fetch error", zap.Error(err))
				time.Sleep(time.Second)
				continue
			}
			for _, msg := range msgs {
				w.processMessage(ctx, msg)
			}
		}
	}
}

func (w *KBExtractWorker) processMessage(ctx context.Context, msg *nats.Msg) {
	ev, err := eventbus.UnmarshalEvent(msg.Data)
	if err != nil {
		middleware.GetLogger().Error("kb extract worker: unmarshal event envelope", zap.Error(err))
		_ = msg.Nak()
		return
	}
	var payload KBCrawlDonePayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		middleware.GetLogger().Error("kb extract worker: unmarshal crawl.done payload", zap.Error(err))
		_ = msg.Nak()
		return
	}

	metadata, metaErr := msg.Metadata()
	deliveryCount := 1
	if metaErr != nil {
		middleware.GetLogger().Debug("kb extract worker: no message metadata available",
			zap.Uint("job_id", payload.JobID), zap.Error(metaErr))
	} else if metadata != nil {
		deliveryCount = int(metadata.NumDelivered)
	}

	if perr := w.ingestAndFinalize(ctx, &payload); perr != nil {
		middleware.GetLogger().Error("kb extract worker: ingest failed",
			zap.Uint("job_id", payload.JobID),
			zap.String("url", payload.URL),
			zap.Error(perr))
		if deliveryCount <= w.maxRetry {
			delay := time.Duration(deliveryCount) * 10 * time.Second
			if delay > 2*time.Minute {
				delay = 2 * time.Minute
			}
			_ = msg.NakWithDelay(delay)
			return
		}
		// 超重试上限：Ack 落 DB 为 failed，避免毒丸阻塞队列。
		if err := w.jobRepo.UpdateStatus(payload.JobID, model.CrawlJobFailed, map[string]interface{}{
			"error_type":    "extract_worker_failed",
			"error_message": perr.Error(),
		}); err != nil {
			middleware.GetLogger().Error("kb extract worker: mark job failed", zap.Error(err))
		}
		_ = msg.Ack()
		return
	}

	_ = msg.Ack()
}

// ingestAndFinalize 执行入库 + 状态终结 + 发 extract.done 事件。
func (w *KBExtractWorker) ingestAndFinalize(ctx context.Context, payload *KBCrawlDonePayload) error {
	ingestResult, err := w.ingestion.IngestURL(&IngestURLRequest{
		URL:      payload.URL,
		Title:    payload.Title,
		Content:  payload.Content,
		Category: "",
		Layer:    "",
	})
	if err != nil {
		return fmt.Errorf("ingest url: %w", err)
	}

	updates := map[string]interface{}{
		"content_length": payload.ContentLength,
		"extractor_used": payload.Extractor,
	}

	var finalStatus model.CrawlJobStatus
	if ingestResult.Status == "duplicate" || ingestResult.Status == "duplicate_content" {
		finalStatus = model.CrawlJobSkipped
	} else {
		finalStatus = model.CrawlJobSuccess
	}

	if err := w.jobRepo.UpdateStatus(payload.JobID, finalStatus, updates); err != nil {
		return fmt.Errorf("update job status to %s: %w", finalStatus, err)
	}

	middleware.GetLogger().Info("kb extract worker: ingested",
		zap.Uint("job_id", payload.JobID),
		zap.String("url", payload.URL),
		zap.String("status", string(finalStatus)))

	// 入库成功 → 发 knowledge.extract.done 供 index-worker 刷新 Meili。
	if finalStatus == model.CrawlJobSuccess && w.eventPublisher != nil {
		extractPayload := KBExtractDonePayload{
			JobID:      payload.JobID,
			URL:        payload.URL,
			FilePath:   ingestResult.FilePath,
			Layer:      "", // index-worker 按文件路径定位
			DocumentID: "",
		}
		if perr := w.eventPublisher.PublishExtractDone(ctx, extractPayload); perr != nil {
			middleware.GetLogger().Warn("kb extract worker: publish extract.done failed",
				zap.Uint("job_id", payload.JobID), zap.Error(perr))
		}
	}
	return nil
}