package service

import (
	"os"
	"path/filepath"
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

	t.Run("6-6闭合窗：昨06:00→今06:00", func(t *testing.T) {
		start, end, err := computeBriefWindow(BriefGenerateOptions{}, now, loc)
		assert.NoError(t, err)
		assert.Equal(t, time.Date(2026, 6, 12, 6, 0, 0, 0, loc), end)
		assert.Equal(t, time.Date(2026, 6, 11, 6, 0, 0, 0, loc), start)
		assert.Equal(t, 24*time.Hour, end.Sub(start))
	})

	t.Run("自定义窗口小时数（右界仍卡06:00）", func(t *testing.T) {
		start, end, err := computeBriefWindow(BriefGenerateOptions{WindowHours: 6}, now, loc)
		assert.NoError(t, err)
		assert.Equal(t, time.Date(2026, 6, 12, 6, 0, 0, 0, loc), end)
		assert.Equal(t, time.Date(2026, 6, 12, 0, 0, 0, 0, loc), start)
	})

	t.Run("指定历史日期→右界该日06:00", func(t *testing.T) {
		start, end, err := computeBriefWindow(BriefGenerateOptions{Date: "2026-06-10"}, now, loc)
		assert.NoError(t, err)
		assert.Equal(t, time.Date(2026, 6, 10, 6, 0, 0, 0, loc), end)
		assert.Equal(t, time.Date(2026, 6, 9, 6, 0, 0, 0, loc), start)
	})

	t.Run("触发时刻早于06:00→右界退到昨日06:00", func(t *testing.T) {
		early := time.Date(2026, 6, 12, 3, 0, 0, 0, loc)
		start, end, err := computeBriefWindow(BriefGenerateOptions{}, early, loc)
		assert.NoError(t, err)
		assert.Equal(t, time.Date(2026, 6, 11, 6, 0, 0, 0, loc), end)
		assert.Equal(t, time.Date(2026, 6, 10, 6, 0, 0, 0, loc), start)
	})

	t.Run("非法日期返回错误", func(t *testing.T) {
		_, _, err := computeBriefWindow(BriefGenerateOptions{Date: "not-a-date"}, now, loc)
		assert.Error(t, err)
	})
}

func TestWriteBriefArchive(t *testing.T) {
	tmp := t.TempDir()
	s := &DailyReportService{knowledgeBasePath: tmp}

	markdown := "## ☀️ 每日资讯早报 · 2026-07-30\n> 🕗 汇总时段 07-29 06:00 → 07-30 06:00（近 24h）· 共 3 条\n\n### 💻 编程（1）\n- [x](https://example.com) · example.com\n"
	dst, err := s.writeBriefArchive("2026-07-30", markdown, 3)
	assert.NoError(t, err)

	// 落地路径须锚定 knowledgeBasePath 下 vault/资讯/<date>.md（与 PKBReportService 读取同一根）。
	assert.Equal(t, filepath.Join(tmp, "vault", "资讯", "2026-07-30.md"), dst)

	raw, err := os.ReadFile(dst)
	assert.NoError(t, err)
	content := string(raw)
	assert.Contains(t, content, "type: pkb_feed")            // 兼容资讯库识别/时间线浏览
	assert.Contains(t, content, "item_count: 3")             // 条目数写入 frontmatter
	assert.Contains(t, content, "source: news_brief")        // 标记来源=早报
	assert.Contains(t, content, "## ☀️ 每日资讯早报 · 2026-07-30") // 整份早报正文原样落地
	assert.Contains(t, content, "汇总时段 07-29 06:00 → 07-30 06:00")

	// 幂等：同日重跑整份覆盖（不追加、不报错）。
	dst2, err := s.writeBriefArchive("2026-07-30", markdown, 3)
	assert.NoError(t, err)
	assert.Equal(t, dst, dst2)
	raw2, _ := os.ReadFile(dst2)
	assert.Equal(t, content, string(raw2))
}

func TestWriteBriefArchive_NoBasePath(t *testing.T) {
	s := &DailyReportService{knowledgeBasePath: ""}
	_, err := s.writeBriefArchive("2026-07-30", "x", 1)
	assert.Error(t, err) // 未配置 knowledge 根须显式报错，不静默吞掉
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
