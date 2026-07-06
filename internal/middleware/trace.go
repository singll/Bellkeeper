// trace.go 实现 HTTP 请求 trace_id 全链路注入（1.0 §4.4 trace_id 全链路）。
//
// TraceID() 中间件：
//   - 优先复用上游 X-Trace-Id header（支持跨服务链路传播）
//   - 否则生成 ULID 作为新 trace_id
//   - 注入 gin.Context（供 handler/service 经 TraceIDFromContext 取用）
//   - 回写 response header X-Trace-Id（供客户端/Loki 关联）
//   - 注入 context.Context（context.WithValue），供经 c.Request.Context() 派生的
//     service 调用链取用
//
// TraceIDFromContext 从 context.Context 或 gin.Context 提取 trace_id。
// eventbus.New / log_center.LogActivity 等经此接入全链路。

package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TraceIDContextKey 是 context.Context 中 trace_id 的 key 类型（不导出，防冲突）。
type TraceIDContextKey struct{}

// traceIDHeader 是 trace_id 的 HTTP header 名。
const traceIDHeader = "X-Trace-Id"

// TraceID 返回 gin 中间件：为每个请求注入 trace_id。
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 优先复用上游 trace_id（跨服务链路传播）
		tid := c.GetHeader(traceIDHeader)
		if tid == "" {
			tid = newTraceID()
		}
		// 注入 gin.Context（handler/service 经 c.Get("trace_id") 取用）
		c.Set("trace_id", tid)
		// 注入 context.Context（经 c.Request.Context() 派生的调用链取用）
		ctx := context.WithValue(c.Request.Context(), TraceIDContextKey{}, tid)
		c.Request = c.Request.WithContext(ctx)
		// 回写 response header
		c.Header(traceIDHeader, tid)
		c.Next()
	}
}

// newTraceID 生成新 trace_id（UUID v4，无连字符，紧凑）。
// 用 UUID 而非 ULID 以避免 eventbus 包对 ulid 的重复依赖；时间有序性由日志时序保证。
func newTraceID() string {
	return uuid.NewString()
}

// TraceIDFromContext 从 context.Context 提取 trace_id（支持标准 context 与 gin.Context）。
// 无 trace_id 时返回空串。
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	// 标准 context.Context 路径
	if v, ok := ctx.Value(TraceIDContextKey{}).(string); ok {
		return v
	}
	// gin.Context 路径（c.Request.Context() 已含 trace_id，但兼容直接传 gin.Context 的场景）
	type traceGetter interface {
		Get(string) (interface{}, bool)
	}
	if g, ok := ctx.(traceGetter); ok {
		if v, exists := g.Get("trace_id"); exists {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}