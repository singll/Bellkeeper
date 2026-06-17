package pkb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// SkeletonOptions 控制一次知识骨架生成（针对单个领域）。
type SkeletonOptions struct {
	Domain string // 领域 name 或 display（必填，须在 domains.yaml 配有 scope）
	TOC    string // 可选：权威源参考目录文本，喂入提示词校正层级与覆盖
	DryRun bool
}

// RunSkeleton 按领域的 scope（大方向）生成初次知识骨架——一棵多层目标概念树，
// 每个节点初始为 `[缺口]`（ADR-0004：骨架是结构唯一真相源，digest 后续把卡片渲染挂上去）。
//
// 写盘复用原子计划护栏 6：覆盖前先快照旧 _index.md → 写 _index.next.md → 原子 rename。
// 存量卡链接由后续「归位」(match) 步骤重新挂载；主题 MOC(topics/<主题>.md) 随 digest 在
// 卡片累积后生成，因此本步只写根 _index.md。
func (c *Curator) RunSkeleton(opts SkeletonOptions) error {
	domain, ok := c.domains.FindDomain(opts.Domain)
	if !ok {
		return fmt.Errorf("unknown domain: %s", opts.Domain)
	}
	scope := strings.TrimSpace(domain.Scope)
	if scope == "" {
		return fmt.Errorf("领域 %s 未配置 scope（大方向）；先在 config/pkb/domains.yaml 给它加一行 `scope:` 再生成骨架", domain.Name)
	}

	fmt.Printf("[pkb-skeleton] 模式=%s 领域=%s(%s) model=%s prompt=%s\n",
		digestMode(opts.DryRun), domain.Display, domain.Name,
		c.domains.Defaults.SkeletonModel, c.skeletonPromptName)
	fmt.Printf("[pkb-skeleton] scope：%s\n", scope)

	indexPath := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	if opts.DryRun {
		fmt.Printf("[pkb-skeleton] DRY-RUN：仅打印 scope 与目标路径，不调用 LLM、不写盘\n")
		fmt.Printf("[pkb-skeleton] 目标：%s\n", indexPath)
		return nil
	}

	now := time.Now()
	prompt := c.skeletonPrompt
	prompt = strings.ReplaceAll(prompt, "{{domain_display}}", domain.Display)
	prompt = strings.ReplaceAll(prompt, "{{domain_name}}", domain.Name)
	prompt = strings.ReplaceAll(prompt, "{{scope}}", scope)
	prompt = strings.ReplaceAll(prompt, "{{generated_at}}", now.Format(time.RFC3339))
	toc := strings.TrimSpace(opts.TOC)
	prompt = strings.ReplaceAll(prompt, "{{toc}}", toc)
	if toc == "" {
		prompt = removeEmptySection(prompt, "## 权威源参考目录")
	}

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.SkeletonModel, "", prompt, c.domains.Defaults.DigestTemperature, "long_context")
	if err != nil {
		return fmt.Errorf("skeleton llm: %w", err)
	}
	card := textutil.StripFence(out)
	if err := validateDigestWithMode(card, digestModeRoot); err != nil {
		return fmt.Errorf("骨架输出不合法（缺 frontmatter 或必需章节）: %w", err)
	}

	dir := filepath.Join(c.basePath, domain.VaultSubpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir vault dir: %w", err)
	}

	// 护栏 6：覆盖前快照旧 _index.md（存量卡链接将由后续「归位」重新挂载，快照可回滚）
	if c.loadExistingIndex(domain) != "" {
		if err := c.snapshotIndex(domain); err != nil {
			fmt.Printf("[pkb-skeleton] ⚠ 快照旧 _index.md 失败（继续）: %v\n", err)
		} else {
			fmt.Printf("[pkb-skeleton] 已快照旧 _index.md → %s/digest/（存量卡链接将由后续「归位」重新挂载）\n", domain.VaultSubpath)
		}
	}

	nextPath := filepath.Join(dir, "_index.next.md")
	if err := os.WriteFile(nextPath, []byte(card+"\n"), 0644); err != nil {
		return fmt.Errorf("write _index.next.md: %w", err)
	}
	if err := os.Rename(nextPath, indexPath); err != nil {
		_ = os.Remove(nextPath)
		return fmt.Errorf("rename _index.next.md→_index.md: %w", err)
	}
	fmt.Printf("[pkb-skeleton] → %s\n", indexPath)

	fmt.Printf("[pkb-skeleton] 触发 rebuild 与文件系统对齐...\n")
	if err := c.client.Rebuild(); err != nil {
		fmt.Printf("[pkb-skeleton] ⚠ rebuild 失败（骨架已写盘，可稍后手动 rebuild）: %v\n", err)
	}

	gaps := strings.Count(card, "[缺口]")
	fmt.Printf("[pkb-skeleton] 完成：根骨架已落盘，%d 个缺口待填充/归位；主题 MOC(topics/) 将随 digest 在卡片累积后生成\n", gaps)
	return nil
}
