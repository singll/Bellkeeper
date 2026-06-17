package pkb

import (
	"os"
	"path/filepath"
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

// 归位在「无骨架」领域上必须 no-op 返回 nil——这是 digest 接入归位后旧领域不崩的保证。
func TestPlaceCardsOntoSkeletonNoOpWithoutIndex(t *testing.T) {
	tmp := t.TempDir()
	domSub := filepath.Join("vault", "test")
	if err := os.MkdirAll(filepath.Join(tmp, domSub), 0755); err != nil {
		t.Fatal(err)
	}
	c := &Curator{basePath: tmp, domains: &DomainsConfig{}}
	dom := Domain{Name: "test", Display: "测试", VaultSubpath: domSub}
	if err := c.placeCardsOntoSkeleton(dom, false, false); err != nil {
		t.Errorf("无 _index.md 应 no-op 返回 nil，实际: %v", err)
	}
}

func TestPlaceCardsOntoSkeletonNoOpEmptyTree(t *testing.T) {
	tmp := t.TempDir()
	domSub := filepath.Join("vault", "test")
	dir := filepath.Join(tmp, domSub)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// _index.md 存在但无「## 知识树」节点
	noTree := "---\ntitle: x\ntype: pkb_map\n---\n\n## 体系概览\n暂无结构\n"
	if err := os.WriteFile(filepath.Join(dir, "_index.md"), []byte(noTree), 0644); err != nil {
		t.Fatal(err)
	}
	c := &Curator{basePath: tmp, domains: &DomainsConfig{}}
	dom := Domain{Name: "test", VaultSubpath: domSub}
	if err := c.placeCardsOntoSkeleton(dom, false, false); err != nil {
		t.Errorf("无骨架节点应 no-op 返回 nil，实际: %v", err)
	}
}

func TestExtractAndReplaceSection(t *testing.T) {
	tree := extractSection(sampleSkeletonOutput, "## 知识树")
	if !strings.Contains(tree, "- 语言基础 [缺口]") || strings.Contains(tree, "## 核心脉络") {
		t.Errorf("extractSection 未正确截取知识树正文:\n%s", tree)
	}
	newTree := "- 全新节点A [缺口]\n- 全新节点B [缺口]"
	out := replaceSection(sampleSkeletonOutput, "## 知识树", newTree)
	if !strings.Contains(out, "- 全新节点A [缺口]") {
		t.Error("replaceSection 未写入新树")
	}
	if strings.Contains(out, "- 语言基础 [缺口]") {
		t.Error("replaceSection 未清除旧树")
	}
	if !strings.Contains(out, "## 核心脉络") || !strings.Contains(out, "type: pkb_map") {
		t.Error("replaceSection 破坏了其它章节/frontmatter")
	}
	// 找不到的标题原样返回
	if got := replaceSection(sampleSkeletonOutput, "## 不存在", "x"); got != sampleSkeletonOutput {
		t.Error("replaceSection 对不存在标题应原样返回")
	}
}

func TestComputeImpactRadius(t *testing.T) {
	// 当前骨架：泛型挂 2 卡、类型系统挂 1 卡
	current := "## 知识树\n- 语言基础 [缺口]\n  - 类型系统 [[卡A]]\n  - 泛型 [[卡B]], [[卡C]]\n\n## 核心脉络\nx\n"
	// 纯加节点：现有节点都在 → 影响半径 0
	addOnly := "- 语言基础 [缺口]\n  - 类型系统 [缺口]\n  - 泛型 [缺口]\n  - 新节点 [缺口]"
	if r := computeImpactRadius(current, addOnly); r != 0 {
		t.Errorf("纯加节点影响半径应为 0，实际 %d", r)
	}
	// 删除「泛型」（挂了 2 卡）→ 影响半径 2
	dropGeneric := "- 语言基础 [缺口]\n  - 类型系统 [缺口]"
	if r := computeImpactRadius(current, dropGeneric); r != 2 {
		t.Errorf("删除挂 2 卡的节点影响半径应为 2，实际 %d", r)
	}
}

func TestParseProposal(t *testing.T) {
	out := "ACTION: add\nSUMMARY: 把并发相关待归位卡归并为新节点\nTREE:\n- 语言基础 [缺口]\n  - 并发原语 [缺口]\n"
	p := parseProposal("programming", out)
	if p.Action != "add" {
		t.Errorf("action=%q，期望 add", p.Action)
	}
	if !strings.Contains(p.Summary, "并发") {
		t.Errorf("summary 解析错误: %q", p.Summary)
	}
	if !strings.Contains(p.ProposedTree, "并发原语 [缺口]") || strings.Contains(p.ProposedTree, "ACTION:") {
		t.Errorf("proposedTree 解析错误:\n%s", p.ProposedTree)
	}
}

func TestProposalApplyRejectRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	domSub := filepath.Join("vault", "prog")
	dir := filepath.Join(tmp, domSub)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "_index.md")
	index := "---\ntype: pkb_map\n---\n\n## 体系概览\n概览\n\n## 知识树\n- 旧节点 [缺口]\n\n## 核心脉络\n脉络\n"
	if err := os.WriteFile(indexPath, []byte(index), 0644); err != nil {
		t.Fatal(err)
	}

	// 保存一条大动作提议
	p := SkeletonProposal{
		ID:           "prog-20260617-120000",
		Domain:       "programming",
		VaultSubpath: domSub,
		Action:       "restructure",
		Summary:      "重排",
		ProposedTree: "- 新节点甲 [缺口]\n- 新节点乙 [缺口]",
		ImpactRadius: 9,
	}
	if err := saveProposalFile(tmp, p); err != nil {
		t.Fatalf("saveProposalFile: %v", err)
	}
	props, err := ListPendingProposals(tmp)
	if err != nil || len(props) != 1 || props[0].ID != p.ID {
		t.Fatalf("ListPendingProposals 应返回 1 条提议，实际 %v err=%v", props, err)
	}

	// approve：替换知识树 + 删提议 + 快照
	msg, err := ApplySkeletonProposal(tmp, p.ID)
	if err != nil {
		t.Fatalf("ApplySkeletonProposal: %v", err)
	}
	if !strings.Contains(msg, p.ID) {
		t.Errorf("apply 返回信息应含 id: %q", msg)
	}
	applied, _ := os.ReadFile(indexPath)
	if !strings.Contains(string(applied), "- 新节点甲 [缺口]") || strings.Contains(string(applied), "- 旧节点 [缺口]") {
		t.Errorf("apply 后知识树未替换:\n%s", string(applied))
	}
	if !strings.Contains(string(applied), "## 核心脉络") {
		t.Error("apply 破坏了其它章节")
	}
	if remain, _ := ListPendingProposals(tmp); len(remain) != 0 {
		t.Errorf("apply 后提议应被删除，仍剩 %d", len(remain))
	}
	// 快照应已生成
	snaps, _ := os.ReadDir(filepath.Join(dir, "digest"))
	if len(snaps) == 0 {
		t.Error("apply 前应快照旧 _index.md 到 digest/")
	}

	// reject：再存一条后驳回
	p2 := p
	p2.ID = "prog-20260617-130000"
	if err := saveProposalFile(tmp, p2); err != nil {
		t.Fatal(err)
	}
	if _, err := RejectSkeletonProposal(tmp, p2.ID); err != nil {
		t.Fatalf("RejectSkeletonProposal: %v", err)
	}
	if remain, _ := ListPendingProposals(tmp); len(remain) != 0 {
		t.Errorf("reject 后提议应被删除，仍剩 %d", len(remain))
	}
	if _, err := RejectSkeletonProposal(tmp, "不存在"); err == nil {
		t.Error("reject 不存在的提议应报错")
	}
}
