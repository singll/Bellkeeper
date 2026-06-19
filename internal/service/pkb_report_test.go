package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/singll/bellkeeper/internal/config"
)

func TestPKBReportServiceVaultCardsByDateSkipsDailyAndDigest(t *testing.T) {
	base := t.TempDir()
	writeTestFile(t, filepath.Join(base, "vault", "编程", "card.md"), `---
title: Good Card
pkb_score: final=8.3
ingest_date: 2026-06-09
tags: [go, pkb]
---

This is a useful card.
`)
	writeTestFile(t, filepath.Join(base, "vault", "daily", "2026-06-09.md"), `---
title: Daily Report
---

Should not be treated as a PKB card.
`)
	writeTestFile(t, filepath.Join(base, "vault", "编程", "digest", "weekly.md"), `---
title: Weekly Digest
pkb_score: final=9.0
ingest_date: 2026-06-09
---

Digest should be excluded from cards.
`)

	svc := NewPKBReportService(config.KnowledgeConfig{BasePath: base}, config.DailyReportConfig{}, nil)
	cards, err := svc.VaultCardsByDate("2026-06-09", 10)
	if err != nil {
		t.Fatalf("VaultCardsByDate returned error: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("cards len = %d, want 1: %+v", len(cards), cards)
	}
	if cards[0].Title != "Good Card" {
		t.Fatalf("card title = %q, want Good Card", cards[0].Title)
	}
	if cards[0].RelPath != "vault/编程/card.md" {
		t.Fatalf("card rel_path = %q", cards[0].RelPath)
	}
}

func TestPKBReportServiceLatestDigests(t *testing.T) {
	base := t.TempDir()
	writeTestFile(t, filepath.Join(base, "vault", "编程", "digest", "weekly.md"), `---
title: Weekly Digest
domain: 编程
period: weekly
generated_at: 2026-06-09T21:00:00Z
---

Digest.
`)

	svc := NewPKBReportService(config.KnowledgeConfig{BasePath: base}, config.DailyReportConfig{}, nil)
	digests, err := svc.LatestDigests()
	if err != nil {
		t.Fatalf("LatestDigests returned error: %v", err)
	}
	if len(digests) != 1 {
		t.Fatalf("digests len = %d, want 1", len(digests))
	}
	if digests[0].RelPath != "vault/编程/digest/weekly.md" {
		t.Fatalf("digest rel_path = %q", digests[0].RelPath)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestPKBReportServiceExcludesFeedArchive 守住 ADR-0005：资讯库容器目录 vault/资讯 下的分领域
// 分日存档不是知识原子卡——既不计入 VaultCardsByDate（今日新卡），也不计入 VaultStats（总卡/知识树）。
func TestPKBReportServiceExcludesFeedArchive(t *testing.T) {
	base := t.TempDir()
	writeTestFile(t, filepath.Join(base, "vault", "编程", "card.md"), `---
title: 委托
pkb_score: final=8.3
ingest_date: 2026-06-17
---

委托卡。
`)
	// 资讯库存档（type: pkb_feed，无 pkb_score）——落在 vault/资讯/<领域>/<日期>.md
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "编程", "2026-06-17.md"), `---
title: 编程资讯 2026-06-17
type: pkb_feed
ingest_date: 2026-06-17
---

今日编程资讯综述。
`)

	svc := NewPKBReportService(config.KnowledgeConfig{BasePath: base}, config.DailyReportConfig{}, nil)

	cards, err := svc.VaultCardsByDate("2026-06-17", 10)
	if err != nil {
		t.Fatalf("VaultCardsByDate returned error: %v", err)
	}
	if len(cards) != 1 || cards[0].Title != "委托" {
		t.Fatalf("应只含知识卡「委托」（资讯库存档被排除），实得 %+v", cards)
	}

	stats, err := svc.VaultStats()
	if err != nil {
		t.Fatalf("VaultStats returned error: %v", err)
	}
	// VaultStats.Cards 的遍历不读 frontmatter，仅靠目录排除——Cards==1 真正验证 vault/资讯 被排除。
	if stats.Cards != 1 {
		t.Fatalf("知识卡总数应为 1（资讯库存档不计），实得 %d", stats.Cards)
	}
	if stats.Trees != 1 {
		t.Fatalf("知识树数应为 1（编程；资讯库容器不计），实得 %d", stats.Trees)
	}
}

// TestFeedArchivesByDate 守住日报联动：列出当日各领域资讯存档 vault/资讯/<领域>/<date>.md，按领域名排序。
func TestFeedArchivesByDate(t *testing.T) {
	base := t.TempDir()
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "编程", "2026-06-17.md"), "---\ntype: pkb_feed\n---\n编程资讯。\n")
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "安全", "2026-06-17.md"), "---\ntype: pkb_feed\n---\n安全资讯。\n")
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "编程", "2026-06-16.md"), "---\ntype: pkb_feed\n---\n昨天的资讯。\n")

	svc := NewPKBReportService(config.KnowledgeConfig{BasePath: base}, config.DailyReportConfig{}, nil)
	archives, err := svc.FeedArchivesByDate("2026-06-17")
	if err != nil {
		t.Fatalf("FeedArchivesByDate returned error: %v", err)
	}
	if len(archives) != 2 {
		t.Fatalf("当日资讯存档应为 2（编程+安全，不含 6-16），实得 %d: %+v", len(archives), archives)
	}
	if archives[0].Domain != "安全" {
		t.Fatalf("应按领域名排序，archives[0]=%q want 安全", archives[0].Domain)
	}
	if archives[1].RelPath != "vault/资讯/编程/2026-06-17.md" {
		t.Fatalf("编程存档相对路径 = %q", archives[1].RelPath)
	}
}

// TestFeedTimeline 守住资讯时间线：自 before 前一天起往前数天，仅保留有存档的日子，按日期降序。
func TestFeedTimeline(t *testing.T) {
	base := t.TempDir()
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "编程", "2026-06-17.md"), "---\ntype: pkb_feed\n---\n编程资讯。\n")
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "安全", "2026-06-17.md"), "---\ntype: pkb_feed\n---\n安全资讯。\n")
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "编程", "2026-06-16.md"), "---\ntype: pkb_feed\n---\n昨天的资讯。\n")

	svc := NewPKBReportService(config.KnowledgeConfig{BasePath: base}, config.DailyReportConfig{}, nil)
	days, err := svc.FeedTimeline(7, "2026-06-18") // 自 2026-06-17 往前 7 天
	if err != nil {
		t.Fatalf("FeedTimeline returned error: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("应有 2 天有存档（06-17、06-16），实得 %d: %+v", len(days), days)
	}
	if days[0].Date != "2026-06-17" || len(days[0].Archives) != 2 {
		t.Fatalf("days[0] 应为 06-17 含 2 篇，实得 %+v", days[0])
	}
	if days[1].Date != "2026-06-16" || len(days[1].Archives) != 1 {
		t.Fatalf("days[1] 应为 06-16 含 1 篇，实得 %+v", days[1])
	}
}

// TestFeedArchiveHTML 守住单篇资讯只读渲染：剥 frontmatter、取首个一级标题、goldmark 渲染、
// 并拒绝路径穿越（ADR-0006 仅限资讯库存档）。
func TestFeedArchiveHTML(t *testing.T) {
	base := t.TempDir()
	writeTestFile(t, filepath.Join(base, "vault", "资讯", "编程", "2026-06-17.md"), "---\ntype: pkb_feed\n---\n# 编程速览\n\n- 要点一\n- 要点二\n")

	svc := NewPKBReportService(config.KnowledgeConfig{BasePath: base}, config.DailyReportConfig{}, nil)

	got, err := svc.FeedArchiveHTML("2026-06-17", "编程")
	if err != nil {
		t.Fatalf("FeedArchiveHTML returned error: %v", err)
	}
	if got.Title != "编程速览" {
		t.Fatalf("Title = %q, want 编程速览", got.Title)
	}
	if !strings.Contains(got.HTML, "<li>") || !strings.Contains(got.HTML, "要点一") {
		t.Fatalf("HTML 应含渲染后的列表，实得 %q", got.HTML)
	}
	if strings.Contains(got.HTML, "type: pkb_feed") || strings.Contains(got.HTML, "---") {
		t.Fatalf("HTML 不应残留 frontmatter，实得 %q", got.HTML)
	}

	// 路径穿越 / 非法 domain 必须被拒
	for _, bad := range []string{"../编程", "编程/../资讯", `编程\x`, ""} {
		if _, err := svc.FeedArchiveHTML("2026-06-17", bad); err == nil {
			t.Fatalf("domain=%q 应被拒", bad)
		}
	}
	// 非法日期
	if _, err := svc.FeedArchiveHTML("2026/06/17", "编程"); err == nil {
		t.Fatalf("非法日期应被拒")
	}
	// 不存在的存档返回 os.ErrNotExist（供 handler 转 404）
	if _, err := svc.FeedArchiveHTML("2026-06-17", "不存在域"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("不存在存档应返回 os.ErrNotExist，实得 %v", err)
	}
}
