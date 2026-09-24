package errors

import (
	"testing"
	"time"
)

func TestClassify_OpenCodeGoFiveHourWindow(t *testing.T) {
	// OpenCode Go 5h rolling window exhaustion: previously hardcoded to a 24h
	// breakdown ("long"), leaving the member out of rotation a full day after
	// the window had already recovered.
	body := `{"error":{"type":"GoUsageLimitError","message":"usage limit reached, resets in 3 hours"}}`
	r := Classify(429, body, "")
	if r.Class != QuotaExhausted {
		t.Fatalf("expected quota_exhausted, got %s", r.Class)
	}
	d := BreakdownDuration(r.BreakdownUntil)
	// 3h + 10% margin = 3h18m
	if d < 3*time.Hour || d > 4*time.Hour {
		t.Fatalf("expected ~3.3h breakdown, got %v (hint %q)", d, r.BreakdownUntil)
	}
}

func TestClassify_OpenCodeGoMinutesWindow(t *testing.T) {
	body := `{"error":"usage limit reached, resets in 42 minutes"}`
	r := Classify(429, body, "")
	if r.Class != QuotaExhausted {
		t.Fatalf("expected quota_exhausted, got %s", r.Class)
	}
	d := BreakdownDuration(r.BreakdownUntil)
	if d < 42*time.Minute || d > 60*time.Minute {
		t.Fatalf("expected ~46m breakdown, got %v (hint %q)", d, r.BreakdownUntil)
	}
}

func TestClassify_MonthlyQuotaStaysLong(t *testing.T) {
	body := `{"error":{"type":"GoUsageLimitError","message":"Monthly usage limit reached. Resets in 12 days"}}`
	r := Classify(429, body, "")
	if r.Class != QuotaExhausted {
		t.Fatalf("expected quota_exhausted, got %s", r.Class)
	}
	if r.BreakdownUntil != "long" {
		t.Fatalf("expected long breakdown for monthly quota, got %q", r.BreakdownUntil)
	}
	if d := BreakdownDuration(r.BreakdownUntil); d != 24*time.Hour {
		t.Fatalf("expected 24h for long, got %v", d)
	}
}

func TestClassify_MonthlyWithoutResetPhrase(t *testing.T) {
	body := `{"error":"monthly usage limit reached"}`
	r := Classify(429, body, "")
	if r.Class != QuotaExhausted || r.BreakdownUntil != "long" {
		t.Fatalf("expected quota_exhausted/long, got %s/%q", r.Class, r.BreakdownUntil)
	}
}

func TestClassify_QuotaNoResetHintDefaultsFiveHours(t *testing.T) {
	// 无 reset 提示、非月度：默认 5h（最短已知滚动窗），配合恢复探针快速回池。
	body := `{"error":"quota exhausted for this window"}`
	r := Classify(429, body, "")
	if r.Class != QuotaExhausted {
		t.Fatalf("expected quota_exhausted, got %s", r.Class)
	}
	if r.BreakdownUntil != "5h" {
		t.Fatalf("expected 5h default, got %q", r.BreakdownUntil)
	}
}

func TestClassify_WeeklyWindowCappedLong(t *testing.T) {
	body := `{"error":"usage limit reached, resets in 4 days"}`
	r := Classify(429, body, "")
	if r.BreakdownUntil != "long" {
		t.Fatalf("expected long for multi-day reset, got %q", r.BreakdownUntil)
	}
}

func TestClassify_TransientRateLimitUnchanged(t *testing.T) {
	body := `{"error":"rate limit exceeded, try again shortly"}`
	r := Classify(429, body, "")
	if r.Class != RateLimitedRetry || r.BreakdownUntil != "30s" {
		t.Fatalf("expected rate_limited_retry/30s, got %s/%q", r.Class, r.BreakdownUntil)
	}
}

func TestParseResetInDuration(t *testing.T) {
	cases := []struct {
		body  string
		want  time.Duration
		ok    bool
	}{
		{"resets in 5 hours", 5 * time.Hour, true},
		{"resets in 30 minutes", 30 * time.Minute, true},
		{"resets in 1 day", 24 * time.Hour, true},
		{"reset in 2.5 hours", 150 * time.Minute, true},
		{"retry after 10 minutes", 10 * time.Minute, true},
		{"no reset info here", 0, false},
		{"resets eventually", 0, false},
	}
	for _, c := range cases {
		got, ok := parseResetInDuration(c.body)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseResetInDuration(%q) = %v,%v want %v,%v", c.body, got, ok, c.want, c.ok)
		}
	}
}
