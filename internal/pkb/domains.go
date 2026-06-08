package pkb

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Weights 三维打分权重（和应为 1.0）
type Weights struct {
	Relevance     float64 `yaml:"relevance"`
	Depth         float64 `yaml:"depth"`
	Actionability float64 `yaml:"actionability"`
	Durability    float64 `yaml:"durability"`
	Novelty       float64 `yaml:"novelty"`
}

// Defaults 全局默认（可被单领域覆盖）
type Defaults struct {
	VaultThreshold         float64 `yaml:"vault_threshold"`
	ArchiveThreshold       float64 `yaml:"archive_threshold"`
	Weights                Weights `yaml:"weights"`
	ScoreModel             string  `yaml:"score_model"`
	ReconstructModel       string  `yaml:"reconstruct_model"`
	DigestModel            string  `yaml:"digest_model"`
	ScoreTemperature       float64 `yaml:"score_temperature"`       // 0=用默认 0.2；kimi-k2.6 等推理模型须设 1.0
	ReconstructTemperature float64 `yaml:"reconstruct_temperature"` // 0=用默认 0.4；kimi-k2.6 等推理模型须设 1.0
	DigestTemperature      float64 `yaml:"digest_temperature"`
	PerRun                 int     `yaml:"per_run"`
	ContentTruncate        int     `yaml:"content_truncate"`
	LLMTokenEnv            string  `yaml:"llm_token_env"` // 可选：专用 LLM token 环境变量名，便于 PKB 独立配额/成本控制
	Budget                 Budget  `yaml:"budget"`
}

// Budget 本轮大模型调用护栏。0 表示不限制。
type Budget struct {
	MaxScoreCallsPerRun       int `yaml:"max_score_calls_per_run"`
	MaxReconstructCallsPerRun int `yaml:"max_reconstruct_calls_per_run"`
	MaxDigestCallsPerRun      int `yaml:"max_digest_calls_per_run"`
}

// Domain 单个领域
type Domain struct {
	Name             string   `yaml:"name"`
	Display          string   `yaml:"display"`
	VaultSubpath     string   `yaml:"vault_subpath"`
	Keywords         []string `yaml:"keywords"`
	IsDefault        bool     `yaml:"is_default"`
	VaultThreshold   float64  `yaml:"vault_threshold"`   // 0 = 用 defaults
	ArchiveThreshold float64  `yaml:"archive_threshold"` // 0 = 用 defaults
}

// DomainsConfig domains.yaml 的根
type DomainsConfig struct {
	Defaults Defaults `yaml:"defaults"`
	Domains  []Domain `yaml:"domains"`
}

// LoadDomains 从 yaml 加载领域配置并补默认（防 0 值导致全部误判 vault）
func LoadDomains(path string) (*DomainsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read domains config %s: %w", path, err)
	}

	var dc DomainsConfig
	if err := yaml.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("parse domains config %s: %w", path, err)
	}
	if len(dc.Domains) == 0 {
		return nil, fmt.Errorf("domains config %s has no domains", path)
	}

	d := &dc.Defaults
	if d.VaultThreshold == 0 {
		d.VaultThreshold = 7.0
	}
	if d.ArchiveThreshold == 0 {
		d.ArchiveThreshold = 4.0
	}
	if d.Weights == (Weights{}) {
		d.Weights = Weights{Relevance: 0.35, Depth: 0.25, Actionability: 0.25, Durability: 0.15}
	}
	if d.ScoreModel == "" {
		d.ScoreModel = "pool-summary"
	}
	if d.ReconstructModel == "" {
		d.ReconstructModel = d.ScoreModel
	}
	if d.DigestModel == "" {
		d.DigestModel = d.ReconstructModel
	}
	if d.ScoreTemperature == 0 {
		d.ScoreTemperature = 0.2
	}
	if d.ReconstructTemperature == 0 {
		d.ReconstructTemperature = 0.4
	}
	if d.DigestTemperature == 0 {
		d.DigestTemperature = 0.4
	}
	if d.PerRun <= 0 {
		d.PerRun = 5
	}
	if d.ContentTruncate <= 0 {
		d.ContentTruncate = 8000
	}

	return &dc, nil
}

// DefaultDomain 返回兜底领域（is_default），没有则返回最后一个
func (dc *DomainsConfig) DefaultDomain() Domain {
	for _, d := range dc.Domains {
		if d.IsDefault {
			return d
		}
	}
	return dc.Domains[len(dc.Domains)-1]
}

// ResolveDomain 按 LLM 返回的 matched_domains 选定落点领域（取第一个在配置中存在的，否则兜底）
func (dc *DomainsConfig) ResolveDomain(matched []string) Domain {
	for _, m := range matched {
		for _, d := range dc.Domains {
			if strings.EqualFold(d.Name, m) {
				return d
			}
		}
	}
	return dc.DefaultDomain()
}

// FindDomain returns a configured domain by name or display label.
func (dc *DomainsConfig) FindDomain(name string) (Domain, bool) {
	for _, d := range dc.Domains {
		if strings.EqualFold(d.Name, name) || d.Display == name {
			return d, true
		}
	}
	return Domain{}, false
}

// VaultThresholdOr 领域阈值优先，否则用全局默认
func (d Domain) VaultThresholdOr(def Defaults) float64 {
	if d.VaultThreshold > 0 {
		return d.VaultThreshold
	}
	return def.VaultThreshold
}

// ArchiveThresholdOr 领域阈值优先，否则用全局默认
func (d Domain) ArchiveThresholdOr(def Defaults) float64 {
	if d.ArchiveThreshold > 0 {
		return d.ArchiveThreshold
	}
	return def.ArchiveThreshold
}

// DomainsPromptBlock 把领域清单渲染成喂给打分 LLM 的文本块
func (dc *DomainsConfig) DomainsPromptBlock() string {
	var b strings.Builder
	for _, d := range dc.Domains {
		kw := strings.Join(d.Keywords, ", ")
		if kw == "" {
			kw = "（兜底领域，无关键词）"
		}
		b.WriteString(fmt.Sprintf("- %s（%s）：%s\n", d.Name, d.Display, kw))
	}
	return b.String()
}
