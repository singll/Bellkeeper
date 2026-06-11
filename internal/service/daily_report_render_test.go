package service

import (
	"strings"
	"testing"
)

func TestRenderDailyReport_Full(t *testing.T) {
	data := &DailyReportData{
		Date: "2026-06-11",
		Health: &DetailedHealth{
			Status: "healthy",
			Services: map[string]ServiceStatus{
				"n8n":          {Status: "up", LatencyMs: 42},
				"meilisearch":  {Status: "up", LatencyMs: 5},
				"rss_fetcher":  {Status: "up"},
			},
		},
		Crawl: &CrawlDashboardStats{
			TodayNew:     100,
			TodaySuccess: 80,
			TodayFailed:  5,
			TodayRate:    0.941,
			FeedsTotal:   30,
			FeedsActive:  25,
			FeedsPaused:  5,
		},
		RSSIngest: &RSSIngestStats{
			Success: 253, Duplicate: 10, Failure: 3,
		},
		FileIngest: &FileIngestStats{
			Success: 5, Failure: 1,
		},
		Classify: &ClassifyStats{
			Success: 200, Failure: 2,
		},
		PKB: &PKBVaultStats{
			Trees: 8, Cards: 500, CardsToday: 6, Digests: 24,
		},
		PKBCards: []PKBCardSummary{
			{Title: "Rust并发模型深度解析", Domain: "programming", Score: 85},
			{Title: "GPT-5架构猜想", Domain: "ai", Score: 92},
		},
		LLM: &LLMDashboardStats{
			Requests24h:   531,
			Errors24h:     3,
			Tokens24h:     1569000,
			CostCents24h:  1250,
			SuccessRate:   0.994,
			AvgDurationMs: 2300,
		},
		Failures: []FailureDetail{
			{Summary: "RSS feed timeout: example.com/feed", RefID: "rss-123"},
		},
		AISummary: "今日知识库新增6张高价值卡片，爬取成功率94.1%，AI领域关注GPT-5架构动态。",
	}

	md := RenderDailyReport(data)

	if !strings.Contains(md, "2026-06-11 Bellkeeper 日报") {
		t.Error("missing report title")
	}
	if !strings.Contains(md, "✅ **n8n**") {
		t.Error("missing n8n status")
	}
	if !strings.Contains(md, "新增: 100") {
		t.Error("missing crawl today_new")
	}
	if !strings.Contains(md, "成功: 253") {
		t.Error("missing rss ingest success")
	}
	if !strings.Contains(md, "知识树: 8") {
		t.Error("missing pkb trees")
	}
	if !strings.Contains(md, "Rust并发模型") {
		t.Error("missing pkb card title")
	}
	if !strings.Contains(md, "请求: 531") {
		t.Error("missing llm requests")
	}
	if !strings.Contains(md, "RSS feed timeout") {
		t.Error("missing failure detail")
	}
	if !strings.Contains(md, "AI 亮点总结") {
		t.Error("missing AI summary section")
	}
}

func TestRenderDailyReport_PartialFailure(t *testing.T) {
	data := &DailyReportData{
		Date: "2026-06-11",
		Health: &DetailedHealth{
			Status: "degraded",
			Services: map[string]ServiceStatus{
				"n8n": {Status: "up"},
				"redis": {Status: "down", Error: "connection refused"},
			},
		},
		Errors: []CollectError{
			{Source: "crawl", Error: "database timeout"},
			{Source: "llm", Error: "unavailable"},
		},
	}

	md := RenderDailyReport(data)

	if !strings.Contains(md, "❌ **redis**") {
		t.Error("missing redis down status")
	}
	if !strings.Contains(md, "connection refused") {
		t.Error("missing redis error detail")
	}
	if !strings.Contains(md, "⚠️ 数据获取失败: crawl") {
		t.Error("missing crawl error in errors section")
	}
	if !strings.Contains(md, "⚠️ 数据获取失败: llm") {
		t.Error("missing llm error in errors section")
	}
	if !strings.Contains(md, "今日爬取") && !strings.Contains(md, "数据获取失败: crawl") {
		t.Error("crawl section should show error when data is nil")
	}
}

func TestRenderDailyReport_Empty(t *testing.T) {
	data := &DailyReportData{
		Date: "2026-06-11",
	}

	md := RenderDailyReport(data)

	if !strings.Contains(md, "2026-06-11 Bellkeeper 日报") {
		t.Error("missing report title")
	}
	if strings.Contains(md, "✅") {
		t.Error("should not have success icons when no data")
	}
}

func TestRenderBriefReport(t *testing.T) {
	data := &DailyReportData{
		Date: "2026-06-11",
		Crawl: &CrawlDashboardStats{
			TodayNew: 50, TodaySuccess: 40, TodayFailed: 2,
		},
		RSSIngest: &RSSIngestStats{
			Success: 120, Duplicate: 5, Failure: 1,
		},
		PKB: &PKBVaultStats{
			CardsToday: 3,
		},
		PKBCards: []PKBCardSummary{
			{Title: "测试卡片1", Domain: "ai", Score: 80},
			{Title: "测试卡片2", Domain: "security", Score: 75},
		},
		AISummary: "今日资讯摘要测试内容。",
	}

	md := RenderBriefReport(data)

	if !strings.Contains(md, "2026-06-11 资讯摘要") {
		t.Error("missing brief title")
	}
	if !strings.Contains(md, "新增 50") {
		t.Error("missing crawl data")
	}
	if !strings.Contains(md, "测试卡片1") {
		t.Error("missing card title")
	}
	if !strings.Contains(md, "今日资讯摘要测试内容") {
		t.Error("missing AI summary")
	}
}

func TestRenderHealthSection_StatusIcons(t *testing.T) {
	tests := []struct {
		status string
		icon   string
	}{
		{"up", "✅"},
		{"down", "❌"},
		{"unhealthy", "⚠️"},
		{"degraded", "⚠️"},
		{"disabled", "⏸️"},
		{"unknown", "❓"},
	}

	for _, tt := range tests {
		data := &DailyReportData{
			Date: "2026-06-11",
			Health: &DetailedHealth{
				Services: map[string]ServiceStatus{
					"test": {Status: tt.status},
				},
			},
		}
		md := RenderDailyReport(data)
		if !strings.Contains(md, tt.icon) {
			t.Errorf("status %q should render icon %q", tt.status, tt.icon)
		}
	}
}
