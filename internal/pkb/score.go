package pkb

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ScoreResult 打分结果（LLM 返回的三维分 + matched_domains + 依据）
type ScoreResult struct {
	Relevance      int      `json:"relevance"`
	Depth          int      `json:"depth"`
	Actionability  int      `json:"actionability"`
	Durability     int      `json:"durability"`
	Novelty        int      `json:"novelty"`
	ContentType    string   `json:"content_type"`
	MatchedDomains []string `json:"matched_domains"`
	Reason         string   `json:"reason"`
}

// FinalScore 按权重计算综合分（0–10）
func (sr *ScoreResult) FinalScore(w Weights) float64 {
	score := w.Relevance*float64(sr.Relevance) +
		w.Depth*float64(sr.Depth) +
		w.Actionability*float64(sr.Actionability) +
		w.Durability*float64(sr.Durability) +
		w.Novelty*float64(sr.Novelty)
	switch strings.ToLower(strings.TrimSpace(sr.ContentType)) {
	case "marketing":
		score -= 2.0
	case "news":
		score -= 1.0
	case "release":
		score -= 0.5
	case "tutorial", "paper", "reference":
		score += 0.5
	case "code", "poc":
		score += 0.7
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

	out, err := c.client.ChatCompletion(c.domains.Defaults.ScoreModel, "", prompt, c.domains.Defaults.ScoreTemperature, "summary")
	if err != nil {
		return nil, fmt.Errorf("score llm: %w", err)
	}

	var sr ScoreResult
	if err := json.Unmarshal([]byte(stripJSONFence(out)), &sr); err != nil {
		return nil, fmt.Errorf("parse score json (%q): %w", truncate(out, 200), err)
	}
	sr.Relevance = clamp10(sr.Relevance)
	sr.Depth = clamp10(sr.Depth)
	sr.Actionability = clamp10(sr.Actionability)
	sr.Durability = clamp10(sr.Durability)
	sr.Novelty = clamp10(sr.Novelty)
	return &sr, nil
}

// stripJSONFence 剥掉 markdown 代码围栏（抄 classify.go 的容错）
func stripJSONFence(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
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
