package service

import (
	"fmt"
	"testing"
	"time"
)

func TestClassifyCrawlErrorRateLimited(t *testing.T) {
	errType, _ := classifyCrawlError(fmt.Errorf("HTTP 429 retry_after=\"120\": too many requests"))
	if errType != "rate_limited" {
		t.Fatalf("errType = %q, want rate_limited", errType)
	}
}

func TestRetryAfterFromErrorSeconds(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	got, ok := retryAfterFromError(fmt.Errorf("HTTP 429 retry_after=\"120\": too many requests"), now)
	if !ok {
		t.Fatal("retryAfterFromError ok = false")
	}
	want := now.Add(120 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("retryAfterFromError = %s, want %s", got, want)
	}
}

func TestRetryAfterFromErrorHTTPDate(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	got, ok := retryAfterFromError(fmt.Errorf("HTTP 429 retry_after=\"Tue, 09 Jun 2026 10:05:00 GMT\""), now)
	if !ok {
		t.Fatal("retryAfterFromError ok = false")
	}
	want := now.Add(5 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("retryAfterFromError = %s, want %s", got, want)
	}
}
