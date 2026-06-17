package pkb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGapNodes(t *testing.T) {
	index := `---
title: 编程骨架
type: pkb_map
---

## 体系概览
略。

## 知识树
- 顶层A [缺口]
  - 子A1 [[已填卡]]
  - 子A2 [缺口]
    - 细分A2a [缺口]
- 顶层B [[卡X]], [[卡Y]]
- 顶层C [缺口]

## 核心脉络
略。`

	gaps := parseGapNodes(index)
	// 只取 [缺口] 节点：顶层A、子A2、细分A2a、顶层C；已挂卡的子A1/顶层B 不算缺口。
	got := make(map[string]int, len(gaps))
	for _, g := range gaps {
		got[g.Concept] = g.Depth
	}
	assert.Len(t, gaps, 4)
	assert.Contains(t, got, "顶层A")
	assert.Contains(t, got, "子A2")
	assert.Contains(t, got, "细分A2a")
	assert.Contains(t, got, "顶层C")
	assert.NotContains(t, got, "子A1")
	assert.NotContains(t, got, "顶层B")
	// 深度：顶层无缩进=0，子层 2 空格=2，细分 4 空格=4。
	assert.Equal(t, 0, got["顶层A"])
	assert.Equal(t, 2, got["子A2"])
	assert.Equal(t, 4, got["细分A2a"])
}

func TestSortGapsBreadthFirst(t *testing.T) {
	gaps := []gapNode{{Concept: "深", Depth: 4}, {Concept: "根", Depth: 0}, {Concept: "中", Depth: 2}}
	sortGapsBreadthFirst(gaps)
	assert.Equal(t, "根", gaps[0].Concept)
	assert.Equal(t, "中", gaps[1].Concept)
	assert.Equal(t, "深", gaps[2].Concept)
}

func TestParseGapDraft(t *testing.T) {
	out := `VOLATILITY: stable
CONFIDENCE: high
SOURCES: https://learn.microsoft.com/x, https://example.com/y
---CARD---
---
title: 测试概念
type: pkb_card
atomic_concept: 测试概念
---

## 定义与本质
一句话。`

	d := parseGapDraft(out)
	assert.Equal(t, "stable", d.volatility)
	assert.Equal(t, "high", d.confidence)
	assert.Equal(t, []string{"https://learn.microsoft.com/x", "https://example.com/y"}, d.sources)
	assert.True(t, strings.HasPrefix(d.card, "---\ntitle: 测试概念"))
	assert.Contains(t, d.card, "## 定义与本质")
	assert.NotContains(t, d.card, "VOLATILITY") // 头部不混入卡体
}

func TestParseSourceURLs(t *testing.T) {
	assert.Nil(t, parseSourceURLs("NONE"))
	assert.Nil(t, parseSourceURLs("  none  "))
	assert.Nil(t, parseSourceURLs(""))
	assert.Equal(t, []string{"https://a.com"}, parseSourceURLs("https://a.com, 不是url"))
	assert.Equal(t, []string{"https://a.com", "http://b.com"}, parseSourceURLs("https://a.com , http://b.com"))
}

func TestParseSupported(t *testing.T) {
	assert.True(t, parseSupported("SUPPORTED: yes\n因为页面讲了这个概念"))
	assert.True(t, parseSupported("supported: YES"))
	assert.False(t, parseSupported("SUPPORTED: no\n页面无关"))
	assert.False(t, parseSupported("模型没按格式输出")) // 解析不出=保守判不支撑
}

func TestHostOf(t *testing.T) {
	assert.Equal(t, "learn.microsoft.com", hostOf("https://learn.microsoft.com/zh-cn/dotnet/csharp/"))
	assert.Equal(t, "example.com", hostOf("http://example.com:8080/path"))
	assert.Equal(t, "", hostOf("   "))
}

func TestIndentDepth(t *testing.T) {
	assert.Equal(t, 0, indentDepth(""))
	assert.Equal(t, 2, indentDepth("  "))
	assert.Equal(t, 4, indentDepth("    "))
	assert.Equal(t, 2, indentDepth("\t")) // tab 记 2
}

func TestIsLowConfidence(t *testing.T) {
	assert.False(t, isLowConfidence("verified"))
	assert.True(t, isLowConfidence("unverified"))
	assert.True(t, isLowConfidence("llm-only"))
}

func TestGapCardSummary(t *testing.T) {
	card := `---
title: x
atomic_concept: x
---

## 定义与本质
这是定义。

## 关键细节
这是细节。

## 适用场景与边界
场景内容。`

	s := gapCardSummary(card)
	assert.Contains(t, s, "这是定义。")
	assert.Contains(t, s, "这是细节。")
	assert.NotContains(t, s, "场景内容。") // 摘要只取定义+关键细节，控上下文
}

// TestFinalizeGapCard 守住红线「缺口卡必带 source/verification/confidence，且 atomic_concept 锚回缺口名」：
// 落卡前覆盖核实结果与稳定字段，结果须通过 validateCard（与 reconstruct.v2 卡结构对齐）。
func TestFinalizeGapCard(t *testing.T) {
	draft := `---
title: 目标缺口
type: pkb_card
source: NONE
ingest_date: 20260101
score: 7.0
domains: programming
tags: [a, b]
atomic_concept: 模型起的旧名
aliases: []
card_type: definition
---

## 定义与本质
一句话定义。

## 关键细节
关键细节。

## 适用场景与边界
适用场景。

## 与其他知识的关系
（暂无关联）`

	dom := Domain{Name: "programming", Display: "编程"}
	card := finalizeGapCard(draft, "目标缺口", dom, "verified", "high", "https://learn.microsoft.com/x")

	assert.Contains(t, card, "atomic_concept: 目标缺口") // 强制锚回缺口名（归位必中）
	assert.NotContains(t, card, "模型起的旧名")
	assert.Contains(t, card, "verification: verified")
	assert.Contains(t, card, "confidence: high")
	assert.Contains(t, card, "source: https://learn.microsoft.com/x")
	assert.Contains(t, card, "pkb_gap_fill: true")
	// 落卡后必须仍是合法卡（含必需 frontmatter + v2 四章）
	assert.NoError(t, validateCard(card))
}

// TestGapFillEnabledFor 守住「全配置驱动」：每域开关优先，其次总开关，再退到新领域默认。
func TestGapFillEnabledFor(t *testing.T) {
	d := &Defaults{GapFillEnabled: map[string]bool{"programming": true, "ai": false}}

	assert.True(t, d.GapFillEnabledFor("programming")) // 显式开
	assert.False(t, d.GapFillEnabledFor("ai"))         // 显式关

	// 显式配置优先于总开关：ai 仍关
	d.GapFillEnabledAll = boolPtr(true)
	assert.False(t, d.GapFillEnabledFor("ai"))
	assert.True(t, d.GapFillEnabledFor("security")) // 未列出 → 总开关 true

	// 总开关关 + 默认关 → 未列出领域关
	d.GapFillEnabledAll = boolPtr(false)
	d.GapFillDefault = boolPtr(false)
	assert.False(t, d.GapFillEnabledFor("security"))

	// 默认开 → 未列出领域开
	d.GapFillDefault = boolPtr(true)
	assert.True(t, d.GapFillEnabledFor("security"))
}

// TestFinalizeCrawledGapCard 守住 F2 路径：定向爬原子化卡标 verified（基于真实抓取），
// 第一张锚回缺口名（归位必中），补充卡保留自身概念；落卡后仍是合法卡。
func TestFinalizeCrawledGapCard(t *testing.T) {
	card := `---
title: 抓取得到的概念
type: pkb_card
source: https://old
ingest_date: 20250101
score: 8.0
domains: programming
tags: [x]
atomic_concept: 抓取得到的概念
aliases: []
card_type: definition
---

## 定义与本质
定义。

## 关键细节
细节。

## 适用场景与边界
场景。

## 与其他知识的关系
（暂无关联）`

	dom := Domain{Name: "programming"}

	// 第一张卡：anchor=缺口名 → 强制改 atomic_concept（归位必中）
	first := finalizeCrawledGapCard(card, "目标缺口", dom, "https://new")
	assert.Contains(t, first, "atomic_concept: 目标缺口")
	assert.NotContains(t, first, "atomic_concept: 抓取得到的概念")
	assert.Contains(t, first, "verification: verified")
	assert.Contains(t, first, "confidence: high")
	assert.Contains(t, first, "source: https://new")
	assert.Contains(t, first, "pkb_gap_fill: true")
	assert.NoError(t, validateCard(first))

	// 补充卡：anchor 空 → 保留自身概念
	supp := finalizeCrawledGapCard(card, "", dom, "https://new")
	assert.Contains(t, supp, "atomic_concept: 抓取得到的概念")
	assert.Contains(t, supp, "verification: verified")
	assert.NoError(t, validateCard(supp))
}
