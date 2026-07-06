// event.go 定义 Bellkeeper 1.0 事件总线的统一 Event envelope 契约。
//
// 所有发布到 JetStream 的业务事件均使用此 envelope 包装，保证跨模块可追溯：
//   - EventID：ULID，时间有序，天然可作为幂等键与排序依据。
//   - Type：事件类型，形如 "knowledge.crawl.done"（对应 stream subject 层级）。
//   - Source：来源模块标识（kb/llm/matrix/system/logs/governance）。
//   - TraceID：贯穿日志链路（见规划 §4.4 trace_id 全链路），供 Loki/Grafana 关联查询。
//
// 见《Bellkeeper 1.0 重构与架构演进规划》§1.2.2。

package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/singll/bellkeeper/internal/middleware"
)

// Source 取值枚举：标识事件来源模块。新模块需在此扩展。
const (
	SourceKB         = "kb"
	SourceLLM        = "llm"
	SourceMatrix     = "matrix"
	SourceSystem     = "system"
	SourceLogs       = "logs"
	SourceGovernance = "governance"
)

// Event 是所有事件总线消息的统一信封。
//
// 字段对应规划 §1.2.2；Payload 用 json.RawMessage 延迟解码，
// 消费者只需按 Type 解析自身关心的 payload 结构。
type Event struct {
	EventID    string          `json:"event_id"`           // ULID，全局唯一且时间有序
	Type       string          `json:"type"`               // 如 "knowledge.crawl.done"
	Source     string          `json:"source"`             // kb/llm/matrix/system/logs/governance
	OccurredAt time.Time       `json:"occurred_at"`        // 事件发生时刻（UTC）
	Subject    string          `json:"subject"`            // 业务主键（如 job_id / room_id）
	Payload    json.RawMessage `json:"payload"`            // 类型特定的 JSON 载荷
	TraceID    string          `json:"trace_id,omitempty"` // 贯穿日志链路（§4.4），可空
}

// New 构造一个新 Event：生成 ULID、记录发生时刻，Payload 由调用方提供。
//
// TraceID 从 ctx 提取（若中间件已注入）；ctx 无 trace_id 时留空，
// 由发布方按需显式设置 SetTraceID。
func New(ctx context.Context, typ, source, subject string, payload any) (*Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("eventbus: marshal payload for %s: %w", typ, err)
	}
	return &Event{
		EventID:    ulid.Make().String(),
		Type:       typ,
		Source:     source,
		OccurredAt: time.Now().UTC(),
		Subject:    subject,
		Payload:    raw,
		TraceID:    traceIDFromContext(ctx),
	}, nil
}

// SetTraceID 显式覆盖 TraceID（用于调用方自有 trace 体系时强制对齐）。
func (e *Event) SetTraceID(id string) { e.TraceID = id }

// Marshal 返回 Event 的 JSON 字节流，供 Publish 使用。
func (e *Event) Marshal() ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("eventbus: marshal event %s: %w", e.EventID, err)
	}
	return b, nil
}

// UnmarshalEvent 从字节流解析为 Event（消费者侧使用）。
func UnmarshalEvent(data []byte) (*Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("eventbus: unmarshal event: %w", err)
	}
	return &e, nil
}

// SubjectFor 返回某 type 在 JetStream 中应发布的完整 subject。
//
// 约定：event.Type 形如 "<stream-prefix>.<verb>.<state>"，
// 完整 subject 即 Type 本身（如 "knowledge.crawl.done"）。
// 调用方据此用 client.Publish(Type, data) 发布，或经 PublishEvent 自动路由。
//
// 该辅助函数保留显式映射意图，便于未来在 Type→subject 规则复杂化时集中收口。
func SubjectFor(typ string) string { return typ }

// traceIDFromContext 从 ctx 取 trace_id（1.0 §4.4 全链路）。
// 中间件 middleware.TraceID 已将 trace_id 注入 context.Context，此处取出贯穿事件链。
func traceIDFromContext(ctx context.Context) string {
	return middleware.TraceIDFromContext(ctx)
}
