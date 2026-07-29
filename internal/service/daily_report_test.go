package service

import (
	"strings"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestFormatTopicsForPrompt_LessThan30(t *testing.T) {
	topics := []string{"topic A", "topic B", "topic C"}
	result := formatTopicsForPrompt(topics)
	assert.Contains(t, result, "- topic A")
	assert.Contains(t, result, "- topic B")
	assert.Contains(t, result, "- topic C")
	assert.NotContains(t, result, "更多省略")
}

func TestFormatTopicsForPrompt_Exactly30(t *testing.T) {
	topics := make([]string, 30)
	for i := range topics {
		topics[i] = "topic"
	}
	result := formatTopicsForPrompt(topics)
	assert.NotContains(t, result, "更多省略")
}

func TestFormatTopicsForPrompt_Over30(t *testing.T) {
	topics := make([]string, 35)
	for i := range topics {
		topics[i] = "topic"
	}
	result := formatTopicsForPrompt(topics)
	assert.Contains(t, result, "更多省略")
}

func TestFormatTopicsForPrompt_Empty(t *testing.T) {
	result := formatTopicsForPrompt(nil)
	assert.Empty(t, strings.TrimSpace(result))
}

func TestActionStatEntryAggregation_RSIngest(t *testing.T) {
	entries := []ActionStatEntry{
		{Action: "ingest", Status: "success", Count: 100},
		{Action: "ingest", Status: "duplicate", Count: 10},
		{Action: "ingest", Status: "failure", Count: 3},
		{Action: "fetch", Status: "success", Count: 50},
	}

	ingest := &RSSIngestStats{}
	for _, s := range entries {
		if s.Action != "ingest" {
			continue
		}
		switch s.Status {
		case "success":
			ingest.Success = s.Count
		case "duplicate":
			ingest.Duplicate = s.Count
		case "failure":
			ingest.Failure = s.Count
		}
	}

	assert.Equal(t, int64(100), ingest.Success)
	assert.Equal(t, int64(10), ingest.Duplicate)
	assert.Equal(t, int64(3), ingest.Failure)
}

func TestActionStatEntryAggregation_FileIngest(t *testing.T) {
	entries := []ActionStatEntry{
		{Action: "ingest", Status: "success", Count: 20},
		{Action: "ingest", Status: "failure", Count: 2},
	}

	fi := &FileIngestStats{}
	for _, s := range entries {
		switch s.Status {
		case "success":
			fi.Success = s.Count
		case "failure":
			fi.Failure = s.Count
		}
	}

	assert.Equal(t, int64(20), fi.Success)
	assert.Equal(t, int64(2), fi.Failure)
}

func TestActionStatEntryAggregation_Classify(t *testing.T) {
	entries := []ActionStatEntry{
		{Action: "classify", Status: "success", Count: 200},
		{Action: "classify", Status: "failure", Count: 5},
	}

	cl := &ClassifyStats{}
	for _, s := range entries {
		switch s.Status {
		case "success":
			cl.Success = s.Count
		case "failure":
			cl.Failure = s.Count
		}
	}

	assert.Equal(t, int64(200), cl.Success)
	assert.Equal(t, int64(5), cl.Failure)
}

func TestDailyReportData_EmptyCollections(t *testing.T) {
	data := &DailyReportData{
		Date: "2026-06-12",
	}
	md := RenderDailyReport(data)
	assert.Contains(t, md, "2026-06-12 Bellkeeper 日报")
}

func TestDailyReport_CollectError_Structure(t *testing.T) {
	ce := CollectError{Source: "crawl", Error: "timeout"}
	assert.Equal(t, "crawl", ce.Source)
	assert.Equal(t, "timeout", ce.Error)
}

func TestGenerateOptions_Fields(t *testing.T) {
	opts := GenerateOptions{Date: "2026-06-12", DryRun: true, SkipNotify: true}
	assert.Equal(t, "2026-06-12", opts.Date)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.SkipNotify)
}

func TestBriefGenerateOptions_Fields(t *testing.T) {
	opts := BriefGenerateOptions{Date: "2026-06-12", WindowHours: 6}
	assert.Equal(t, "2026-06-12", opts.Date)
	assert.Equal(t, 6, opts.WindowHours)
}

func TestComputeBriefWindow(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 模拟每日 08:00 推送时刻触发。
	now := time.Date(2026, 6, 12, 8, 0, 0, 0, loc)

	t.Run("默认滚动24h：昨08:00→今08:00", func(t *testing.T) {
		start, end, err := computeBriefWindow(BriefGenerateOptions{}, now, loc)
		assert.NoError(t, err)
		assert.Equal(t, now, end)
		assert.Equal(t, time.Date(2026, 6, 11, 8, 0, 0, 0, loc), start)
		assert.Equal(t, 24*time.Hour, end.Sub(start))
	})

	t.Run("自定义窗口小时数", func(t *testing.T) {
		start, end, err := computeBriefWindow(BriefGenerateOptions{WindowHours: 6}, now, loc)
		assert.NoError(t, err)
		assert.Equal(t, now, end)
		assert.Equal(t, time.Date(2026, 6, 12, 2, 0, 0, 0, loc), start)
	})

	t.Run("指定历史日期→右界该日08:00", func(t *testing.T) {
		start, end, err := computeBriefWindow(BriefGenerateOptions{Date: "2026-06-10"}, now, loc)
		assert.NoError(t, err)
		assert.Equal(t, time.Date(2026, 6, 10, 8, 0, 0, 0, loc), end)
		assert.Equal(t, time.Date(2026, 6, 9, 8, 0, 0, 0, loc), start)
	})

	t.Run("非法日期返回错误", func(t *testing.T) {
		_, _, err := computeBriefWindow(BriefGenerateOptions{Date: "not-a-date"}, now, loc)
		assert.Error(t, err)
	})
}

func TestCategoryForArticle(t *testing.T) {
	domCat := map[string]string{"rss-source.example.com": "网络安全"}
	cases := []struct{ name, domain, tag, want string }{
		{"RSS源匹配优先", "rss-source.example.com", "", "网络安全"},
		{"技术字典-github归编程", "github.com", "", "编程"},
		{"技术字典-arxiv归AI", "arxiv.org", "", "人工智能"},
		{"技术字典-thehackernews归安全", "thehackernews.com", "", "网络安全"},
		{"www前缀归一命中", "www.github.com", "", "编程"},
		{"标签兜底归技术", "unknown.example.com", "人工智能", "人工智能"},
		{"标签为资讯不算数", "unknown.example.com", "资讯", "资讯"},
		{"全未知落资讯兜底", "unknown.example.com", "", "资讯"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := model.ArticleTag{SourceDomain: c.domain, Tag: model.Tag{Name: c.tag}}
			assert.Equal(t, c.want, categoryForArticle(a, domCat))
		})
	}
}

func TestIsNoiseDomain(t *testing.T) {
	assert.True(t, isNoiseDomain("youtube.com"))
	assert.True(t, isNoiseDomain("x.com"))
	assert.False(t, isNoiseDomain("github.com"))
	assert.False(t, isNoiseDomain(""))
}
