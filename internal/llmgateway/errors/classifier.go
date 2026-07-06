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
	Unknown            Class = "unknown"
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
		if strings.Contains(lowerBody, "quota") || strings.Contains(lowerBody, "limit") {
			return Result{Class: RateLimitedRetry, BreakdownUntil: "30s", CanRetry: true}
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

// BreakdownDuration converts a human-readable breakdown hint to a concrete duration.
// If the hint starts with a number, it's parsed as seconds. Special values:
//   "permanent" → 0 (never auto-recover)
//   "long"      → 24h
//   "5h_or_7d"  → 5h (caller should use probe strategy for longer)
func BreakdownDuration(hint string) time.Duration {
	switch hint {
	case "permanent":
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
