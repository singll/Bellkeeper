package pkb

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func (c *Curator) chatCompletionWithRetry(model, systemPrompt, userPrompt string, temperature float64, taskType string) (string, error) {
	retry := c.domains.Defaults.Retry
	maxAttempts := retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	initialBackoff := time.Duration(retry.InitialBackoffSeconds) * time.Second
	if initialBackoff <= 0 {
		initialBackoff = 20 * time.Second
	}
	maxBackoff := time.Duration(retry.MaxBackoffSeconds) * time.Second
	if maxBackoff <= 0 {
		maxBackoff = 300 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out, err := c.client.ChatCompletion(model, systemPrompt, userPrompt, temperature, taskType)
		if err == nil {
			return out, nil
		}
		lastErr = err
		wait, retryable := llmRetryDelay(err, attempt, initialBackoff, maxBackoff)
		if !retryable || attempt == maxAttempts {
			break
		}
		fmt.Printf("    ↻ LLM 限流/繁忙，等待 %s 后重试（%d/%d）\n", wait, attempt+1, maxAttempts)
		time.Sleep(wait)
	}
	return "", lastErr
}

func llmRetryDelay(err error, attempt int, initialBackoff, maxBackoff time.Duration) (time.Duration, bool) {
	if !isRetryableLLMError(err) {
		return 0, false
	}
	var httpErr *LLMHTTPError
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

func isRetryableLLMError(err error) bool {
	var httpErr *LLMHTTPError
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

func isBudgetExhausted(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "budget exhausted")
}
