package service

import (
	"testing"

	"github.com/singll/bellkeeper/internal/repository"
)

func TestScoreContentEmpty(t *testing.T) {
	optimizer := &RuleOptimizerService{}
	score := optimizer.scoreContent("", 200)
	if score != 0 {
		t.Fatalf("score for empty content = %.2f, want 0", score)
	}
}

func TestScoreContentGoodArticle(t *testing.T) {
	optimizer := &RuleOptimizerService{}
	content := "This is a good article about technology and science. It contains many words and provides useful information for the reader. The content is substantial and does not contain any paywall or subscription keywords. It is a well-written piece that would be valuable for knowledge management and research purposes. We can see that this text has enough length and quality to pass the extraction threshold."
	score := optimizer.scoreContent(content, 200)
	if score < 0.5 {
		t.Fatalf("score for good article = %.2f, want >= 0.5", score)
	}
}

func TestScoreContentPaywall(t *testing.T) {
	optimizer := &RuleOptimizerService{}
	content := "This article requires a subscription to continue reading. Premium content is only available to members. Subscribe to continue accessing exclusive articles and in-depth analysis."
	score := optimizer.scoreContent(content, 200)
	if score > 0.3 {
		t.Fatalf("score for paywall content = %.2f, want <= 0.3", score)
	}
}

func TestScoreContentShortButValid(t *testing.T) {
	optimizer := &RuleOptimizerService{}
	content := "A short but valid piece of text with some useful words about technology and programming."
	score := optimizer.scoreContent(content, 200)
	if score <= 0 {
		t.Fatalf("score for short valid content = %.2f, want > 0", score)
	}
}

func TestDefaultOverridesForForbidden(t *testing.T) {
	ov, analysis := defaultOverridesFor("forbidden")
	if ov == nil {
		t.Fatal("expected overrides for forbidden, got nil")
	}
	if ov.UserAgent == "" {
		t.Fatal("expected a spoofed User-Agent for 403")
	}
	if ov.Headers["Referer"] == "" {
		t.Fatal("expected a Referer header for 403")
	}
	if analysis == "" {
		t.Fatal("expected analysis text")
	}
}

func TestDefaultOverridesForEmptyContent(t *testing.T) {
	// empty_content 不再走确定性 waitFor 规则（会强制 playwright 烧机场流量）；改为返回 nil，
	// 交 LLM 分析 + gateWaitFor 验证闸决定是否真需 waitFor。
	ov, _ := defaultOverridesFor("empty_content")
	if ov != nil {
		t.Fatalf("expected nil overrides for empty_content (no auto waitFor), got %+v", ov)
	}
}

func TestDefaultOverridesForTransientReturnsNil(t *testing.T) {
	// rate_limited and server_error are handled by backoff, not overrides → LLM fallback path.
	for _, errType := range []string{"rate_limited", "server_error", "unknown", ""} {
		if ov, _ := defaultOverridesFor(errType); ov != nil {
			t.Fatalf("expected nil overrides for %q, got %+v", errType, ov)
		}
	}
}

func TestDominantErrorType(t *testing.T) {
	samples := []repository.DomainFailureSample{
		{ErrorType: "forbidden"},
		{ErrorType: "forbidden"},
		{ErrorType: "timeout"},
		{ErrorType: ""},
	}
	if got := dominantErrorType(samples); got != "forbidden" {
		t.Fatalf("dominantErrorType = %q, want forbidden", got)
	}
	if got := dominantErrorType(nil); got != "" {
		t.Fatalf("dominantErrorType(nil) = %q, want empty", got)
	}
}

func TestNormalizePositive(t *testing.T) {
	if got := normalizePositive(0, 60); got != 60 {
		t.Fatalf("normalizePositive(0, 60) = %d, want 60", got)
	}
	if got := normalizePositive(-1, 60); got != 60 {
		t.Fatalf("normalizePositive(-1, 60) = %d, want 60", got)
	}
	if got := normalizePositive(30, 60); got != 30 {
		t.Fatalf("normalizePositive(30, 60) = %d, want 30", got)
	}
}

func TestStripWaitFor(t *testing.T) {
	ov := &RequestOverrides{
		UserAgent:        "UA",
		Headers:          map[string]string{"Accept-Language": "en"},
		Strategy:         "firecrawl",
		FirecrawlWaitFor: 3000,
		TimeoutSeconds:   60,
	}
	got := stripWaitFor(ov)
	if got.FirecrawlWaitFor != 0 {
		t.Fatalf("waitFor not stripped: %d", got.FirecrawlWaitFor)
	}
	if got.Strategy != "" {
		t.Fatalf("firecrawl strategy not cleared: %q", got.Strategy)
	}
	// 反 403 伪装（UA/Header/Timeout）必须保留
	if got.UserAgent != "UA" || got.Headers["Accept-Language"] != "en" || got.TimeoutSeconds != 60 {
		t.Fatalf("stripWaitFor damaged non-waitFor fields: %+v", got)
	}
	// 不得改动入参
	if ov.FirecrawlWaitFor != 3000 {
		t.Fatal("stripWaitFor mutated the input")
	}
	// nil 安全
	if stripWaitFor(nil) != nil {
		t.Fatal("stripWaitFor(nil) should be nil")
	}
	// 非 firecrawl 的 strategy 应保留
	if got2 := stripWaitFor(&RequestOverrides{Strategy: "trafilatura", FirecrawlWaitFor: 1000}); got2.Strategy != "trafilatura" {
		t.Fatalf("non-firecrawl strategy should be kept, got %q", got2.Strategy)
	}
}

func TestDecideKeepWaitFor(t *testing.T) {
	const pass = 0.6
	cases := []struct {
		name                     string
		fetchScore, waitForScore float64
		want                     bool
	}{
		{"fetch充分则不保留", 0.7, 0.9, false},
		{"fetch刚好达标则不保留", 0.6, 0.9, false},
		{"fetch不足且pw充分则保留", 0.3, 0.7, true},
		{"fetch不足pw刚达标则保留", 0.5, 0.6, true},
		{"fetch不足但pw也不足则不保留", 0.3, 0.5, false},
		{"两者皆0则不保留", 0.0, 0.0, false},
	}
	for _, c := range cases {
		if got := decideKeepWaitFor(c.fetchScore, c.waitForScore, pass); got != c.want {
			t.Errorf("%s: decideKeepWaitFor(%.2f,%.2f)=%v want %v", c.name, c.fetchScore, c.waitForScore, got, c.want)
		}
	}
}
