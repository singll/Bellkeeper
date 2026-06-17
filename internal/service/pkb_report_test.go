package service

import (
	"os"
	"path/filepath"
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
