// kb_events.go 定义知识库事件总线的 payload 契约与发布辅助。
//
// 对应《Bellkeeper 1.0 重构与架构演进规划》§1.2.1 knowledge stream：
//   - knowledge.crawl.done   : 爬取+提取完成 → extract-worker 入库
//   - knowledge.extract.done : 入库完成 → index-worker 刷新 Meili
//   - knowledge.crawl.failed : 爬取失败 → 域名健康度评估（后续任务）
//
// DB 表 crawl_jobs.status 保留为状态真相源，NATS 消息携带 job_id + content，
// 消费者更新状态。至少一次投递 + AckExplicit + NakWithDelay 重试。

package service

import (
	"context"
	"fmt"

	"github.com/singll/bellkeeper/internal/eventbus"
)

// KBCrawlDonePayload 是 knowledge.crawl.done 事件的 payload。
//
// Content 直接随消息投递（多数文章 < 128KB，JetStream 默认 max payload 1MB 足够）；
// 发布前由调用方按 maxCrawlEventBytes 截断保护，超限走 fallback 同步入库。
type KBCrawlDonePayload struct {
	JobID         uint   `json:"job_id"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	SourceID      uint   `json:"source_id"`
	SourceDomain  string `json:"source_domain"`
	Content       string `json:"content"`
	Extractor     string `json:"extractor"`
	ContentLength int    `json:"content_length"`
}

// KBExtractDonePayload 是 knowledge.extract.done 事件的 payload。
type KBExtractDonePayload struct {
	JobID      uint   `json:"job_id"`
	URL        string `json:"url"`
	FilePath   string `json:"file_path"`
	Layer      string `json:"layer"`
	DocumentID string `json:"document_id"`
}

// KBCrawlFailedPayload 是 knowledge.crawl.failed 事件的 payload（1.0 §2.1.1）。
// 供域名健康度 worker 消费 → EvaluateDomainHealth 自动暂停/恢复 + 发 system.health.alert。
type KBCrawlFailedPayload struct {
	JobID        uint   `json:"job_id"`
	URL          string `json:"url"`
	SourceDomain string `json:"source_domain"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
	RetryCount   int    `json:"retry_count"`
	Terminal     bool   `json:"terminal"` // true=已终结（skipped/dead），false=将重试
}

// KBEventPublisher 封装知识库事件的发布，供 CrawlQueueService 调用。
type KBEventPublisher struct {
	bus *eventbus.Client
}

// NewKBEventPublisher 构造发布器。bus 可为 nil（未启用 eventbus 时降级为同步）。
func NewKBEventPublisher(bus *eventbus.Client) *KBEventPublisher {
	return &KBEventPublisher{bus: bus}
}

// maxCrawlEventBytes 限制 crawl.done 事件 payload 大小，避免超 NATS max payload。
// 超限的内容由调用方走 fallback 同步入库。
const maxCrawlEventBytes = 512 * 1024 // 512KB

// PublishCrawlDone 发布 knowledge.crawl.done 事件。
// 返回 (published, error)：published=false 表示超限或 bus 未配置，调用方应走 fallback。
func (p *KBEventPublisher) PublishCrawlDone(ctx context.Context, payload KBCrawlDonePayload) (bool, error) {
	if p.bus == nil {
		return false, nil
	}
	if len(payload.Content) > maxCrawlEventBytes {
		return false, nil
	}
	ev, err := eventbus.New(ctx, "knowledge.crawl.done", eventbus.SourceKB, fmt.Sprintf("crawl:%d", payload.JobID), payload)
	if err != nil {
		return false, err
	}
	if err := p.bus.PublishEvent(ctx, ev); err != nil {
		return false, err
	}
	return true, nil
}

// PublishExtractDone 发布 knowledge.extract.done 事件。
func (p *KBEventPublisher) PublishExtractDone(ctx context.Context, payload KBExtractDonePayload) error {
	if p.bus == nil {
		return nil
	}
	ev, err := eventbus.New(ctx, "knowledge.extract.done", eventbus.SourceKB, fmt.Sprintf("extract:%d", payload.JobID), payload)
	if err != nil {
		return err
	}
	return p.bus.PublishEvent(ctx, ev)
}

// PublishCrawlFailed 发布 knowledge.crawl.failed 事件（1.0 §2.1.1 域名健康度）。
func (p *KBEventPublisher) PublishCrawlFailed(ctx context.Context, payload KBCrawlFailedPayload) error {
	if p.bus == nil {
		return nil
	}
	ev, err := eventbus.New(ctx, "knowledge.crawl.failed", eventbus.SourceKB, fmt.Sprintf("crawlfail:%s:%d", payload.SourceDomain, payload.JobID), payload)
	if err != nil {
		return err
	}
	return p.bus.PublishEvent(ctx, ev)
}