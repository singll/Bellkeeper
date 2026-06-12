package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMProxyRepository_CreateLogAndGetRecentLogs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	log1 := &model.LLMProxyLog{
		ChannelName: "openai-main",
		Model:       "gpt-4o",
		RequestPath: "/v1/chat/completions",
		StatusCode:  200,
		DurationMs:  500,
		CallerID:    "caller-1",
	}
	assertNoError(t, repo.CreateLog(log1), "CreateLog")

	logs, err := repo.GetRecentLogs("", 10)
	assertNoError(t, err, "GetRecentLogs")
	assertEqual(t, len(logs), 1)
	assertEqual(t, logs[0].Model, "gpt-4o")
}

func TestLLMProxyRepository_GetRecentLogsByChannel(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "openai-main", Model: "gpt-4o"}), "Create 1")
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "deepseek", Model: "deepseek-chat"}), "Create 2")

	logs, err := repo.GetRecentLogs("openai-main", 10)
	assertNoError(t, err, "GetRecentLogs")
	assertEqual(t, len(logs), 1)
	assertEqual(t, logs[0].ChannelName, "openai-main")
}

func TestLLMProxyRepository_SaveAlertEventAndList(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	since := time.Now().Add(-1 * time.Hour)
	event := &model.LLMAlertEvent{
		AlertType:   "circuit_open",
		Severity:    "error",
		ChannelID:   1,
		ChannelName: "openai-main",
		Message:     "Circuit breaker opened",
		DedupKey:    "circuit-openai",
	}
	assertNoError(t, repo.SaveAlertEvent(event), "SaveAlertEvent")

	events, err := repo.ListAlertEvents(since, "", "", 10)
	assertNoError(t, err, "ListAlertEvents")
	assertEqual(t, len(events), 1)
	assertEqual(t, events[0].AlertType, "circuit_open")
}

func TestLLMProxyRepository_ListAlertEventsWithFilters(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	since := time.Now().Add(-1 * time.Hour)
	assertNoError(t, repo.SaveAlertEvent(&model.LLMAlertEvent{
		AlertType: "circuit_open", Severity: "error", ChannelName: "ch1", DedupKey: "k1",
	}), "Create 1")
	assertNoError(t, repo.SaveAlertEvent(&model.LLMAlertEvent{
		AlertType: "quota_threshold", Severity: "warning", ChannelName: "ch2", DedupKey: "k2",
	}), "Create 2")

	events, err := repo.ListAlertEvents(since, "error", "", 10)
	assertNoError(t, err, "ListAlertEvents severity=error")
	assertEqual(t, len(events), 1)

	events, err = repo.ListAlertEvents(since, "", "circuit_open", 10)
	assertNoError(t, err, "ListAlertEvents alertType=circuit_open")
	assertEqual(t, len(events), 1)
}

func TestLLMProxyRepository_CleanOldLogs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "ch1", Model: "m1"}), "Create")

	affected, err := repo.CleanOldLogs(0)
	assertNoError(t, err, "CleanOldLogs")
	assertEqual(t, affected, int64(1))
}

func TestLLMProxyRepository_SummarySince(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	since := time.Now().Add(-1 * time.Hour)
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "ch1", Model: "gpt-4o", StatusCode: 200, PromptTokens: 100, CompTokens: 50, CostMicroCents: 2000}), "Create 1")
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "ch1", Model: "gpt-4o", StatusCode: 429, IsRateLimit: true, PromptTokens: 0, CompTokens: 0}), "Create 2")
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "ch2", Model: "deepseek", StatusCode: 500, PromptTokens: 200, CompTokens: 100, CostMicroCents: 500}), "Create 3")

	summary, err := repo.SummarySince(since)
	assertNoError(t, err, "SummarySince")
	assertEqual(t, summary.TotalRequests, int64(3))
	assertEqual(t, summary.ErrorCount, int64(2))
	assertEqual(t, summary.RateLimits, int64(1))
	assertEqual(t, summary.PromptTokens, int64(300))
	assertEqual(t, summary.CompTokens, int64(150))
}

func TestLLMProxyRepository_GetStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	since := time.Now().Add(-1 * time.Hour)
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "ch1", Model: "gpt-4o", StatusCode: 200, DurationMs: 500, RetryCount: 0}), "Create 1")
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "ch1", Model: "gpt-4o", StatusCode: 429, IsRateLimit: true, DurationMs: 100, RetryCount: 1}), "Create 2")
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{ChannelName: "ch2", Model: "deepseek", StatusCode: 200, DurationMs: 300, RetryCount: 0}), "Create 3")

	stats, err := repo.GetStats(since)
	assertNoError(t, err, "GetStats")
	assertEqual(t, len(stats), 2)

	var ch1Stat *ChannelStat
	for i := range stats {
		if stats[i].ChannelName == "ch1" {
			ch1Stat = &stats[i]
		}
	}
	assertEqual(t, ch1Stat != nil, true)
	assertEqual(t, ch1Stat.TotalRequests, int64(2))
	assertEqual(t, ch1Stat.RateLimitCount, int64(1))
}

func TestLLMProxyRepository_AggregateByModel(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMProxyRepository(db)

	from := time.Now().Add(-1 * time.Hour)
	to := time.Now().Add(1 * time.Hour)
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{Model: "gpt-4o", StatusCode: 200, PromptTokens: 100, CompTokens: 50, CostCents: 1, CostMicroCents: 1500}), "Create 1")
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{Model: "gpt-4o", StatusCode: 200, PromptTokens: 200, CompTokens: 100, CostCents: 2, CostMicroCents: 2500}), "Create 2")
	assertNoError(t, repo.CreateLog(&model.LLMProxyLog{Model: "deepseek-chat", StatusCode: 500, PromptTokens: 50, CompTokens: 25, CostMicroCents: 300}), "Create 3")

	results, err := repo.AggregateByModel(from, to)
	assertNoError(t, err, "AggregateByModel")
	assertEqual(t, len(results), 2)

	var gptResult map[string]interface{}
	for _, r := range results {
		if r["model"] == "gpt-4o" {
			gptResult = r
		}
	}
	assertEqual(t, gptResult != nil, true)
	assertEqual(t, gptResult["requests"], int64(2))
}
