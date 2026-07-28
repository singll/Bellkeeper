// eval.go 实现 pkb-curate eval 子命令：对 golden set 跑评分回归。
//
// 对应《Bellkeeper 1.0 重构与架构演进规划》§2.1.3：
//   config/pkb/eval/*.json 放 10-20 篇带预期样本，pkb-curate eval 跑评分回归。
//
// 评分对比：
//   - 各维度分差绝对值 ≤ tolerance（默认 2）视为通过
//   - matched_domains 与预期有交集视为通过（避免顺序差异）
//   - content_type 完全匹配
//   - decision（vault/archive/discard）由 FinalScore + 阈值推导，与预期一致
//
// 输出 EvalReport：逐条结果 + 整体 accuracy + 各维度 MAE。

package pkb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/singll/bellkeeper/internal/config"
)

// EvalSample 单条 golden 样本。
type EvalSample struct {
	Title           string        `json:"title"`
	Content         string        `json:"content"`
	Expected        EvalExpected  `json:"expected"`
	ExpectedDecision string       `json:"expected_decision"`
}

// EvalExpected 预期评分。
type EvalExpected struct {
	Relevance       int      `json:"relevance"`
	Depth           int      `json:"depth"`
	Actionability   int      `json:"actionability"`
	Durability      int      `json:"durability"`
	Novelty         int      `json:"novelty"`
	AtomicPotential int      `json:"atomic_potential"`
	ContentType     string   `json:"content_type"`
	MatchedDomains  []string `json:"matched_domains"`
}

// EvalCaseResult 单条样本评估结果。
type EvalCaseResult struct {
	File          string  `json:"file"`
	Title         string  `json:"title"`
	Passed        bool    `json:"passed"`
	ActualDecision string `json:"actual_decision"`
	ExpectedDecision string `json:"expected_decision"`
	FinalScore    float64 `json:"final_score"`
	ScoreDiff     EvalScoreDiff `json:"score_diff"`
	DomainMatch   bool    `json:"domain_match"`
	ContentTypeMatch bool `json:"content_type_match"`
	DecisionMatch bool    `json:"decision_match"`
	Errors        string  `json:"errors,omitempty"`
}

// EvalScoreDiff 各维度实际-预期分差绝对值。
type EvalScoreDiff struct {
	Relevance       int `json:"relevance"`
	Depth           int `json:"depth"`
	Actionability   int `json:"actionability"`
	Durability      int `json:"durability"`
	Novelty         int `json:"novelty"`
	AtomicPotential int `json:"atomic_potential"`
}

// EvalReport 整体评估报告。
type EvalReport struct {
	Total      int               `json:"total"`
	Passed     int               `json:"passed"`
	Accuracy   float64           `json:"accuracy"`
	MAE        map[string]float64 `json:"mae"` // 各维度平均绝对误差
	Cases      []EvalCaseResult  `json:"cases"`
}

// EvalOptions eval 子命令选项。
type EvalOptions struct {
	ConfigDir string // config/pkb
	Tolerance int    // 分差容忍度（默认 2）
}

// RunEval 加载 golden set 并跑评分回归。
//
// 内部构造 DryRun Curator 复用 scoreArticle + 决策逻辑；不写文件、不移动、不索引。
// golden set 目录：ConfigDir/eval/*.json。cfg 提供 LLM Proxy URL/APIKey/basePath。
func RunEval(cfg *config.Config, opts EvalOptions) (*EvalReport, error) {
	if opts.Tolerance <= 0 {
		opts.Tolerance = 2
	}
	samples, err := loadEvalSamples(filepath.Join(opts.ConfigDir, "eval"))
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("no eval samples found in %s/eval", opts.ConfigDir)
	}

	// 构造 DryRun Curator 复用评分 prompt + domains + LLM 客户端。
	curator, err := NewCurator(cfg, Options{ConfigDir: opts.ConfigDir, DryRun: true}, nil)
	if err != nil {
		return nil, fmt.Errorf("construct curator: %w", err)
	}

	report := &EvalReport{Total: len(samples), MAE: map[string]float64{
		"relevance": 0, "depth": 0, "actionability": 0, "durability": 0, "novelty": 0, "atomic_potential": 0,
	}}
	sumAbs := map[string]int{"relevance": 0, "depth": 0, "actionability": 0, "durability": 0, "novelty": 0, "atomic_potential": 0}

	for _, s := range samples {
		cr := EvalCaseResult{File: s.file, Title: s.Title, ExpectedDecision: s.ExpectedDecision}
		score, err := curator.scoreArticle(ArticleMeta{Title: s.Title}, s.Content)
		if err != nil {
			cr.Errors = fmt.Sprintf("score failed: %v", err)
			report.Cases = append(report.Cases, cr)
			continue
		}
		final := score.FinalScore(curator.domains.Defaults)
		domain := curator.domains.ResolveDomain(score.MatchedDomains)
		cr.FinalScore = final
		cr.ActualDecision = decideFor(final, domain, curator.domains.Defaults)
		cr.DecisionMatch = cr.ActualDecision == cr.ExpectedDecision

		// 分差
		cr.ScoreDiff = EvalScoreDiff{
			Relevance:       abs(score.Relevance - s.Expected.Relevance),
			Depth:           abs(score.Depth - s.Expected.Depth),
			Actionability:   abs(score.Actionability - s.Expected.Actionability),
			Durability:      abs(score.Durability - s.Expected.Durability),
			Novelty:         abs(score.Novelty - s.Expected.Novelty),
			AtomicPotential: abs(score.AtomicPotential - s.Expected.AtomicPotential),
		}
		sumAbs["relevance"] += cr.ScoreDiff.Relevance
		sumAbs["depth"] += cr.ScoreDiff.Depth
		sumAbs["actionability"] += cr.ScoreDiff.Actionability
		sumAbs["durability"] += cr.ScoreDiff.Durability
		sumAbs["novelty"] += cr.ScoreDiff.Novelty
		sumAbs["atomic_potential"] += cr.ScoreDiff.AtomicPotential

		// 维度通过：所有分差 ≤ tolerance
		scorePassed := cr.ScoreDiff.Relevance <= opts.Tolerance &&
			cr.ScoreDiff.Depth <= opts.Tolerance &&
			cr.ScoreDiff.Actionability <= opts.Tolerance &&
			cr.ScoreDiff.Durability <= opts.Tolerance &&
			cr.ScoreDiff.Novelty <= opts.Tolerance

		// domain 匹配：有交集
		cr.DomainMatch = domainsIntersect(score.MatchedDomains, s.Expected.MatchedDomains)
		cr.ContentTypeMatch = strings.EqualFold(strings.TrimSpace(score.ContentType), strings.TrimSpace(s.Expected.ContentType))

		cr.Passed = scorePassed && cr.DomainMatch && cr.ContentTypeMatch && cr.DecisionMatch
		if cr.Passed {
			report.Passed++
		}
		report.Cases = append(report.Cases, cr)
	}

	report.Accuracy = float64(report.Passed) / float64(report.Total)
	for k, v := range sumAbs {
		report.MAE[k] = float64(v) / float64(report.Total)
	}
	return report, nil
}

// decideFor 复刻 curator 的决策逻辑（final < archive → discard, < vault → archive, else vault）。
func decideFor(final float64, domain Domain, defaults Defaults) string {
	if final < domain.ArchiveThresholdOr(defaults) {
		return "discard"
	}
	if final < domain.VaultThresholdOr(defaults) {
		return "archive"
	}
	return "vault"
}

type evalFile struct {
	EvalSample
	file string
}

func loadEvalSamples(dir string) ([]evalFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read eval dir: %w", err)
	}
	var out []evalFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var s EvalSample
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		out = append(out, evalFile{EvalSample: s, file: e.Name()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].file < out[j].file })
	return out, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func domainsIntersect(a, b []string) bool {
	if len(b) == 0 {
		return len(a) == 0
	}
	set := make(map[string]struct{}, len(a))
	for _, d := range a {
		set[strings.ToLower(strings.TrimSpace(d))] = struct{}{}
	}
	for _, d := range b {
		if _, ok := set[strings.ToLower(strings.TrimSpace(d))]; ok {
			return true
		}
	}
	return false
}