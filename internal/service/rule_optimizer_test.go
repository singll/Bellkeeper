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
	ov, _ := defaultOverridesFor("empty_content")
	if ov == nil || ov.Strategy != "firecrawl" {
		t.Fatalf("expected firecrawl strategy for empty_content, got %+v", ov)
	}
	if ov.FirecrawlWaitFor <= 0 {
		t.Fatal("expected a positive firecrawl_wait_for for empty_content")
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
