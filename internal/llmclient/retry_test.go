package llmclient

import (
	"fmt"
	"testing"
	"time"
)

func TestParseRetryAfterSeconds(t *testing.T) {
	got := ParseRetryAfter("45")
	if got != 45*time.Second {
		t.Fatalf("ParseRetryAfter seconds = %s, want 45s", got)
	}
}

func TestRetryableHTTPErrorThroughWrapping(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &HTTPError{
		StatusCode: 429,
		Body:       `{"error":"rate limit"}`,
		RetryAfter: 30 * time.Second,
	})
	if !IsRetryable(err) {
		t.Fatal("wrapped 429 should be retryable")
	}
	wait, retryable := RetryDelay(err, 1, 10*time.Second, 5*time.Minute)
	if !retryable {
		t.Fatal("wrapped 429 should produce retry delay")
	}
	if wait != 30*time.Second {
		t.Fatalf("retry delay = %s, want retry-after 30s", wait)
	}
}

func TestRetryDelayCapsExponentialBackoff(t *testing.T) {
	err := &HTTPError{StatusCode: 503, Body: `{"error":"all group members exhausted"}`}
	wait, retryable := RetryDelay(err, 5, 20*time.Second, 60*time.Second)
	if !retryable {
		t.Fatal("503 exhausted should be retryable")
	}
	if wait != 60*time.Second {
		t.Fatalf("retry delay = %s, want cap 60s", wait)
	}
}
