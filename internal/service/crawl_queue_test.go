package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
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

func TestDecideDomainThrottleNextAllowedAt(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	next := now.Add(2 * time.Minute)
	decision := decideDomainThrottle(&model.CrawlDomainProfile{
		Domain:              "example.com",
		DefaultDelaySeconds: 60,
		MaxConcurrency:      1,
		NextAllowedAt:       &next,
	}, 1, now)

	if decision.Allowed {
		t.Fatal("decision.Allowed = true, want false")
	}
	if decision.Reason != "next_allowed_at" {
		t.Fatalf("decision.Reason = %q, want next_allowed_at", decision.Reason)
	}
	if !decision.RetryAt.Equal(next) {
		t.Fatalf("decision.RetryAt = %s, want %s", decision.RetryAt, next)
	}
}

func TestDecideDomainThrottleMaxConcurrency(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	decision := decideDomainThrottle(&model.CrawlDomainProfile{
		Domain:              "example.com",
		DefaultDelaySeconds: 60,
		MaxConcurrency:      1,
	}, 2, now)

	if decision.Allowed {
		t.Fatal("decision.Allowed = true, want false")
	}
	if decision.Reason != "max_concurrency" {
		t.Fatalf("decision.Reason = %q, want max_concurrency", decision.Reason)
	}
	if !decision.RetryAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("decision.RetryAt = %s, want %s", decision.RetryAt, now.Add(time.Minute))
	}
}

func TestNextAllowedForDomainOutcomeUsesRetryAt(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	retryAt := now.Add(17 * time.Minute)
	got := nextAllowedForDomainOutcome(&model.CrawlDomainProfile{
		Domain:              "example.com",
		DefaultDelaySeconds: 60,
	}, string(model.CrawlJobRetrying), "rate_limited", now, &retryAt)
	if got == nil || !got.Equal(retryAt) {
		t.Fatalf("nextAllowed = %v, want %s", got, retryAt)
	}
}

func TestNextAllowedForDomainOutcomeRateLimitedFallback(t *testing.T) {
	now := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	got := nextAllowedForDomainOutcome(&model.CrawlDomainProfile{
		Domain:              "example.com",
		DefaultDelaySeconds: 60,
	}, string(model.CrawlJobRetrying), "rate_limited", now, nil)
	want := now.Add(5 * time.Minute)
	if got == nil || !got.Equal(want) {
		t.Fatalf("nextAllowed = %v, want %s", got, want)
	}
}
