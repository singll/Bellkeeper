package pkb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCollectFeedItems 守住 ADR-0005 §5.1 资讯采集口径：只取当日(ingested_at)、资讯类(pkb_type∈feed_content_types)、
// 已打分(有 pkb_type/pkb_domain)的文章，跨 raw+archive 两层，按 pkb_domain 归领域。
func TestCollectFeedItems(t *testing.T) {
	base := t.TempDir()
	// 当日资讯类(news, raw 层) → 入选
	writeFeedTestFile(t, filepath.Join(base, "raw", "a.md"),
		"---\ntitle: \"Go 1.24 发布\"\nurl: \"https://go.dev/blog/go1.24\"\npkb_type: news\npkb_domain: programming\ningested_at: 2026-06-17T08:00:00+08:00\n---\nGo 1.24 带来新特性。\n")
	// 当日资讯类(release, archive 层) → 入选
	writeFeedTestFile(t, filepath.Join(base, "archive", "b.md"),
		"---\ntitle: \"React 20\"\nurl: \"https://react.dev\"\npkb_type: release\npkb_domain: programming\ningested_at: 2026-06-17T09:00:00+08:00\n---\nReact 20 正式发布。\n")
	// 当日但非资讯类(tutorial) → 排除
	writeFeedTestFile(t, filepath.Join(base, "raw", "c.md"),
		"---\ntitle: \"教程\"\npkb_type: tutorial\npkb_domain: programming\ningested_at: 2026-06-17T10:00:00+08:00\n---\n教程正文。\n")
	// 资讯类但非当日 → 排除
	writeFeedTestFile(t, filepath.Join(base, "raw", "d.md"),
		"---\ntitle: \"旧闻\"\npkb_type: news\npkb_domain: security\ningested_at: 2026-06-16T10:00:00+08:00\n---\n昨天的旧闻。\n")
	// 未打分(无 pkb_type) → 排除（主流程尚未处理）
	writeFeedTestFile(t, filepath.Join(base, "raw", "e.md"),
		"---\ntitle: \"未处理\"\ningested_at: 2026-06-17T11:00:00+08:00\n---\n尚未打分。\n")

	c := &Curator{
		basePath: base,
		domains:  &DomainsConfig{Defaults: Defaults{FeedContentTypes: []string{"news", "release"}}},
	}
	items := c.collectFeedItems("2026-06-17")
	if len(items) != 2 {
		t.Fatalf("当日资讯类应为 2（news+release），实得 %d: %+v", len(items), items)
	}
	by := groupFeedItems(items)
	if len(by["programming"]) != 2 {
		t.Fatalf("programming 应有 2 条，实得 %d", len(by["programming"]))
	}
	var titles []string
	for _, it := range items {
		titles = append(titles, it.title)
		assert.NotEmpty(t, it.url, "资讯条目应提取到 url")
	}
	assert.Contains(t, titles, "Go 1.24 发布")
	assert.Contains(t, titles, "React 20")
}

// TestFeedItemDate 守住「当日」判定：优先 ingested_at，回退 pkb_scored_at，RFC3339 截到 YYYY-MM-DD。
func TestFeedItemDate(t *testing.T) {
	assert.Equal(t, "2026-06-17", feedItemDate(map[string]string{"ingested_at": "2026-06-17T08:00:00+08:00"}))
	assert.Equal(t, "2026-06-15", feedItemDate(map[string]string{"pkb_scored_at": "2026-06-15T20:00:00Z"}))
	// ingested_at 优先于 pkb_scored_at
	assert.Equal(t, "2026-06-17", feedItemDate(map[string]string{
		"ingested_at":   "2026-06-17T08:00:00+08:00",
		"pkb_scored_at": "2026-06-18T20:00:00Z",
	}))
	assert.Equal(t, "", feedItemDate(map[string]string{}))
}

// TestFeedArchiveRoot 守住资讯库容器根取自 feed:true 领域；无则报缺失。
func TestFeedArchiveRoot(t *testing.T) {
	c := &Curator{domains: &DomainsConfig{Domains: []Domain{
		{Name: "programming", VaultSubpath: "vault/编程"},
		{Name: "news", VaultSubpath: "vault/资讯", Feed: true},
	}}}
	root, ok := c.feedArchiveRoot()
	assert.True(t, ok)
	assert.Equal(t, "vault/资讯", root)

	c2 := &Curator{domains: &DomainsConfig{Domains: []Domain{{Name: "programming"}}}}
	_, ok2 := c2.feedArchiveRoot()
	assert.False(t, ok2, "无 feed 领域时应返回 false")
}

// TestWriteFeedArchive 守住资讯存档落盘契约：路径 <root>/<display>/<date>.md + frontmatter type: pkb_feed。
func TestWriteFeedArchive(t *testing.T) {
	base := t.TempDir()
	c := &Curator{basePath: base}
	domain := Domain{Name: "programming", Display: "编程"}
	dst, err := c.writeFeedArchive("vault/资讯", domain, "2026-06-17", "今日要点：Go 1.24 发布。", 3)
	if err != nil {
		t.Fatalf("writeFeedArchive: %v", err)
	}
	want := filepath.Join(base, "vault", "资讯", "编程", "2026-06-17.md")
	if dst != want {
		t.Fatalf("落盘路径 = %q, want %q", dst, want)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	assert.Contains(t, content, "type: pkb_feed", "资讯存档须标 type: pkb_feed（非知识卡）")
	assert.Contains(t, content, "domain: programming")
	assert.Contains(t, content, "item_count: 3")
	assert.Contains(t, content, "### 2026-06-17 · 编程 资讯")
	assert.Contains(t, content, "今日要点：Go 1.24 发布。")
}

func writeFeedTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestRealDomainsYAMLLoadsFeedConfig 守住生产 config/pkb/domains.yaml 能加载且 Phase H 字段就位：
// promote_model 有值、feed_content_types 含 news、news 领域 feed: true（资讯库容器）。
func TestRealDomainsYAMLLoadsFeedConfig(t *testing.T) {
	dc, err := LoadDomains(filepath.Join("..", "..", "config", "pkb", "domains.yaml"))
	if err != nil {
		t.Fatalf("加载生产 domains.yaml 失败: %v", err)
	}
	assert.NotEmpty(t, dc.Defaults.PromoteModel, "promote_model 应有值（显式或回退）")
	assert.Contains(t, dc.Defaults.FeedContentTypes, "news", "feed_content_types 应含 news")
	news, ok := dc.FindDomain("news")
	assert.True(t, ok, "应有 news 领域")
	assert.True(t, news.Feed, "news 领域应配 feed: true（资讯库容器）")
}
