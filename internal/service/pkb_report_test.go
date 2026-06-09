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
