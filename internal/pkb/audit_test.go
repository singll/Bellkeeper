package pkb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComputeAuditMetrics_LowConfidence 守住 §6 低置信卡指标：
// verification ∈ {unverified, llm-only} 计入低置信；verified/空与非卡节点不计。
func TestComputeAuditMetrics_LowConfidence(t *testing.T) {
	g := &auditGraph{
		nodes: map[string]*auditNode{
			"a.md":   {AtomicConcept: "A", Type: "pkb_card", Verification: "verified"},
			"b.md":   {AtomicConcept: "B", Type: "pkb_card", Verification: "unverified"},
			"c.md":   {AtomicConcept: "C", Type: "pkb_card", Verification: "llm-only"},
			"d.md":   {AtomicConcept: "D", Type: "pkb_card", Verification: ""},
			"map.md": {AtomicConcept: "M", Type: "pkb_map", Verification: ""}, // 非卡，不计入 TotalCards
		},
		conceptIndex: map[string][]string{},
		validTargets: map[string]bool{},
	}
	r := g.computeAuditMetrics()
	assert.Equal(t, 4, r.TotalCards)         // 4 张 pkb_card（pkb_map 不计）
	assert.Equal(t, 2, r.LowConfidenceCards) // unverified + llm-only
	assert.InDelta(t, 50.0, r.LowConfidenceRate, 0.01)
}

// TestBuildAuditGraphSkipsFeedDomain 守住 ADR-0005：资讯库容器领域（Feed=true）的分日存档
// 不是知识原子卡，audit 建图按领域跳过它——其文件不进 nodes/conceptIndex。
func TestBuildAuditGraphSkipsFeedDomain(t *testing.T) {
	base := t.TempDir()
	writeAuditTestFile(t, filepath.Join(base, "vault", "编程", "card.md"),
		"---\ntype: pkb_card\natomic_concept: 委托\nverification: verified\n---\n委托是方法的类型安全引用。\n")
	writeAuditTestFile(t, filepath.Join(base, "vault", "资讯", "编程", "2026-06-17.md"),
		"---\ntype: pkb_feed\natomic_concept: 不应进图\n---\n今日编程资讯综述。\n")

	c := &Curator{
		basePath: base,
		domains: &DomainsConfig{Domains: []Domain{
			{Name: "programming", Display: "编程", VaultSubpath: "vault/编程"},
			{Name: "news", Display: "最新资讯", VaultSubpath: "vault/资讯", Feed: true},
		}},
	}
	g := c.buildAuditGraph()

	assert.Contains(t, g.conceptIndex, "委托", "普通知识领域的卡应进 audit 图")
	assert.NotContains(t, g.conceptIndex, "不应进图", "feed 领域(资讯库)文件不应进 audit 图")
	for path := range g.nodes {
		assert.NotContains(t, path, filepath.Join("vault", "资讯"),
			"资讯库容器目录下的文件不应进 audit 图（领域级跳过）")
	}
}

// TestLoadDomainsParsesFeedFlag 守住 feed 字段从 domains.yaml 正确解析（默认 false，显式 true 生效）。
func TestLoadDomainsParsesFeedFlag(t *testing.T) {
	dir := t.TempDir()
	content := `defaults:
  vault_threshold: 7.0
domains:
  - name: programming
    display: 编程
    vault_subpath: vault/编程
  - name: news
    display: 最新资讯
    vault_subpath: vault/资讯
    feed: true
`
	path := filepath.Join(dir, "domains.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	dc, err := LoadDomains(path)
	if err != nil {
		t.Fatalf("LoadDomains: %v", err)
	}
	prog, ok := dc.FindDomain("programming")
	assert.True(t, ok)
	assert.False(t, prog.Feed, "普通领域 feed 默认应为 false")
	news, ok := dc.FindDomain("news")
	assert.True(t, ok)
	assert.True(t, news.Feed, "news 领域配置了 feed: true，应解析为 true")
}

func writeAuditTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
