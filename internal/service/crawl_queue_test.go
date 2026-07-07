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

// TestClassifyCrawlErrorExtractorUnavailable 坐实：firecrawl 传输层失败（连接失败 /
// 请求未发出的 "ContentLength=N with Body length 0"）归类为 extractor_unavailable，
// 不再落入 unknown 污染统计（线上 unknown 172 条根因）。
func TestClassifyCrawlErrorExtractorUnavailable(t *testing.T) {
	cases := []string{
		`firecrawl extraction failed: HTTP request failed: Post "http://192.168.7.220:3002/v1/scrape": http: ContentLength=124 with Body length 0`,
		`firecrawl extraction failed: HTTP request failed: dial tcp 192.168.7.220:3002: connect: connection refused`,
	}
	for _, msg := range cases {
		errType, _ := classifyCrawlError(fmt.Errorf("%s", msg))
		if errType != "extractor_unavailable" {
			t.Fatalf("errType = %q, want extractor_unavailable (msg=%q)", errType, msg)
		}
	}
}

// TestExtractorUnavailableIsNotDomainLevel 断言我方抓取器故障不计入域名健康度、
// 不触发整域暂停（边界：不得误伤目标站点）。
func TestExtractorUnavailableIsNotDomainLevel(t *testing.T) {
	if domainLevelFailure("extractor_unavailable") {
		t.Fatal("extractor_unavailable must not be domain-level")
	}
	// 反向守卫：真正的域名级失败仍需为 true。
	for _, dl := range []string{"network", "server_error", "forbidden", "rate_limited", "timeout"} {
		if !domainLevelFailure(dl) {
			t.Fatalf("%s must be domain-level", dl)
		}
	}
}

// TestNextAllowedForExtractorUnavailableShortCooling 断言 extractor_unavailable 使用
// 30s 短冷却，快速重试而非惩罚域名。
func TestNextAllowedForExtractorUnavailableShortCooling(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	got := nextAllowedForDomainOutcome(&model.CrawlDomainProfile{
		Domain:              "example.com",
		DefaultDelaySeconds: 60,
	}, string(model.CrawlJobRetrying), "extractor_unavailable", now, nil)
	want := now.Add(30 * time.Second)
	if got == nil || !got.Equal(want) {
		t.Fatalf("nextAllowed = %v, want %s", got, want)
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
