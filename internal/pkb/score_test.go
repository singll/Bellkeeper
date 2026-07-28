package pkb

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestFinalScoreContentTypeAndAtomic 验证结构性改造后的 FinalScore：
// content_type 加减分配置化（可覆盖内置默认）+ atomic_potential 达标上浮（修死字段/防漏召）。
func TestFinalScoreContentTypeAndAtomic(t *testing.T) {
	// 只给 relevance 权重 1.0，其余 0 → 加权和 = relevance 值，便于手算避免浮点噪声。
	def := Defaults{
		Weights:                 Weights{Relevance: 1.0},
		AtomicPotentialBonus:    0.3,
		AtomicPotentialBonusMin: 8,
		// ContentTypeAdjust 留 nil → FinalScore 回退 defaultContentTypeAdjust
	}
	base := ScoreResult{Relevance: 5}

	cases := []struct {
		name   string
		ct     string
		atomic int
		adjust map[string]float64
		want   float64
	}{
		{"reference 内置+0.5", "reference", 0, nil, 5.5},
		{"marketing 内置-2.0", "marketing", 0, nil, 3.0},
		{"未知类型不调整", "misc", 0, nil, 5.0},
		{"atomic 达标上浮+0.3", "misc", 9, nil, 5.3},
		{"atomic 未达标不上浮", "misc", 7, nil, 5.0},
		{"配置覆盖内置(news 0)", "news", 0, map[string]float64{"news": 0}, 5.0},
		{"配置覆盖内置(code -1)", "code", 0, map[string]float64{"code": -1.0}, 4.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := def
			d.ContentTypeAdjust = tc.adjust
			sr := base
			sr.ContentType = tc.ct
			sr.AtomicPotential = tc.atomic
			if got := sr.FinalScore(d); !approxEqual(got, tc.want) {
				t.Errorf("FinalScore = %.4f, want %.4f", got, tc.want)
			}
		})
	}
}

// TestDecideGateQuota 验证决策分流：相关度硬地板/相关度门/常规阈值/领域配额。
func TestDecideGateQuota(t *testing.T) {
	dc := &DomainsConfig{
		Defaults: Defaults{
			VaultThreshold: 7, ArchiveThreshold: 4,
			RelevanceGate: 5, RelevanceHardFloor: 3,
		},
		Domains: []Domain{{Name: "ai", Display: "AI", VaultSubpath: "vault/AI"}},
	}
	c := &Curator{domains: dc, vaultCount: map[string]int{}}
	dom := dc.Domains[0]

	cases := []struct {
		name         string
		rel          int
		final        float64
		wantDecision string
		wantGate     string
	}{
		{"硬地板：离题直接弃(即便高分)", 2, 9.0, "discard", "hard_floor"},
		{"相关度门：达vault线但离题→降级", 4, 8.0, "archive", "gate"},
		{"正常入库", 8, 8.0, "vault", ""},
		{"常规archive(4~7)", 8, 5.0, "archive", ""},
		{"常规discard(<4)", 8, 3.0, "discard", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, g := c.decide(&ScoreResult{Relevance: tc.rel}, tc.final, dom)
			if d != tc.wantDecision || g != tc.wantGate {
				t.Errorf("decide = (%s,%s), want (%s,%s)", d, g, tc.wantDecision, tc.wantGate)
			}
		})
	}

	// 领域配额：配额 1、本轮已满 → 高分卡降级 archive/quota。
	domQuota := Domain{Name: "ai", VaultQuotaPerRun: 1}
	c.vaultCount["ai"] = 1
	if d, g := c.decide(&ScoreResult{Relevance: 8}, 8.0, domQuota); d != "archive" || g != "quota" {
		t.Errorf("quota: got (%s,%s), want (archive,quota)", d, g)
	}
	// 配额未满 → 正常入库。
	c.vaultCount["ai"] = 0
	if d, g := c.decide(&ScoreResult{Relevance: 8}, 8.0, domQuota); d != "vault" || g != "" {
		t.Errorf("quota-open: got (%s,%s), want (vault,)", d, g)
	}
}

// TestLedgerCell 验证台账单元格清理（| 与换行会破坏 Markdown 表格）。
func TestLedgerCell(t *testing.T) {
	cases := map[string]string{
		"a|b":          "a／b",
		"line1\nline2": "line1 line2",
		"  x  ":        "x",
		"正常":           "正常",
	}
	for in, want := range cases {
		if got := ledgerCell(in); got != want {
			t.Errorf("ledgerCell(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAppendReviewLedger 验证拒收台账：被拒/降级条目落账、标题 | 转义、vault 不记、表头一次。
func TestAppendReviewLedger(t *testing.T) {
	tmp := t.TempDir()
	dc := &DomainsConfig{Defaults: Defaults{ReviewLedgerEnabled: boolPtr(true)}, Domains: []Domain{{Name: "ai"}}}
	c := &Curator{basePath: tmp, domains: dc}
	art := ArticleMeta{Title: "离题|标题", URL: "http://x"}
	sr := &ScoreResult{Relevance: 2, Depth: 8, ContentType: "reference", AtomicPotential: 5}

	c.appendReviewLedger(art, sr, 7.7, dc.Domains[0], "discard", "hard_floor")
	c.appendReviewLedger(art, sr, 8.0, dc.Domains[0], "vault", "") // vault 不记

	path := filepath.Join(tmp, "vault", "_拒收台账", time.Now().Format("2006-01")+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("台账未生成: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "hard_floor") {
		t.Error("缺 hard_floor 行")
	}
	if !strings.Contains(content, "离题／标题") {
		t.Error("标题内 | 未转义为 ／")
	}
	if n := strings.Count(content, "| discard |"); n != 1 {
		t.Errorf("应恰好 1 条 discard（vault 不记），实际 %d 条\n%s", n, content)
	}
	if n := strings.Count(content, "# 拒收台账"); n != 1 {
		t.Errorf("表头应只写一次，实际 %d 次", n)
	}

	// 关闭台账时不落账。
	c2 := &Curator{basePath: t.TempDir(), domains: &DomainsConfig{Defaults: Defaults{ReviewLedgerEnabled: boolPtr(false)}}}
	c2.appendReviewLedger(art, sr, 3.0, dc.Domains[0], "discard", "")
	if _, err := os.Stat(filepath.Join(c2.basePath, "vault", "_拒收台账")); !os.IsNotExist(err) {
		t.Error("台账关闭时不应创建目录")
	}
}
