package pkb

import (
	"strings"
	"testing"
)

// sampleSkeletonOutput 是与 config/pkb/prompts/skeleton.md 约定一致的代表性骨架输出。
// 用它钉死「骨架输出 ↔ validateDigestWithMode(digestModeRoot)」契约：
// 若 skeleton.md 漏写某个必需章节、或校验器改了必需集，该测试立刻报警。
const sampleSkeletonOutput = `---
title: C# 知识骨架
type: pkb_map
domain: programming
period: skeleton
generated_at: 2026-06-17T10:00:00+08:00
source_cards: 0
root_concepts: [语言基础, 异步编程, 内存与GC]
---

## 体系概览
C# 知识体系分为语言基础、异步编程、内存管理三大板块，语言基础是进入其余板块的前提。

## 知识树
- 语言基础 [缺口]
  - 类型系统 [缺口]
  - 泛型 [缺口]
- 异步编程 [缺口]
  - async/await [缺口]
- 内存与GC [缺口]

## 核心脉络
语言入门链：[[类型系统]] → [[泛型]] → [[async/await]] — 从静态类型到并发的递进。

## 新增与变化
骨架初次生成，全部节点为缺口，等待缺口填充与涌现卡归位。

## 缺口与探索方向
- 类型系统：语言地基，应最先填充，位于「语言基础」下。
- async/await：并发核心，位于「异步编程」下。
`

func TestSkeletonOutputPassesRootValidation(t *testing.T) {
	if err := validateDigestWithMode(sampleSkeletonOutput, digestModeRoot); err != nil {
		t.Errorf("代表性骨架输出未通过根索引校验（skeleton.md 与校验器契约漂移）: %v", err)
	}
}

func TestSkeletonOutputMissingSectionFails(t *testing.T) {
	broken := strings.Replace(sampleSkeletonOutput, "## 知识树", "## 这不是知识树", 1)
	if err := validateDigestWithMode(broken, digestModeRoot); err == nil {
		t.Error("缺「## 知识树」章节的骨架应校验失败，但通过了")
	}
}

func TestRunSkeletonRejectsDomainWithoutScope(t *testing.T) {
	c := &Curator{
		domains: &DomainsConfig{
			Domains: []Domain{{Name: "noscope", Display: "无方向", Scope: ""}},
		},
	}
	err := c.RunSkeleton(SkeletonOptions{Domain: "noscope"})
	if err == nil {
		t.Fatal("领域无 scope 时 RunSkeleton 应返回错误，但返回 nil")
	}
	if !strings.Contains(err.Error(), "scope") {
		t.Errorf("错误信息应提到 scope，实际: %v", err)
	}
}

func TestRunSkeletonRejectsUnknownDomain(t *testing.T) {
	c := &Curator{
		domains: &DomainsConfig{
			Domains: []Domain{{Name: "known", Display: "已知", Scope: "有方向"}},
		},
	}
	err := c.RunSkeleton(SkeletonOptions{Domain: "nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "unknown domain") {
		t.Errorf("未知领域应返回 unknown domain 错误，实际: %v", err)
	}
}

func TestParseSkeletonNodes(t *testing.T) {
	nodes := parseSkeletonNodes(sampleSkeletonOutput)
	want := []string{"语言基础", "类型系统", "泛型", "异步编程", "async/await", "内存与GC"}
	if len(nodes) != len(want) {
		t.Fatalf("parseSkeletonNodes 得到 %d 个节点 %v，期望 %d 个 %v", len(nodes), nodes, len(want), want)
	}
	for i := range want {
		if nodes[i] != want[i] {
			t.Errorf("节点[%d]=%q，期望 %q", i, nodes[i], want[i])
		}
	}
	// 核心脉络里的 [[类型系统]] 等不应被当成新节点（只读「## 知识树」章节）
	seen := map[string]int{}
	for _, n := range nodes {
		seen[n]++
	}
	if seen["类型系统"] != 1 {
		t.Errorf("「类型系统」应只出现一次（来自知识树，非核心脉络），实际 %d 次", seen["类型系统"])
	}
}

func TestRenderSkeletonWithCards(t *testing.T) {
	placed := map[string][]string{
		"泛型":   {"C#泛型约束"},
		"类型系统": {"值类型与引用类型", "可空引用类型"},
	}
	out := renderSkeletonWithCards(sampleSkeletonOutput, placed)

	if !strings.Contains(out, "- 泛型 [[C#泛型约束]]") {
		t.Errorf("泛型节点应挂上 [[C#泛型约束]]，实际输出:\n%s", out)
	}
	if !strings.Contains(out, "- 类型系统 [[值类型与引用类型]], [[可空引用类型]]") {
		t.Errorf("类型系统节点应挂上两张卡，实际输出:\n%s", out)
	}
	if !strings.Contains(out, "- 异步编程 [缺口]") {
		t.Error("无卡节点「异步编程」应保持 [缺口]")
	}
	// frontmatter 与其余章节不动
	if !strings.Contains(out, "type: pkb_map") {
		t.Error("frontmatter 被破坏：丢失 type: pkb_map")
	}
	if !strings.Contains(out, "## 核心脉络") || !strings.Contains(out, "## 缺口与探索方向") {
		t.Error("散文章节被破坏")
	}
}

func TestRenderSkeletonIdempotent(t *testing.T) {
	placed := map[string][]string{"泛型": {"C#泛型约束"}}
	once := renderSkeletonWithCards(sampleSkeletonOutput, placed)
	twice := renderSkeletonWithCards(once, placed)
	if once != twice {
		t.Errorf("归位渲染应幂等（已挂卡节点再渲染不变）。\n首次:\n%s\n二次:\n%s", once, twice)
	}
	// 第二次解析仍应得到全部 6 个节点（已填 [[..]] 节点不丢）
	if got := len(parseSkeletonNodes(once)); got != 6 {
		t.Errorf("已渲染骨架再解析应得 6 节点，实际 %d", got)
	}
}

func TestParseMatchJSON(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantLen int
		check   func(map[string]string) bool
	}{
		{
			"裸JSON数组",
			`[{"concept":"a","node":"X"},{"concept":"b","node":"待归位"}]`,
			2,
			func(m map[string]string) bool { return m["a"] == "X" && m["b"] == "待归位" },
		},
		{
			"代码块包裹",
			"```json\n[{\"concept\":\"泛型约束\",\"node\":\"泛型\"}]\n```",
			1,
			func(m map[string]string) bool { return m["泛型约束"] == "泛型" },
		},
		{
			"前后有解释文字",
			`这是结果：[{"concept":"x","node":"Y"}] 完毕`,
			1,
			func(m map[string]string) bool { return m["x"] == "Y" },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseMatchJSON(tc.in)
			if err != nil {
				t.Fatalf("parseMatchJSON 失败: %v", err)
			}
			if len(m) != tc.wantLen {
				t.Errorf("解析得 %d 项，期望 %d：%v", len(m), tc.wantLen, m)
			}
			if !tc.check(m) {
				t.Errorf("内容校验失败：%v", m)
			}
		})
	}

	if _, err := parseMatchJSON("没有任何 JSON 的输出"); err == nil {
		t.Error("无 JSON 数组的输出应返回错误")
	}
}
