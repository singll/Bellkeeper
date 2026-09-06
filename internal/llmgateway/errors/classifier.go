package errors

import (
	"strconv"
	"strings"
	"time"
)

// Class represents the semantic category of an upstream error.
type Class string

const (
	QuotaExhausted     Class = "quota_exhausted"
	AuthFailed         Class = "auth_failed"
	SubscriptionInvalid Class = "subscription_invalid"
	RateLimitedRetry   Class = "rate_limited_retry"
	BalanceZero        Class = "balance_zero"
	SessionExpired     Class = "session_expired"
	ServerError        Class = "server_error"
	// ContextTooLong marks "this request exceeds the member model's context
	// window" (HTTP 400 with a context-length message). It is a property of the
	// (request, model) pair — not a fault of the channel — so it must never
	// count against channel health; the router should simply pick a member
	// with a larger window.
	ContextTooLong Class = "context_too_long"
	Unknown        Class = "unknown"
)

// Result holds the classification outcome.
type Result struct {
	Class      Class
	BreakdownUntil string // human-readable duration or timestamp hint
	CanRetry   bool
}

// Classify maps status code + body + provider type to a semantic error class.
func Classify(statusCode int, body string, providerType string) Result {
	lowerBody := strings.ToLower(body)

	// 401 → auth failed (all providers)
	if statusCode == 401 {
		return Result{Class: AuthFailed, BreakdownUntil: "permanent", CanRetry: false}
	}

	// 400 + context-length message → request exceeds the model's context
	// window. Provider phrasings covered: OpenAI/SenseNova "maximum context
	// length", Anthropic "prompt is too long", OpenRouter "context_length_exceeded".
	if statusCode == 400 && isContextTooLong(lowerBody) {
		return Result{Class: ContextTooLong, BreakdownUntil: "none", CanRetry: true}
	}

	// Provider-specific classification
	switch providerType {
	case "kimi-code", "anthropic":
		return classifyKimiCode(statusCode, lowerBody)
	case "deepseek":
		return classifyDeepSeek(statusCode, lowerBody)
	case "newapi", "anyrouter":
		return classifyNewAPI(statusCode, lowerBody)
	case "moonshot":
		return classifyMoonshot(statusCode, lowerBody)
	case "aliyun":
		return classifyAliyun(statusCode, lowerBody)
	}

	// Generic fallback
	if statusCode == 429 {
		// 先判"瞬时限流"（rate limit / QPS / TPM / RPM 字样，几十秒即恢复），
		// 再判"额度已耗尽"：月额度/余额耗尽类错误（如 OpenCode Go 的 GoUsageLimitError
		// "Monthly usage limit reached. Resets in N days"）再重试也无意义，判 QuotaExhausted
		// 长熔断（24h）让渠道退出轮换——否则死的兜底渠道每 30s 复活，全池失败时把它的
		// 月额度 429 原样透传给客户端（2026-09-03 secagent 任务中断根因之一）。
		if strings.Contains(lowerBody, "rate limit") || strings.Contains(lowerBody, "rate_limit") ||
			strings.Contains(lowerBody, "qps") || strings.Contains(lowerBody, "tpm") || strings.Contains(lowerBody, "rpm") {
			return Result{Class: RateLimitedRetry, BreakdownUntil: "30s", CanRetry: true}
		}
		if strings.Contains(lowerBody, "monthly usage") || strings.Contains(lowerBody, "resets in") ||
			strings.Contains(lowerBody, "gousagelimit") || strings.Contains(lowerBody, "usage limit") ||
			strings.Contains(lowerBody, "insufficient") || strings.Contains(lowerBody, "exhausted") {
			return Result{Class: QuotaExhausted, BreakdownUntil: "long", CanRetry: false}
		}
		return Result{Class: RateLimitedRetry, BreakdownUntil: "30s", CanRetry: true}
	}
	if statusCode >= 500 {
		return Result{Class: ServerError, BreakdownUntil: "60s", CanRetry: true}
	}
	if statusCode == 403 {
		if strings.Contains(lowerBody, "quota") || strings.Contains(lowerBody, "limit") {
			return Result{Class: QuotaExhausted, BreakdownUntil: "long", CanRetry: false}
		}
		return Result{Class: AuthFailed, BreakdownUntil: "permanent", CanRetry: false}
	}

	return Result{Class: Unknown, BreakdownUntil: "30s", CanRetry: true}
}

func classifyKimiCode(statusCode int, body string) Result {
	switch statusCode {
	case 401:
		return Result{Class: AuthFailed, BreakdownUntil: "permanent", CanRetry: false}
	case 402:
		return Result{Class: SubscriptionInvalid, BreakdownUntil: "1h", CanRetry: true}
	case 403:
		if strings.Contains(body, "usage limit") || strings.Contains(body, "quota") {
			return Result{Class: QuotaExhausted, BreakdownUntil: "5h_or_7d", CanRetry: false}
		}
		return Result{Class: QuotaExhausted, BreakdownUntil: "5h", CanRetry: false}
	case 429:
		if strings.Contains(body, "engine overload") {
			return Result{Class: RateLimitedRetry, BreakdownUntil: "30s", CanRetry: true}
		}
		if strings.Contains(body, "quota") {
			return Result{Class: QuotaExhausted, BreakdownUntil: "5h_or_7d", CanRetry: false}
		}
		return Result{Class: RateLimitedRetry, BreakdownUntil: "30s", CanRetry: true}
	case 500, 502, 503:
		return Result{Class: ServerError, BreakdownUntil: "60s", CanRetry: true}
	}
	return Result{Class: Unknown, BreakdownUntil: "30s", CanRetry: true}
}

func classifyDeepSeek(statusCode int, body string) Result {
	if statusCode == 429 || statusCode == 403 {
		if strings.Contains(body, "insufficient quota") || strings.Contains(body, "balance") {
			return Result{Class: BalanceZero, BreakdownUntil: "long", CanRetry: false}
		}
	}
	if statusCode >= 500 {
		return Result{Class: ServerError, BreakdownUntil: "60s", CanRetry: true}
	}
	return Classify(statusCode, body, "")
}

func classifyNewAPI(statusCode int, body string) Result {
	if statusCode == 401 {
		return Result{Class: SessionExpired, BreakdownUntil: "long", CanRetry: false}
	}
	if statusCode == 429 {
		return Result{Class: RateLimitedRetry, BreakdownUntil: "60s", CanRetry: true}
	}
	if statusCode >= 500 {
		return Result{Class: ServerError, BreakdownUntil: "60s", CanRetry: true}
	}
	return Classify(statusCode, body, "")
}

func classifyMoonshot(statusCode int, body string) Result {
	if statusCode == 429 || statusCode == 403 {
		if strings.Contains(body, "quota") || strings.Contains(body, "balance") {
			return Result{Class: BalanceZero, BreakdownUntil: "long", CanRetry: false}
		}
	}
	return Classify(statusCode, body, "")
}

func classifyAliyun(statusCode int, body string) Result {
	if statusCode >= 500 {
		return Result{Class: ServerError, BreakdownUntil: "120s", CanRetry: true}
	}
	return Classify(statusCode, body, "")
}

// isContextTooLong matches the common context-window-exceeded phrasings across
// providers (matched against a lowercased body).
func isContextTooLong(lowerBody string) bool {
	return strings.Contains(lowerBody, "maximum context length") ||
		strings.Contains(lowerBody, "context_length_exceeded") ||
		strings.Contains(lowerBody, "prompt is too long") ||
		strings.Contains(lowerBody, "exceeds the context") ||
		strings.Contains(lowerBody, "context window")
}

// BreakdownDuration converts a human-readable breakdown hint to a concrete duration.
// If the hint starts with a number, it's parsed as seconds. Special values:
//   "permanent" → 0 (never auto-recover)
//   "long"      → 24h
//   "5h_or_7d"  → 5h (caller should use probe strategy for longer)
//   "none"      → 0 (no breakdown at all — not a channel fault)
func BreakdownDuration(hint string) time.Duration {
	switch hint {
	case "permanent":
		return 0
	case "none":
		return 0
	case "long":
		return 24 * time.Hour
	case "5h_or_7d":
		return 5 * time.Hour
	case "1h":
		return time.Hour
	case "5h":
		return 5 * time.Hour
	case "60s":
		return 60 * time.Second
	case "120s":
		return 120 * time.Second
	case "30s":
		return 30 * time.Second
	default:
		// Try to parse as seconds
		if s, err := strconv.Atoi(hint); err == nil {
			return time.Duration(s) * time.Second
		}
		return 60 * time.Second
	}
}
