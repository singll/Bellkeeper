// kb_health_worker.go 实现 knowledge.crawl.failed 事件的消费者（域名健康度 worker）。
//
// 对应《Bellkeeper 1.0 重构与架构演进规划》§2.1.1：
//   crawl 失败 → 发 knowledge.crawl.failed → 本 worker 消费 →
//   EvaluateDomainHealth（ConsecutiveFailures≥阈值暂停、HealthScore≥阈值恢复）→
//   暂停/恢复时发 system.health.alert（经 NotificationService 投递 Matrix alerts 频道）。
//
// 域名健康度状态由 crawl_domain_profiles 表持久化，DequeueFair 出队时过滤 is_paused
// 域名，避免持续浪费 worker。

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/eventbus"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// KBHealthWorker 消费 knowledge.crawl.failed 事件，评估域名健康度并自动暂停/恢复。
type KBHealthWorker struct {
	bus        *eventbus.Client
	domainRepo *repository.CrawlDomainProfileRepository
	notify     *NotificationService
	cfg        config.CrawlQueueConfig

	sub    *nats.Subscription
	stopCh chan struct{}
	wg     sync.WaitGroup
	running bool
	mu     sync.Mutex
}

// NewKBHealthWorker 构造域名健康度 worker。
func NewKBHealthWorker(
	bus *eventbus.Client,
	domainRepo *repository.CrawlDomainProfileRepository,
	notify *NotificationService,
	cfg config.CrawlQueueConfig,
) *KBHealthWorker {
	return &KBHealthWorker{
		bus:        bus,
		domainRepo: domainRepo,
		notify:     notify,
		cfg:        cfg,
		stopCh:     make(chan struct{}),
	}
}

// Start 订阅 knowledge.crawl.failed 并启动消费循环。
func (w *KBHealthWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	if w.bus == nil {
		middleware.GetLogger().Info("kb health worker disabled (eventbus not configured)")
		return nil
	}

	sub, err := w.bus.Subscribe("knowledge.crawl.failed", "bellkeeper-domain-health-worker")
	if err != nil {
		return fmt.Errorf("kb health worker subscribe: %w", err)
	}
	w.sub = sub

	w.wg.Add(1)
	go w.consumeLoop(ctx)
	middleware.GetLogger().Info("kb health worker started")
	return nil
}

// Stop 优雅停止。
func (w *KBHealthWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()
	middleware.GetLogger().Info("kb health worker stopped")
}

func (w *KBHealthWorker) consumeLoop(ctx context.Context) {
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
				middleware.GetLogger().Warn("kb health worker fetch error", zap.Error(err))
				time.Sleep(time.Second)
				continue
			}
			for _, msg := range msgs {
				w.processMessage(ctx, msg)
			}
		}
	}
}

func (w *KBHealthWorker) processMessage(ctx context.Context, msg *nats.Msg) {
	ev, err := eventbus.UnmarshalEvent(msg.Data)
	if err != nil {
		middleware.GetLogger().Error("kb health worker: unmarshal event envelope", zap.Error(err))
		_ = msg.Nak()
		return
	}
	var payload KBCrawlFailedPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		middleware.GetLogger().Error("kb health worker: unmarshal crawl.failed payload", zap.Error(err))
		_ = msg.Nak()
		return
	}

	if payload.SourceDomain == "" || w.domainRepo == nil {
		_ = msg.Ack()
		return
	}

	// 评估健康度并按阈值自动暂停/恢复。
	action, err := w.domainRepo.EvaluateDomainHealth(payload.SourceDomain, w.cfg.DomainPauseThreshold, w.cfg.DomainResumeThreshold)
	if err != nil {
		middleware.GetLogger().Error("kb health worker: evaluate domain health",
			zap.String("domain", payload.SourceDomain), zap.Error(err))
		_ = msg.NakWithDelay(30 * time.Second)
		return
	}

	if action == "paused" || action == "resumed" {
		w.sendHealthAlert(ctx, payload.SourceDomain, action, payload)
	}

	_ = msg.Ack()
}

// sendHealthAlert 在域名暂停/恢复时发 system.health.alert（经 NotificationService 投递 Matrix）。
func (w *KBHealthWorker) sendHealthAlert(ctx context.Context, domain, action string, payload KBCrawlFailedPayload) {
	if w.notify == nil {
		return
	}
	var msg string
	severity := "warning"
	switch action {
	case "paused":
		msg = fmt.Sprintf("🔶 域名已自动暂停：%s\n连续失败触发阈值（job=%d, err=%s）\nDequeueFair 将跳过此域名直至 HealthScore 恢复。",
			domain, payload.JobID, payload.ErrorType)
	case "resumed":
		msg = fmt.Sprintf("✅ 域名已自动恢复：%s\nHealthScore 已回升至阈值，恢复出队。", domain)
		severity = "info"
	}
	_, err := w.notify.Send(ctx, &NotificationRequest{
		Channel:  "alerts",
		Message:  msg,
		Severity: severity,
		DedupKey: fmt.Sprintf("domain-health:%s:%s", domain, action),
	})
	if err != nil {
		middleware.GetLogger().Warn("kb health worker: send alert failed",
			zap.String("domain", domain), zap.String("action", action), zap.Error(err))
	}
}