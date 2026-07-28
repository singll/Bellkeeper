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

// TestWriteFeedDaily 守住资讯「一天一文件」落盘契约：路径 <root>/<date>.md + type: pkb_feed + 按 ## 小节。
func TestWriteFeedDaily(t *testing.T) {
	base := t.TempDir()
	c := &Curator{basePath: base}
	sections := []feedSection{
		{name: "编程", summary: "今日要点：Go 1.24 发布。", count: 3},
		{name: "AI", summary: "GPT 新模型发布。", count: 2},
	}
	dst, err := c.writeFeedDaily("vault/资讯", "2026-06-17", sections, 5)
	if err != nil {
		t.Fatalf("writeFeedDaily: %v", err)
	}
	want := filepath.Join(base, "vault", "资讯", "2026-06-17.md")
	if dst != want {
		t.Fatalf("落盘路径 = %q, want %q", dst, want)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	assert.Contains(t, content, "type: pkb_feed", "资讯存档须标 type: pkb_feed（非知识卡）")
	assert.Contains(t, content, "item_count: 5")
	assert.Contains(t, content, "# 2026-06-17 资讯")
	assert.Contains(t, content, "## 编程")
	assert.Contains(t, content, "## AI")
	assert.Contains(t, content, "今日要点：Go 1.24 发布。")
	assert.Contains(t, content, "GPT 新模型发布。")
}

// TestFeedSectionName 守住领域→小节映射：兜底/资讯容器域归「其他」，其余用 FeedSection/Display。
func TestFeedSectionName(t *testing.T) {
	assert.Equal(t, "AI", feedSectionName(Domain{Name: "ai", Display: "人工智能", FeedSection: "AI"}))
	assert.Equal(t, "编程", feedSectionName(Domain{Name: "programming", Display: "编程"}))
	assert.Equal(t, "其他", feedSectionName(Domain{Name: "misc", Display: "周边杂项", IsDefault: true}))
	assert.Equal(t, "其他", feedSectionName(Domain{Name: "news", Display: "最新资讯", Feed: true}))
}

// TestOrderedSections 守住小节排序：「其他」始终垫底。
func TestOrderedSections(t *testing.T) {
	byName := map[string]*feedSection{
		"AI": {name: "AI"},
		"其他": {name: "其他"},
		"编程": {name: "编程"},
	}
	got := orderedSections(byName, []string{"AI", "其他", "编程"})
	if len(got) != 3 {
		t.Fatalf("应 3 小节，实得 %d", len(got))
	}
	if got[len(got)-1].name != "其他" {
		t.Errorf("「其他」应垫底，实际末位: %s", got[len(got)-1].name)
	}
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

// TestParsePromoteCandidates 守住晋升候选解析：CONCEPT/DURABILITY/NOVELTY/EVENT 字段提取 + 去引号。
func TestParsePromoteCandidates(t *testing.T) {
	out := "CONCEPT: 分代垃圾回收 | DURABILITY: 9 | NOVELTY: 5 | EVENT: no | REASON: 通用 GC 原理\n" +
		"CONCEPT: \"Go 1.24 发布\" | DURABILITY: 3 | NOVELTY: 7 | EVENT: yes | REASON: 版本事件\n" +
		"NONE"
	cands := parsePromoteCandidates(out)
	if len(cands) != 2 {
		t.Fatalf("应解析 2 个候选，实得 %d: %+v", len(cands), cands)
	}
	assert.Equal(t, "分代垃圾回收", cands[0].concept)
	assert.Equal(t, 9, cands[0].durability)
	assert.Equal(t, 5, cands[0].novelty)
	assert.False(t, cands[0].event)
	assert.Equal(t, "Go 1.24 发布", cands[1].concept, "应去掉引号")
	assert.True(t, cands[1].event, "版本事件应标 event=true")
}

// TestParsePromoteCandidatesEmpty 守住 NONE / 无 CONCEPT 行 / fence 包裹时返回空。
func TestParsePromoteCandidatesEmpty(t *testing.T) {
	assert.Empty(t, parsePromoteCandidates("NONE"))
	assert.Empty(t, parsePromoteCandidates("```\nNONE\n```"))
	assert.Empty(t, parsePromoteCandidates("当天没有耐久知识点。"))
}

// TestShouldPromote 守住晋升把闸（ADR-0005 §5.2）：非事件 && durability≥阈值才晋升。
func TestShouldPromote(t *testing.T) {
	assert.True(t, shouldPromote(promoteCandidate{durability: 8, event: false}, 7.0), "耐久且达阈值应晋升")
	assert.False(t, shouldPromote(promoteCandidate{durability: 9, event: true}, 7.0), "事件性不晋升")
	assert.False(t, shouldPromote(promoteCandidate{durability: 5, event: false}, 7.0), "durability 不足不晋升")
	assert.True(t, shouldPromote(promoteCandidate{durability: 7, event: false}, 7.0), "恰好达阈值应晋升")
}
