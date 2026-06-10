package pkb

import (
	"strings"
	"testing"
)

func TestSplitCards(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"单卡无分隔符", "---\ntitle: A\n---\nbody", 1},
		{"双卡带换行分隔符", "---\ntitle: A\n---\nbody1\n---CARD---\n---\ntitle: B\n---\nbody2", 2},
		{"三卡", "card1\n---CARD---\ncard2\n---CARD---\ncard3", 3},
		{"尾部多余分隔符", "card1\n---CARD---\n", 1},
		{"纯空内容被跳过", "\n---CARD---\n  \n---CARD---\ncard3", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCards(tc.input)
			if len(got) != tc.want {
				t.Errorf("splitCards(%q) returned %d cards, want %d", tc.input, len(got), tc.want)
			}
		})
	}
}

func TestBuildValidLinkSet(t *testing.T) {
	vault := []string{"概念A", "概念B"}
	batch := []string{"概念C"}
	cards := []string{
		"---\ntitle: X\natomic_concept: 概念C\naliases: [别名C]\n---\nbody",
	}
	result := buildValidLinkSet(vault, batch, cards)
	seen := make(map[string]bool)
	for _, r := range result {
		seen[r] = true
	}
	for _, want := range []string{"概念A", "概念B", "概念C", "别名C"} {
		if !seen[want] {
			t.Errorf("buildValidLinkSet missing %q in result %v", want, result)
		}
	}
}

func TestAppendAlias(t *testing.T) {
	cases := []struct {
		name    string
		card    string
		alias   string
		wantHas string
	}{
		{
			"无aliases追加",
			"---\ntitle: X\natomic_concept: 概念A\n---\nbody",
			"旧文件名",
			"[旧文件名]",
		},
		{
			"空aliases追加",
			"---\ntitle: X\naliases: []\n---\nbody",
			"旧文件名",
			"[旧文件名]",
		},
		{
			"已有aliases追加不覆盖",
			"---\ntitle: X\naliases: [别名1, 别名2]\n---\nbody",
			"旧文件名",
			"[别名1, 别名2, 旧文件名]",
		},
		{
			"重复不追加",
			"---\ntitle: X\naliases: [别名1]\n---\nbody",
			"别名1",
			"[别名1]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := appendAlias(tc.card, tc.alias)
			if !strings.Contains(result, tc.wantHas) {
				t.Errorf("appendAlias result = %q, want to contain %q", result, tc.wantHas)
			}
			if tc.name == "已有aliases追加不覆盖" {
				if !strings.Contains(result, "别名1") || !strings.Contains(result, "别名2") {
					t.Errorf("appendAlias overwrote existing aliases: %q", result)
				}
			}
		})
	}
}

func TestCollectTopicCards(t *testing.T) {
	allCards := []digestCard{
		{Title: "JWT攻击", AtomicConcept: "JWT算法混淆攻击"},
		{Title: "XSS防护", AtomicConcept: "XSS防护策略"},
		{Title: "SQL注入", AtomicConcept: "SQL注入防御"},
	}
	c := &Curator{}
	matched := c.collectTopicCards(Domain{}, "JWT", allCards)
	if len(matched) != 1 || matched[0].AtomicConcept != "JWT算法混淆攻击" {
		t.Errorf("collectTopicCards JWT = %v, want 1 card with JWT算法混淆攻击", matched)
	}
	zero := c.collectTopicCards(Domain{}, "不存在的主题", allCards)
	if len(zero) != 0 {
		t.Errorf("collectTopicCards 不存在的主题 = %d cards, want 0", len(zero))
	}
}

func TestRemoveEmptySection(t *testing.T) {
	prompt := `## 体系概览
一些概览内容

## 已有体系结构（请在此基础上增量更新，不要从零重写）

## 知识树
树内容`
	result := removeEmptySection(prompt, "## 已有体系结构")
	if strings.Contains(result, "已有体系结构") {
		t.Errorf("removeEmptySection did not remove section: %q", result)
	}
	if !strings.Contains(result, "## 体系概览") {
		t.Errorf("removeEmptySection removed wrong section: %q", result)
	}
	if !strings.Contains(result, "## 知识树") {
		t.Errorf("removeEmptySection removed ## 知识树: %q", result)
	}
}

func TestPruneWikilinks(t *testing.T) {
	card := "正文 [[概念A]] 和 [[概念B]] 和 [[概念C]] 结尾"
	valid := []string{"概念A", "概念B"}
	result := pruneWikilinks(card, valid)
	if strings.Contains(result, "[[概念A]]") && !strings.Contains(result, "[[概念B]]") {
	} else if !strings.Contains(result, "[[概念A]]") {
		t.Errorf("pruneWikilinks removed valid link 概念A: %q", result)
	}
	if strings.Contains(result, "[[概念C]]") {
		t.Errorf("pruneWikilinks kept invalid link 概念C: %q", result)
	}
	if !strings.Contains(result, "概念C") {
		t.Errorf("pruneWikilinks removed 概念C text entirely: %q", result)
	}
}

func TestMergeCandidates(t *testing.T) {
	titles := []string{"概念A"}
	content := []ContentMatch{
		{Concept: "概念B", Excerpt: "摘要B"},
		{Concept: "概念A", Excerpt: "摘要A"},
	}
	concepts, display := mergeCandidates(titles, content)
	if len(concepts) != 2 {
		t.Errorf("mergeCandidates concepts = %d, want 2 (deduped)", len(concepts))
	}
	foundB := false
	for _, d := range display {
		if strings.Contains(d, "概念B") && strings.Contains(d, "摘要B") {
			foundB = true
		}
	}
	if !foundB {
		t.Errorf("mergeCandidates display missing excerpt for 概念B: %v", display)
	}
	hasExcerptInConcepts := false
	for _, c := range concepts {
		if strings.Contains(c, "摘要") {
			hasExcerptInConcepts = true
		}
	}
	if hasExcerptInConcepts {
		t.Errorf("mergeCandidates concepts should not contain excerpts: %v", concepts)
	}
}
