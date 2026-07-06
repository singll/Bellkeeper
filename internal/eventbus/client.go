// Package eventbus 提供 Bellkeeper 1.0 的统一 NATS JetStream 事件总线。
//
// 该包从 internal/matrix/infra/nats.go 提升而来，作为一级共享基础设施，
// 供 KB / LLM / Matrix / 日志 / 治理中台等所有模块复用。
//
// 设计要点（见《Bellkeeper 1.0 重构与架构演进规划》§1.2）：
//   - 删除僵尸 commands stream（全仓无任何 Publish/Subscribe）。
//   - 按 §1.2.1 表扩展 notifications/knowledge/llm/matrix/system/logs 六条 stream。
//   - 每条 stream 用 FileStorage + AckExplicit，至少一次投递。
//   - 消费者统一用持久 Pull 模式（PullSubscribe + Fetch）。
//   - 失败重试走 NakWithDelay 退避（由调用方决定，本包仅提供能力）。
//
// 禁止：任何模块绕过本包直接 nats.Connect。
package eventbus

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// Client 包装 NATS 连接与 JetStream 上下文，管理多 stream 生命周期。
type Client struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	streams config.NATSStreamsConfig
}

// NewClient 创建并初始化一个 NATS JetStream 客户端，确保所有 stream 存在。
func NewClient(cfg config.NATSConfig) (*Client, error) {
	opts := []nats.Option{
		nats.Name("bellkeeper-eventbus"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // 无限重连
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				middleware.GetLogger().Warn("eventbus NATS disconnected", zap.Error(err))
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			middleware.GetLogger().Info("eventbus NATS reconnected")
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			middleware.GetLogger().Warn("eventbus NATS connection closed")
		}),
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("eventbus: connect to NATS %s: %w", cfg.URL, err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("eventbus: create JetStream context: %w", err)
	}

	c := &Client{
		conn:    conn,
		js:      js,
		streams: cfg.Streams,
	}

	if err := c.ensureStreams(); err != nil {
		conn.Close()
		return nil, err
	}

	return c, nil
}

// streamSpec 描述单条 JetStream stream 的创建参数。
// 对应规划 §1.2.1 的 stream 划分表。
type streamSpec struct {
	name      string
	subjects  []string
	retention nats.RetentionPolicy
	maxAge    time.Duration
}

// streamSpecs 按配置生成全部 stream 规格。
//
// 注意：
//   - commands stream 已删除（1.0 前为僵尸配置，全仓无 Publish/Subscribe）。
//   - notifications/knowledge/llm/matrix 为 WorkQueue（点对点消费，每消息一个消费者）。
//   - system 为 Interest（多消费者广播：日报/告警/Matrix 各自独立消费）。
//   - logs 为 Limits + 短保留（供 Loki 外挂消费，7 天后自动清理）。
func (c *Client) streamSpecs() []streamSpec {
	s := c.streams
	return []streamSpec{
		{
			name:      s.Notifications,
			subjects:  []string{s.Notifications + ".>"},
			retention: nats.WorkQueuePolicy,
			maxAge:    72 * time.Hour,
		},
		{
			name:      s.Knowledge,
			subjects:  []string{s.Knowledge + ".>"},
			retention: nats.WorkQueuePolicy,
			maxAge:    24 * time.Hour,
		},
		{
			name:      s.LLM,
			subjects:  []string{s.LLM + ".>"},
			retention: nats.WorkQueuePolicy,
			maxAge:    24 * time.Hour,
		},
		{
			name:      s.Matrix,
			subjects:  []string{s.Matrix + ".>"},
			retention: nats.WorkQueuePolicy,
			maxAge:    24 * time.Hour,
		},
		{
			name:      s.System,
			subjects:  []string{s.System + ".>"},
			retention: nats.InterestPolicy,
			maxAge:    72 * time.Hour,
		},
		{
			name:      s.Logs,
			subjects:  []string{s.Logs + ".>"},
			retention: nats.LimitsPolicy,
			maxAge:    7 * 24 * time.Hour,
		},
	}
}

// ensureStreams 幂等创建所有配置的 JetStream stream。
//
// 已存在的 stream 不重建（保留其历史消息与消费者绑定）；
// 仅在 ErrStreamNotFound 时 AddStream。若已存在 stream 的 subjects 与当前配置不符，
// 需运维手动处理（避免误删生产消息），本方法只保证"存在即放过"。
func (c *Client) ensureStreams() error {
	for _, spec := range c.streamSpecs() {
		if spec.name == "" {
			continue
		}
		if _, err := c.js.StreamInfo(spec.name); err == nil {
			continue
		} else if err != nats.ErrStreamNotFound {
			return fmt.Errorf("eventbus: get stream info for %s: %w", spec.name, err)
		}

		if _, err := c.js.AddStream(&nats.StreamConfig{
			Name:      spec.name,
			Subjects:  spec.subjects,
			Retention: spec.retention,
			MaxAge:    spec.maxAge,
			Storage:   nats.FileStorage,
		}); err != nil {
			return fmt.Errorf("eventbus: create stream %s: %w", spec.name, err)
		}
		middleware.GetLogger().Info("eventbus: created NATS stream",
			zap.String("name", spec.name),
			zap.Strings("subjects", spec.subjects),
			zap.String("retention", retentionName(spec.retention)),
			zap.Duration("max_age", spec.maxAge))
	}
	return nil
}

func retentionName(r nats.RetentionPolicy) string {
	switch r {
	case nats.WorkQueuePolicy:
		return "WorkQueue"
	case nats.InterestPolicy:
		return "Interest"
	case nats.LimitsPolicy:
		return "Limits"
	default:
		return "Unknown"
	}
}

// Publish 发布一条消息到指定 subject（JetStream 至少一次投递）。
func (c *Client) Publish(subject string, data []byte) error {
	if _, err := c.js.Publish(subject, data); err != nil {
		return fmt.Errorf("eventbus: publish to %s: %w", subject, err)
	}
	return nil
}

// PublishEvent 发布一个 Event envelope：按 e.Type 作为 subject 路由，
// 自动序列化后投递到对应 stream（stream subjects 形如 "<prefix>.>"）。
//
// 调用方无需关心 subject 拼接，统一用 New(...) 构造 + PublishEvent 发布。
func (c *Client) PublishEvent(ctx context.Context, e *Event) error {
	if e == nil {
		return fmt.Errorf("eventbus: publish nil event")
	}
	if e.EventID == "" {
		return fmt.Errorf("eventbus: event missing EventID (type=%s)", e.Type)
	}
	if e.TraceID == "" {
		e.TraceID = traceIDFromContext(ctx)
	}
	data, err := e.Marshal()
	if err != nil {
		return err
	}
	subject := SubjectFor(e.Type)
	if err := c.Publish(subject, data); err != nil {
		return err
	}
	middleware.GetLogger().Debug("eventbus: published event",
		zap.String("event_id", e.EventID),
		zap.String("type", e.Type),
		zap.String("source", e.Source),
		zap.String("subject", subject),
		zap.String("trace_id", e.TraceID))
	return nil
}

// Subscribe 创建一个持久 Pull 订阅（AckExplicit）。
// durableName 用于标识消费者，重启后可恢复未 ack 的消息。
func (c *Client) Subscribe(subject, durableName string) (*nats.Subscription, error) {
	sub, err := c.js.PullSubscribe(subject, durableName, nats.AckExplicit())
	if err != nil {
		return nil, fmt.Errorf("eventbus: pull subscribe to %s: %w", subject, err)
	}
	return sub, nil
}

// Close 优雅关闭连接（Drain 等待 in-flight 消息处理完毕）。
func (c *Client) Close() {
	if err := c.conn.Drain(); err != nil {
		middleware.GetLogger().Warn("eventbus: failed to drain connection", zap.Error(err))
	}
}

// JetStream 返回底层 JetStream 上下文，供高级操作使用。
func (c *Client) JetStream() nats.JetStreamContext {
	return c.js
}

// StreamsConfig 返回当前配置的 stream 名称集合。
func (c *Client) StreamsConfig() config.NATSStreamsConfig {
	return c.streams
}
