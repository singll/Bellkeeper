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

	sub     *nats.Subscription
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running bool
	mu      sync.Mutex
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
	w.wg.Add(1)
	go w.maintenanceLoop(ctx) // half-open 恢复 + 长期不可用升级告警
	middleware.GetLogger().Info("kb health worker started")
	return nil
}

// maintenanceLoop 周期性维护暂停域名：
//  1. half-open 探测恢复：暂停时长超过冷静期的域名解除暂停、给一次重试窗口
//     （补齐事件驱动恢复对已暂停域名失效的盲区）。
//  2. 长期不可用升级：暂停超 24h 且健康度探底的域名，每轮升级告警一次（人工介入）。
func (w *KBHealthWorker) maintenanceLoop(ctx context.Context) {
	defer w.wg.Done()
	// 探测间隔取冷静期的一半，保证冷静期到点后较快放行；下限 5 分钟避免空转。
	cooldown := w.pauseCooldown()
	interval := cooldown / 2
	if interval < 5*time.Minute {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runMaintenance(ctx)
		}
	}
}

// pauseCooldown 返回暂停 half-open 冷静期：优先取配置，缺省 30 分钟。
func (w *KBHealthWorker) pauseCooldown() time.Duration {
	if w.cfg.DomainPauseCooldownMinutes > 0 {
		return time.Duration(w.cfg.DomainPauseCooldownMinutes) * time.Minute
	}
	return 30 * time.Minute
}

func (w *KBHealthWorker) runMaintenance(ctx context.Context) {
	if w.domainRepo == nil {
		return
	}
	// 1. half-open 恢复
	cutoff := time.Now().Add(-w.pauseCooldown())
	recovered, err := w.domainRepo.HalfOpenRecoverDomains(cutoff)
	if err != nil {
		middleware.GetLogger().Warn("kb health worker: half-open recover failed", zap.Error(err))
	} else {
		for _, d := range recovered {
			w.sendResumedAlert(ctx, d)
		}
		if len(recovered) > 0 {
			middleware.GetLogger().Info("kb health worker: half-open recovered domains", zap.Int("count", len(recovered)))
		}
	}
	// 2. 长期不可用升级告警（暂停超 24h 且健康度探底）
	longCutoff := time.Now().Add(-24 * time.Hour)
	stuck, err := w.domainRepo.FindLongUnavailableDomains(longCutoff)
	if err != nil {
		middleware.GetLogger().Warn("kb health worker: long-unavailable scan failed", zap.Error(err))
		return
	}
	for i := range stuck {
		w.sendLongUnavailableAlert(ctx, stuck[i].Domain, stuck[i].PausedAt)
	}
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
	res, err := w.domainRepo.EvaluateDomainHealth(payload.SourceDomain, w.cfg.DomainPauseThreshold, w.cfg.DomainResumeThreshold)
	if err != nil {
		middleware.GetLogger().Error("kb health worker: evaluate domain health",
			zap.String("domain", payload.SourceDomain), zap.Error(err))
		_ = msg.NakWithDelay(30 * time.Second)
		return
	}

	switch res.Action {
	case "paused":
		w.onPaused(ctx, payload.SourceDomain, res, payload)
	}
	// 恢复不在事件路径：暂停域名不再出队，无 crawl.failed；恢复由 maintenanceLoop
	// 的 half-open 时间驱动扫描处理（sendResumedAlert）。

	_ = msg.Ack()
}

// onPaused 按可用性语义决定「暂停」事件是否推送人工房间：
//   - HealthScore 已探底（<=0）：域名事实上不可用 → 推送 🔴 warning（边缘触发一次）。
//   - HealthScore 仍有余量：只是短暂/波动性暂停 → 静默（仅 DB 审计 + debug 日志），
//     避免 123 个好域名的瞬时抖动刷屏。若持续恶化到探底，会再走上面的分支告警一次。
func (w *KBHealthWorker) onPaused(ctx context.Context, domain string, res repository.DomainHealthResult, payload KBCrawlFailedPayload) {
	if res.HealthScore > 0 {
		middleware.GetLogger().Debug("domain paused (transient, suppressed)",
			zap.String("domain", domain),
			zap.Int("health_score", res.HealthScore),
			zap.Int("consecutive_failures", res.ConsecutiveFailures),
			zap.Uint64("job", uint64(payload.JobID)),
			zap.String("err", payload.ErrorType))
		return
	}
	w.sendUnavailableAlert(ctx, domain, payload)
}

// sendUnavailableAlert 域名事实不可用告警（HealthScore 探底）。dedup_key 含域名以
// 保证同一域名边缘触发只发一次；文案保留原有 emoji/排版风格。
func (w *KBHealthWorker) sendUnavailableAlert(ctx context.Context, domain string, payload KBCrawlFailedPayload) {
	if w.notify == nil {
		return
	}
	msg := fmt.Sprintf("🔴 域名疑似不可用：%s\n连续域名级失败已致 HealthScore 探底（job=%d, err=%s）。\nDequeueFair 已跳过此域名；将在冷静期后自动 half-open 探测恢复。",
		domain, payload.JobID, payload.ErrorType)
	w.deliver(ctx, "warning", msg, fmt.Sprintf("domain-health:unavailable:%s", domain), domain, "unavailable")
}

// sendResumedAlert 域名恢复出队通知（聚合，低优先级）。
func (w *KBHealthWorker) sendResumedAlert(ctx context.Context, domain string) {
	if w.notify == nil {
		return
	}
	msg := fmt.Sprintf("✅ 域名已恢复出队：%s\nHealthScore 回升/冷静期结束，重新纳入调度。", domain)
	// dedup_key 不含域名 → 同窗口多个恢复聚合成一条摘要，不逐条刷屏。
	w.deliver(ctx, "info", msg, "domain-health:resumed", domain, "resumed")
}

// sendLongUnavailableAlert 长期不可用/疑似失效告警（每日一次升级提醒，需人工介入）。
func (w *KBHealthWorker) sendLongUnavailableAlert(ctx context.Context, domain string, pausedAt *time.Time) {
	if w.notify == nil {
		return
	}
	dur := "长时间"
	if pausedAt != nil {
		dur = time.Since(*pausedAt).Round(time.Hour).String()
	}
	msg := fmt.Sprintf("🛑 域名长期不可用：%s\n已暂停 %s 且 HealthScore 持续探底，可能已失效，请人工核查是否停用该源。",
		domain, dur)
	w.deliver(ctx, "warning", msg, fmt.Sprintf("domain-health:long-unavailable:%s", domain), domain, "long-unavailable")
}

// deliver 统一投递并记录发送结果（失败仅告警级日志，不吞噬）。
func (w *KBHealthWorker) deliver(ctx context.Context, severity, msg, dedupKey, domain, kind string) {
	_, err := w.notify.Send(ctx, &NotificationRequest{
		Channel:  "alerts",
		Message:  msg,
		Severity: severity,
		DedupKey: dedupKey,
	})
	if err != nil {
		middleware.GetLogger().Warn("kb health worker: send alert failed",
			zap.String("domain", domain), zap.String("kind", kind), zap.Error(err))
	}
}
