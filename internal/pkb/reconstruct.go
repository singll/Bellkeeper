package pkb

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// reconstructStats 记录重构过程中的丢弃统计
type reconstructStats struct {
	Validated      int
	FailedValidate int
	Truncated      int
}

// reconstructCard 调重构 LLM，产出 1-N 张原子 Obsidian markdown 卡片；写盘前清理死链。
// 返回多卡数组（v2 提示词用 ---CARD--- 分隔；v1 提示词仍返回单元素数组）。
// validCandidates 只含裸 concept（供 pruneWikilinks 有效集），displayCandidates 含摘要（供提示词渲染）。
func (c *Curator) reconstructCard(art ArticleMeta, body string, score *ScoreResult, domain Domain, validCandidates []string, displayCandidates []string, date string) ([]string, *reconstructStats, error) {
	if max := c.domains.Defaults.Budget.MaxReconstructCallsPerRun; max > 0 && c.reconstructCalls >= max {
		return nil, nil, fmt.Errorf("reconstruct budget exhausted: %d/%d", c.reconstructCalls, max)
	}
	c.reconstructCalls++
	content := truncateRunes(body, c.domains.Defaults.ContentTruncate)

	candBlock := "（暂无候选）"
	if len(displayCandidates) > 0 {
		candBlock = "- " + strings.Join(displayCandidates, "\n- ")
	}
	scoreStr := fmt.Sprintf("relevance=%d depth=%d actionability=%d final=%.1f",
		score.Relevance, score.Depth, score.Actionability, score.FinalScore(c.domains.Defaults))
	maxCards := c.domains.Defaults.MaxCardsPerArticle
	if maxCards <= 0 {
		maxCards = 5
	}
	tagsStr := strings.Join(score.MatchedDomains, ", ")

	prompt := c.reconstructPrompt
	prompt = strings.ReplaceAll(prompt, "{{title}}", art.Title)
	prompt = strings.ReplaceAll(prompt, "{{url}}", art.URL)
	prompt = strings.ReplaceAll(prompt, "{{date}}", date)
	prompt = strings.ReplaceAll(prompt, "{{score}}", scoreStr)
	prompt = strings.ReplaceAll(prompt, "{{domains}}", strings.Join(score.MatchedDomains, ", "))
	prompt = strings.ReplaceAll(prompt, "{{tags}}", tagsStr)
	prompt = strings.ReplaceAll(prompt, "{{max_cards}}", strconv.Itoa(maxCards))
	prompt = strings.ReplaceAll(prompt, "{{candidates}}", candBlock)
	prompt = strings.ReplaceAll(prompt, "{{content}}", content)

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.ReconstructModel, "", prompt, c.domains.Defaults.ReconstructTemperature, "long_context")
	if err != nil {
		return nil, nil, fmt.Errorf("reconstruct llm: %w", err)
	}
	raw := textutil.StripFence(out)
	if isNoCardReconstruct(raw) {
		return nil, nil, fmt.Errorf("reconstruct declined: 正文无有效内容或与主题无关（NO_CARD）")
	}

	cards := splitCards(raw)
	if len(cards) == 0 {
		return nil, nil, fmt.Errorf("reconstruct produced no cards")
	}
	stats := &reconstructStats{}
	if len(cards) > maxCards {
		fmt.Printf("    ⚠ LLM 产出 %d 张卡，超出 max_cards=%d 限制，截取前 %d 张\n", len(cards), maxCards, maxCards)
		stats.Truncated = len(cards) - maxCards
		cards = cards[:maxCards]
	}

	batchConcepts := make([]string, 0, len(cards))
	for _, card := range cards {
		if concept := frontmatterValue(card, "atomic_concept"); concept != "" {
			batchConcepts = append(batchConcepts, concept)
		}
	}

	validSet := buildValidLinkSet(validCandidates, batchConcepts, cards)

	var result []string
	for i, card := range cards {
		card = pruneWikilinks(card, validSet)
		if err := validateCard(card); err != nil {
			fmt.Printf("    ⚠ 第 %d 张卡校验失败（跳过）: %v\n", i+1, err)
			stats.FailedValidate++
			continue
		}
		stats.Validated++
		result = append(result, card)
	}
	if len(result) == 0 {
		return nil, stats, fmt.Errorf("all cards failed validation")
	}
	if stats.FailedValidate > 0 {
		survivorConcepts := make([]string, 0, len(result))
		for _, card := range result {
			if concept := frontmatterValue(card, "atomic_concept"); concept != "" {
				survivorConcepts = append(survivorConcepts, concept)
			}
		}
		prunedValidSet := buildValidLinkSet(validCandidates, survivorConcepts, result)
		for i, card := range result {
			result[i] = pruneWikilinks(card, prunedValidSet)
		}
	}
	return result, stats, nil
}

// splitCards 按 ---CARD--- 分隔符拆分 LLM 输出为多张卡片。
func splitCards(raw string) []string {
	parts := strings.Split(raw, "\n---CARD---\n")
	if len(parts) > 1 {
		var cards []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cards = append(cards, p)
			}
		}
		return cards
	}
	if strings.Contains(raw, "---CARD---") {
		parts = strings.Split(raw, "---CARD---")
		var cards []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cards = append(cards, p)
			}
		}
		return cards
	}
	return []string{strings.TrimSpace(raw)}
}

// buildValidLinkSet 构建有效链接目标集 = 已有 vault 候选 ∪ 本批次 concept ∪ 所有卡的 aliases。
func buildValidLinkSet(vaultCandidates, batchConcepts []string, cards []string) []string {
	seen := make(map[string]bool)
	var result []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, c := range vaultCandidates {
		add(c)
	}
	for _, c := range batchConcepts {
		add(c)
	}
	for _, card := range cards {
		if aliases := frontmatterValue(card, "aliases"); aliases != "" {
			for _, a := range strings.Split(aliases, ",") {
				a = strings.TrimSpace(strings.Trim(a, "[]\"' "))
				add(a)
			}
		}
		if concept := frontmatterValue(card, "atomic_concept"); concept != "" {
			add(concept)
		}
	}
	return result
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
	v1Sections := []string{
		"## 核心洞察",
		"## 关键技术要点 / 可复用资产",
		"## 深度摘要",
		"## 关联",
	}
	v2Sections := []string{
		"## 定义与本质",
		"## 关键细节",
		"## 适用场景与边界",
		"## 与其他知识的关系",
	}
	v1Ok := true
	for _, section := range v1Sections {
		if !strings.Contains(trimmed, section) {
			v1Ok = false
			break
		}
	}
	v2Ok := true
	for _, section := range v2Sections {
		if !strings.Contains(trimmed, section) {
			v2Ok = false
			break
		}
	}
	if !v1Ok && !v2Ok {
		return fmt.Errorf("generated card missing required sections (neither v1 nor v2 format)")
	}
	if v2Ok {
		if !strings.Contains(trimmed, "\natomic_concept:") {
			return fmt.Errorf("generated card missing frontmatter key atomic_concept (v2)")
		}
		if !strings.Contains(trimmed, "\naliases:") && !strings.Contains(trimmed, "\naliases: [") && !strings.Contains(trimmed, "\naliases:[]") {
			return fmt.Errorf("generated card missing frontmatter key aliases (v2)")
		}
		if !strings.Contains(trimmed, "\ncard_type:") {
			return fmt.Errorf("generated card missing frontmatter key card_type (v2)")
		}
		if strings.Contains(trimmed, "card_type: supplement") && !strings.Contains(trimmed, "supplement_to:") {
			return fmt.Errorf("supplement card missing supplement_to field (v2)")
		}
	}
	if len([]rune(trimmed)) < 100 {
		return fmt.Errorf("generated card too short")
	}
	return nil
}

// isNoCardReconstruct 识别重构 LLM 拒绝产卡的信号（正文实为反爬/验证/错误页，或与参考主题完全无关）。
func isNoCardReconstruct(raw string) bool {
	t := strings.TrimSpace(raw)
	return t == "NO_CARD" || strings.HasPrefix(t, "NO_CARD")
}
