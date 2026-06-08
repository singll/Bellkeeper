package llmclient

import (
	"errors"
	"strings"
	"time"
)

type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func IsRetryable(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case 408, 425, 429, 500, 502, 503, 504:
			return true
		}
		body := strings.ToLower(httpErr.Body)
		return strings.Contains(body, "rate limit") ||
			strings.Contains(body, "quota") ||
			strings.Contains(body, "exhausted")
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "quota") ||
		strings.Contains(msg, "exhausted") ||
		strings.Contains(msg, "temporarily unavailable")
}

func ErrorClass(err error) string {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case 429:
			return "rate_limited"
		case 503:
			return "unavailable"
		case 500, 502, 504:
			return "upstream_error"
		case 408, 425:
			return "retryable_http"
		default:
			return "http_error"
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "quota"):
		return "quota"
	case strings.Contains(msg, "rate limit"):
		return "rate_limited"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timeout"
	default:
		return "error"
	}
}

func RetryDelay(err error, attempt int, initialBackoff, maxBackoff time.Duration) (time.Duration, bool) {
	if !IsRetryable(err) {
		return 0, false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr.RetryAfter > 0 {
		if httpErr.RetryAfter > maxBackoff {
			return maxBackoff, true
		}
		return httpErr.RetryAfter, true
	}
	wait := initialBackoff
	for i := 1; i < attempt; i++ {
		wait *= 2
		if wait >= maxBackoff {
			return maxBackoff, true
		}
	}
	if wait > maxBackoff {
		wait = maxBackoff
	}
	return wait, true
}
