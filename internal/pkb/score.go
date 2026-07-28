package pkb

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// ScoreResult 打分结果（LLM 返回的三维分 + matched_domains + 依据）
type ScoreResult struct {
	Relevance       int      `json:"relevance"`
	Depth           int      `json:"depth"`
	Actionability   int      `json:"actionability"`
	Durability      int      `json:"durability"`
	Novelty         int      `json:"novelty"`
	AtomicPotential int      `json:"atomic_potential"`
	ContentType     string   `json:"content_type"`
	MatchedDomains  []string `json:"matched_domains"`
	Reason          string   `json:"reason"`
}

// FinalScore 综合分（0–10）：五维加权 + content_type 调整(配置化) + atomic_potential 上浮(防漏召)。
// 注意：相关度门（gate / hard_floor）不在此计算——门是「决策封顶」而非分数变换，由 Curator.decide 应用。
func (sr *ScoreResult) FinalScore(def Defaults) float64 {
	w := def.Weights
	score := w.Relevance*float64(sr.Relevance) +
		w.Depth*float64(sr.Depth) +
		w.Actionability*float64(sr.Actionability) +
		w.Durability*float64(sr.Durability) +
		w.Novelty*float64(sr.Novelty)
	// content_type 调整：配置优先（domains.yaml content_type_adjust），未配则回退内置默认。
	ct := strings.ToLower(strings.TrimSpace(sr.ContentType))
	if adj, ok := def.ContentTypeAdjust[ct]; ok {
		score += adj
	} else if adj, ok := defaultContentTypeAdjust[ct]; ok {
		score += adj
	}
	// atomic_potential 上浮（修死字段 + 防漏召）：信息密集（含多个可独立知识点）的好文，
	// 即使单维不突出也给小幅上浮，避免被阈值误杀。达标阈值与幅度均配置化。
	if def.AtomicPotentialBonusMin > 0 && sr.AtomicPotential >= def.AtomicPotentialBonusMin {
		score += def.AtomicPotentialBonus
	}
	if score < 0 {
		return 0
	}
	if score > 10 {
		return 10
	}
	return score
}

// scoreArticle 调打分 LLM 并解析为 ScoreResult（注入领域+标题+正文截断；以分数为准，不信任 LLM 自报 decision）
func (c *Curator) scoreArticle(art ArticleMeta, body string) (*ScoreResult, error) {
	if max := c.domains.Defaults.Budget.MaxScoreCallsPerRun; max > 0 && c.scoreCalls >= max {
		return nil, fmt.Errorf("score budget exhausted: %d/%d", c.scoreCalls, max)
	}
	c.scoreCalls++
	content := truncateRunes(body, c.domains.Defaults.ContentTruncate)

	prompt := c.scorePrompt
	prompt = strings.ReplaceAll(prompt, "{{domains}}", c.domains.DomainsPromptBlock())
	prompt = strings.ReplaceAll(prompt, "{{title}}", art.Title)
	prompt = strings.ReplaceAll(prompt, "{{content}}", content)

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.ScoreModel, "", prompt, c.domains.Defaults.ScoreTemperature, "summary")
	if err != nil {
		return nil, fmt.Errorf("score llm: %w", err)
	}

	var sr ScoreResult
	if err := json.Unmarshal([]byte(textutil.StripJSONFence(out)), &sr); err != nil {
		return nil, fmt.Errorf("parse score json (%q): %w", truncate(out, 200), err)
	}
	sr.Relevance = clamp10(sr.Relevance)
	sr.Depth = clamp10(sr.Depth)
	sr.Actionability = clamp10(sr.Actionability)
	sr.Durability = clamp10(sr.Durability)
	sr.Novelty = clamp10(sr.Novelty)
	sr.AtomicPotential = clamp10(sr.AtomicPotential)
	return &sr, nil
}

func clamp10(v int) int {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return v
}

func truncate(s string, n int) string {
	return truncateRunes(s, n)
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
