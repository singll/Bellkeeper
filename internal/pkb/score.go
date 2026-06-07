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
	MatchedDomains []string `json:"matched_domains"`
	Reason         string   `json:"reason"`
}

// FinalScore 按权重计算综合分（0–10）
func (sr *ScoreResult) FinalScore(w Weights) float64 {
	return w.Relevance*float64(sr.Relevance) +
		w.Depth*float64(sr.Depth) +
		w.Actionability*float64(sr.Actionability)
}

// scoreArticle 调打分 LLM 并解析为 ScoreResult（注入领域+标题+正文截断；以分数为准，不信任 LLM 自报 decision）
func (c *Curator) scoreArticle(art ArticleMeta, body string) (*ScoreResult, error) {
	content := body
	if c.domains.Defaults.ContentTruncate > 0 && len(content) > c.domains.Defaults.ContentTruncate {
		content = content[:c.domains.Defaults.ContentTruncate]
	}

	prompt := c.scorePrompt
	prompt = strings.ReplaceAll(prompt, "{{domains}}", c.domains.DomainsPromptBlock())
	prompt = strings.ReplaceAll(prompt, "{{title}}", art.Title)
	prompt = strings.ReplaceAll(prompt, "{{content}}", content)

	out, err := c.client.ChatCompletion(c.domains.Defaults.ScoreModel, "", prompt, c.domains.Defaults.ScoreTemperature)
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
	if len(s) > n {
		return s[:n]
	}
	return s
}
