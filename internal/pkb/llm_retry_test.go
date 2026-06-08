package pkb

import (
	"fmt"
	"testing"
	"time"
)

func TestParseRetryAfterSeconds(t *testing.T) {
	got := parseRetryAfter("45")
	if got != 45*time.Second {
		t.Fatalf("parseRetryAfter seconds = %s, want 45s", got)
	}
}

func TestRetryableLLMErrorThroughWrapping(t *testing.T) {
	err := fmt.Errorf("score llm: %w", &LLMHTTPError{
		StatusCode: 429,
		Body:       `{"error":"rate limit"}`,
		RetryAfter: 30 * time.Second,
	})
	if !isRetryableLLMError(err) {
		t.Fatal("wrapped 429 should be retryable")
	}
	wait, retryable := llmRetryDelay(err, 1, 10*time.Second, 5*time.Minute)
	if !retryable {
		t.Fatal("wrapped 429 should produce retry delay")
	}
	if wait != 30*time.Second {
		t.Fatalf("retry delay = %s, want retry-after 30s", wait)
	}
}

func TestRetryDelayCapsExponentialBackoff(t *testing.T) {
	err := &LLMHTTPError{StatusCode: 503, Body: `{"error":"all group members exhausted"}`}
	wait, retryable := llmRetryDelay(err, 5, 20*time.Second, 60*time.Second)
	if !retryable {
		t.Fatal("503 exhausted should be retryable")
	}
	if wait != 60*time.Second {
		t.Fatalf("retry delay = %s, want cap 60s", wait)
	}
}
