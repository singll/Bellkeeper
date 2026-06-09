package service

import (
	"testing"
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

func TestTruncateStr(t *testing.T) {
	if got := truncateStr("hello", 10); got != "hello" {
		t.Fatalf("truncateStr(\"hello\", 10) = %q, want \"hello\"", got)
	}
	if got := truncateStr("hello world", 5); got != "hello" {
		t.Fatalf("truncateStr(\"hello world\", 5) = %q, want \"hello\"", got)
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
