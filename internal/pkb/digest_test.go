package pkb

import (
	"strings"
	"testing"
	"time"
)

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
