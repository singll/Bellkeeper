package pkb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func listMD(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
			out = append(out, e.Name())
		}
	}
	return out
}

// TestNormalizeMapFrontmatter 复刻线上「安全/_index.md」三个真实 bug：缺闭合 ---、
// generated_at 幻觉未来时间、末尾误抄「## 元信息」——验证落盘前规整全部修正。
func TestNormalizeMapFrontmatter(t *testing.T) {
	now := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	dom := Domain{Name: "security", Display: "网络安全"}

	in := `---
title: 攻防安全目标知识索引
type: pkb_map
domain: security
generated_at: 2026-08-10T03:00:17+08:00
root_concepts: [Web 安全]

## 体系概览
本领域以七大根域为顶层骨架。

## 知识树
- Web 安全 [缺口]

## 元信息
领域：网络安全（security）
生成时间：2026-08-10T03:00:17+08:00`

	out := normalizeMapFrontmatter(in, dom, now)

	if !hasClosedFrontmatter(out) {
		t.Errorf("frontmatter 未补闭合:\n%s", out)
	}
	if strings.Contains(out, "2026-08-10") {
		t.Errorf("generated_at 幻觉未来时间未被覆盖:\n%s", out)
	}
	if !strings.Contains(out, "generated_at: "+now.Format(time.RFC3339)) {
		t.Errorf("generated_at 未强制为 now:\n%s", out)
	}
	if strings.Contains(out, "## 元信息") {
		t.Errorf("误抄的「## 元信息」段未剥离:\n%s", out)
	}
	if !strings.Contains(out, "## 知识树") || !strings.Contains(out, "## 体系概览") {
		t.Errorf("合法章节被误删:\n%s", out)
	}
	if !strings.Contains(out, "domain: security") {
		t.Errorf("domain 丢失:\n%s", out)
	}
}

// TestEnsureFrontmatterClosed 校验闭合补齐的边界：已闭合不动、缺闭合按首个空行/标题补。
func TestEnsureFrontmatterClosed(t *testing.T) {
	closed := "---\ntitle: x\n---\n\n## 正文\n内容"
	if got := ensureFrontmatterClosed(closed); got != closed {
		t.Errorf("已闭合不应改动:\n%s", got)
	}

	// 缺闭合、以空行分隔正文
	missing := "---\ntitle: x\ndomain: ai\n\n## 正文"
	got := ensureFrontmatterClosed(missing)
	if !hasClosedFrontmatter(got) {
		t.Errorf("缺闭合未补:\n%s", got)
	}
	if !strings.Contains(got, "domain: ai\n---") {
		t.Errorf("--- 未补在 frontmatter 末尾:\n%s", got)
	}

	// 无 frontmatter 不处理
	none := "# 普通标题\n正文"
	if got := ensureFrontmatterClosed(none); got != none {
		t.Errorf("无 frontmatter 不应改动:\n%s", got)
	}
}

// TestSnapshotIndexIncremental 验证增量快照：只存结构+增量段、结构不变去重、结构变化产新份。
func TestSnapshotIndexIncremental(t *testing.T) {
	tmp := t.TempDir()
	sub := "vault/AI"
	dir := filepath.Join(tmp, sub)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "_index.md")
	digestDir := filepath.Join(dir, "digest")

	index := `---
title: AI
type: pkb_map
domain: ai
root_concepts: [A, B]
---

## 体系概览
一大段散文不该进快照。

## 知识树
- A [[卡1]]
- B [缺口]

## 新增与变化
新增卡1。

## 缺口与探索方向
略。`
	mustWrite(t, indexPath, index)

	// 首次快照：只含结构+增量段，不含散文全文。
	if err := snapshotIndexIncremental(tmp, sub, 5, false); err != nil {
		t.Fatal(err)
	}
	snaps := listMD(digestDir)
	if len(snaps) != 1 {
		t.Fatalf("首次应产 1 份快照，实际 %d", len(snaps))
	}
	body, _ := os.ReadFile(filepath.Join(digestDir, snaps[0]))
	sb := string(body)
	if !strings.Contains(sb, "## 知识树") || !strings.Contains(sb, "## 新增与变化") {
		t.Errorf("快照缺结构/增量段:\n%s", sb)
	}
	if strings.Contains(sb, "体系概览") || strings.Contains(sb, "缺口与探索方向") {
		t.Errorf("快照不应含散文全文:\n%s", sb)
	}

	// 结构不变再快照 → 去重跳过（即便 weekly=false）。
	if err := snapshotIndexIncremental(tmp, sub, 5, false); err != nil {
		t.Fatal(err)
	}
	if n := len(listMD(digestDir)); n != 1 {
		t.Errorf("结构不变应去重，实际 %d 份", n)
	}

	// 知识树结构变化 → 去重不跳过，最新快照应反映新结构（卡2/C）。
	index2 := strings.Replace(index, "- B [缺口]", "- B [[卡2]]\n- C [缺口]", 1)
	mustWrite(t, indexPath, index2)
	if err := snapshotIndexIncremental(tmp, sub, 5, false); err != nil {
		t.Fatal(err)
	}
	latest := latestSnapshotContent(digestDir)
	if !strings.Contains(latest, "C [缺口]") || !strings.Contains(latest, "卡2") {
		t.Errorf("结构变化后最新快照未反映新结构:\n%s", latest)
	}
}

// TestPruneSnapshots 验证滚动保留最新 keepN 份。
func TestPruneSnapshots(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, fmt.Sprintf("snap%d.md", i))
		mustWrite(t, p, "x")
		mt := time.Now().Add(time.Duration(i) * time.Hour) // mtime 递增，snap3 最新
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	pruneSnapshots(dir, 2)
	left := listMD(dir)
	if len(left) != 2 {
		t.Fatalf("应保留 2 份，实际 %d：%v", len(left), left)
	}
	for _, name := range left {
		if name == "snap0.md" || name == "snap1.md" {
			t.Errorf("应删最旧份，却保留 %s", name)
		}
	}
}
