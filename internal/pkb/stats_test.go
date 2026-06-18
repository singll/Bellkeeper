package pkb

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDomainStatsOverview(t *testing.T) {
	base := t.TempDir()
	domsPath := filepath.Join(base, "domains.yaml")
	doms := `defaults:
  vault_threshold: 7.0
domains:
  - name: programming
    display: 编程
    scope: 后端与全栈
    vault_subpath: vault/prog
    keywords: [go]
  - name: news
    display: 资讯
    vault_subpath: vault/news
    feed: true
`
	if err := os.WriteFile(domsPath, []byte(doms), 0644); err != nil {
		t.Fatal(err)
	}
	progDir := filepath.Join(base, "vault/prog")
	if err := os.MkdirAll(filepath.Join(progDir, "digest"), 0755); err != nil {
		t.Fatal(err)
	}

	index := "---\ntype: pkb_map\n---\n\n## 知识树\n\n- 根 [缺口]\n  - 子A [[卡A]]\n  - 子B [缺口]\n  - 子C [缺口]\n"
	mustWrite(t, filepath.Join(progDir, "_index.md"), index)
	mustWrite(t, filepath.Join(progDir, "_待归位.md"), "# 待归位\n- [[待A]]\n- [[待B]]\n")

	today := time.Now().Format("20060102")
	writeCard := func(name, date, verification string) {
		mustWrite(t, filepath.Join(progDir, name),
			"---\ntype: pkb_card\ningest_date: "+date+"\nverification: "+verification+"\n---\n\n正文\n")
	}
	writeCard("a.md", today, "verified")
	writeCard("b.md", today, "llm-only")        // 低置信 + 当天
	writeCard("c.md", "20200101", "unverified") // 低置信 + 旧
	mustWrite(t, filepath.Join(progDir, "topic.md"), "---\ntype: pkb_topic\n---\n") // 非卡，忽略
	mustWrite(t, filepath.Join(progDir, "digest", "d1.md"), "digest")

	stats, err := DomainStatsOverview(base, domsPath)
	if err != nil {
		t.Fatalf("DomainStatsOverview: %v", err)
	}
	prog := findStat(stats, "programming")
	if prog == nil {
		t.Fatal("programming 未返回")
	}
	checks := []struct {
		field string
		got   int
		want  int
	}{
		{"缺口数", prog.SkeletonGaps, 3},
		{"已挂节点", prog.SkeletonFilled, 1},
		{"待归位", prog.Waitlist, 2},
		{"卡片数", prog.Cards, 3},
		{"当天新增", prog.CardsToday, 2},
		{"近7天", prog.CardsWeek, 2},
		{"低置信", prog.LowConfidence, 2},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s want %d got %d", c.field, c.want, c.got)
		}
	}
	if !prog.HasSkeleton || !prog.HasScope || prog.LastDigestAt == "" {
		t.Errorf("HasSkeleton/HasScope/LastDigestAt 不符: %+v", prog)
	}

	news := findStat(stats, "news")
	if news == nil || !news.Feed || news.HasScope || news.HasSkeleton {
		t.Errorf("news(feed) 域状态不符: %+v", news)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写 %s: %v", path, err)
	}
}

func findStat(stats []DomainStat, name string) *DomainStat {
	for i := range stats {
		if stats[i].Name == name {
			return &stats[i]
		}
	}
	return nil
}
