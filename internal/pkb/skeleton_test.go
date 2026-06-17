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
