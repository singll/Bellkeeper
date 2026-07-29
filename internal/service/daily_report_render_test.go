package service

import (
	"strings"
	"testing"
)

// TestRenderFeedArchiveSection 守住日报「今日资讯存档」弱联动节（ADR-0005 §5.1）：列出领域→存档链接；空则不渲染。
func TestRenderFeedArchiveSection(t *testing.T) {
	data := &DailyReportData{FeedArchives: []PKBFeedArchive{
		{Domain: "编程", RelPath: "vault/资讯/编程/2026-06-17.md"},
		{Domain: "安全", RelPath: "vault/资讯/安全/2026-06-17.md"},
	}}
	out := renderFeedArchiveSection(data)
	if !strings.Contains(out, "#### 今日资讯存档") {
		t.Fatal("应含「今日资讯存档」节标题")
	}
	if !strings.Contains(out, "[编程](vault/资讯/编程/2026-06-17.md)") {
		t.Fatalf("应含编程资讯链接，实得: %s", out)
	}
	if renderFeedArchiveSection(&DailyReportData{}) != "" {
		t.Fatal("无资讯存档时应返回空串（不渲染该节）")
	}
}

func TestRenderDailyReport_Full(t *testing.T) {
	data := &DailyReportData{
		Date: "2026-06-11",
		Health: &DetailedHealth{
			Status: "healthy",
			Services: map[string]ServiceStatus{
				"n8n":         {Status: "up", LatencyMs: 42},
				"meilisearch": {Status: "up", LatencyMs: 5},
				"rss_fetcher": {Status: "up"},
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
				"n8n":   {Status: "up"},
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

func TestRenderNewsBrief(t *testing.T) {
	data := &NewsBriefData{
		Date:        "2026-06-11",
		WindowStart: "06-10 08:00",
		WindowEnd:   "06-11 08:00",
		WindowHours: 24,
		Total:       3,
		AISummary:   "今日总结测试内容。\n\n**看点**\n- 要点一",
		Groups: []NewsGroup{
			{Category: "编程", Items: []NewsItem{
				{Title: "Go 1.99 发布", URL: "https://go.dev/a", Domain: "go.dev", Category: "编程"},
			}},
			{Category: "人工智能", Items: []NewsItem{
				{Title: "某模型 [beta] 上线", URL: "https://x.ai/b", Domain: "x.ai", Category: "人工智能"},
				{Title: "论文速览", URL: "https://y.com/c", Category: "人工智能"},
			}},
		},
	}

	md := RenderNewsBrief(data)

	if !strings.Contains(md, "每日资讯早报 · 2026-06-11") {
		t.Error("missing brief title")
	}
	if !strings.Contains(md, "共 3 条") {
		t.Error("missing total count")
	}
	if !strings.Contains(md, "今日总结测试内容") {
		t.Error("missing AI summary")
	}
	if !strings.Contains(md, "[Go 1.99 发布](https://go.dev/a)") {
		t.Error("missing item link")
	}
	// 标题内方括号应被转义，避免破坏 markdown 链接
	if !strings.Contains(md, "某模型 【beta】 上线") {
		t.Error("bracket in title not escaped")
	}
}

func TestRenderNewsBrief_Empty(t *testing.T) {
	data := &NewsBriefData{Date: "2026-06-11", WindowStart: "06-10 08:00", WindowEnd: "06-11 08:00", WindowHours: 24, Total: 0}
	md := RenderNewsBrief(data)
	if !strings.Contains(md, "暂无新入库资讯") {
		t.Error("empty brief should state no news")
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
