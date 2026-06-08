package pkb

import (
	"fmt"
	"regexp"
	"strings"
)

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// reconstructCard 调重构 LLM，产出完整 Obsidian markdown 卡片；写盘前清理死链。
func (c *Curator) reconstructCard(art ArticleMeta, body string, score *ScoreResult, domain Domain, candidates []string, date string) (string, error) {
	if max := c.domains.Defaults.Budget.MaxReconstructCallsPerRun; max > 0 && c.reconstructCalls >= max {
		return "", fmt.Errorf("reconstruct budget exhausted: %d/%d", c.reconstructCalls, max)
	}
	c.reconstructCalls++
	content := truncateRunes(body, c.domains.Defaults.ContentTruncate)

	candBlock := "（暂无候选）"
	if len(candidates) > 0 {
		candBlock = "- " + strings.Join(candidates, "\n- ")
	}
	scoreStr := fmt.Sprintf("relevance=%d depth=%d actionability=%d final=%.1f",
		score.Relevance, score.Depth, score.Actionability, score.FinalScore(c.domains.Defaults.Weights))

	prompt := c.reconstructPrompt
	prompt = strings.ReplaceAll(prompt, "{{title}}", art.Title)
	prompt = strings.ReplaceAll(prompt, "{{url}}", art.URL)
	prompt = strings.ReplaceAll(prompt, "{{date}}", date)
	prompt = strings.ReplaceAll(prompt, "{{score}}", scoreStr)
	prompt = strings.ReplaceAll(prompt, "{{domains}}", strings.Join(score.MatchedDomains, ", "))
	prompt = strings.ReplaceAll(prompt, "{{candidates}}", candBlock)
	prompt = strings.ReplaceAll(prompt, "{{content}}", content)

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.ReconstructModel, "", prompt, c.domains.Defaults.ReconstructTemperature, "long_context")
	if err != nil {
		return "", fmt.Errorf("reconstruct llm: %w", err)
	}
	card := stripCardFence(out)
	card = pruneWikilinks(card, candidates) // 防 Obsidian 死链（§4.7）
	if err := validateCard(card); err != nil {
		return "", err
	}
	return card, nil
}

// stripCardFence 去掉整卡被 ``` 包裹的情况（保留卡内 frontmatter 的 ---）
func stripCardFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```markdown") {
		s = strings.TrimPrefix(s, "```markdown")
		s = strings.TrimSuffix(s, "```")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

// pruneWikilinks 移除指向不存在卡片的 [[wikilink]]（候选为空则全部降级为纯文本）。
func pruneWikilinks(card string, candidates []string) string {
	if len(candidates) == 0 {
		return wikilinkRe.ReplaceAllString(card, "$1")
	}
	valid := make(map[string]bool, len(candidates))
	for _, t := range candidates {
		valid[strings.TrimSpace(t)] = true
	}
	return wikilinkRe.ReplaceAllStringFunc(card, func(m string) string {
		inner := wikilinkRe.FindStringSubmatch(m)[1]
		target := strings.TrimSpace(strings.SplitN(inner, "|", 2)[0]) // 支持 [[target|alias]]
		if valid[target] {
			return m
		}
		return strings.TrimSpace(inner) // 降级为纯文本，去掉 [[]]
	})
}

func validateCard(card string) error {
	trimmed := strings.TrimSpace(card)
	if !strings.HasPrefix(trimmed, "---\n") {
		return fmt.Errorf("generated card missing YAML frontmatter")
	}
	requiredKeys := []string{"title:", "source:", "ingest_date:", "score:", "domains:", "tags:"}
	for _, key := range requiredKeys {
		if !strings.Contains(trimmed, "\n"+key) {
			return fmt.Errorf("generated card missing frontmatter key %s", strings.TrimSuffix(key, ":"))
		}
	}
	requiredSections := []string{
		"## 核心洞察",
		"## 关键技术要点 / 可复用资产",
		"## 深度摘要",
		"## 关联",
	}
	for _, section := range requiredSections {
		if !strings.Contains(trimmed, section) {
			return fmt.Errorf("generated card missing section %s", section)
		}
	}
	if len([]rune(trimmed)) < 200 {
		return fmt.Errorf("generated card too short")
	}
	return nil
}
