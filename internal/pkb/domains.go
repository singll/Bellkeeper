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
	VaultThreshold                  float64         `yaml:"vault_threshold"`
	ArchiveThreshold                float64         `yaml:"archive_threshold"`
	Weights                         Weights         `yaml:"weights"`
	ScoreModel                      string          `yaml:"score_model"`
	ReconstructModel                string          `yaml:"reconstruct_model"`
	DigestModel                     string          `yaml:"digest_model"`
	SkeletonModel                   string          `yaml:"skeleton_model"`     // 骨架生成用顶级推理档（高价值低频）；空则回退 digest_model
	MatchModel                      string          `yaml:"match_model"`        // 归位匹配用快强档（卡↔骨架节点高频判断）；空则回退 score_model
	GapfillModel                    string          `yaml:"gapfill_model"`      // 缺口填充起草用顶级推理档；空则回退 skeleton_model
	VerifyModel                     string          `yaml:"verify_model"`       // 缺口填充 V2 核实用快强档；空则回退 match_model
	PromoteModel                    string          `yaml:"promote_model"`      // 资讯综述/晋升判定用快强档；空则回退 match_model
	FeedContentTypes                []string        `yaml:"feed_content_types"` // 视为「资讯」的 content_type(pkb_type)，默认 [news, release]
	ScoreTemperature                float64         `yaml:"score_temperature"`
	ReconstructTemperature          float64         `yaml:"reconstruct_temperature"`
	DigestTemperature               float64         `yaml:"digest_temperature"`
	PerRun                          int             `yaml:"per_run"`
	ContentTruncate                 int             `yaml:"content_truncate"`
	LLMTokenEnv                     string          `yaml:"llm_token_env"`
	MaxCardsPerArticle              int             `yaml:"max_cards_per_article"`
	EnableSemanticDedup             *bool           `yaml:"enable_semantic_dedup"`
	MapSnapshotOnRefresh            *bool           `yaml:"map_snapshot_on_refresh"`
	TopicMocEnabled                 *bool           `yaml:"topic_moc_enabled"`
	TopicMinCards                   int             `yaml:"topic_min_cards"`
	SkeletonChangeApprovalThreshold int             `yaml:"skeleton_change_approval_threshold"` // 骨架变更影响半径≤此值=小动作自动应用，>此值=大动作走 Matrix 批准
	GapFillPerRun                   int             `yaml:"gap_fill_per_run"`                   // 缺口填充每领域每轮上限（默认10）
	GapFillOrder                    string          `yaml:"gap_fill_order"`                     // breadth=自顶向下广度优先（先填根/主题层缺口）
	GapFillEnabled                  map[string]bool `yaml:"gap_fill_enabled"`                   // 每领域开关（优先级最高，打样先只开一个域）
	GapFillEnabledAll               *bool           `yaml:"gap_fill_enabled_all"`               // 一键总开关
	GapFillDefault                  *bool           `yaml:"gap_fill_default"`                   // 未列入 gap_fill_enabled 的新领域默认开/关
	AuditOnRun                      *bool           `yaml:"audit_on_run"`
	Budget                          Budget          `yaml:"budget"`
	Retry                           Retry           `yaml:"retry"`
}

// Budget 本轮大模型调用护栏。0 表示不限制。
type Budget struct {
	MaxScoreCallsPerRun       int `yaml:"max_score_calls_per_run"`
	MaxReconstructCallsPerRun int `yaml:"max_reconstruct_calls_per_run"`
	MaxDigestCallsPerRun      int `yaml:"max_digest_calls_per_run"`
}

// Retry controls PKB-level backoff for low-throughput/free LLM pools.
type Retry struct {
	MaxAttempts           int   `yaml:"max_attempts"`
	InitialBackoffSeconds int   `yaml:"initial_backoff_seconds"`
	MaxBackoffSeconds     int   `yaml:"max_backoff_seconds"`
	StopRunOnRateLimit    *bool `yaml:"stop_run_on_rate_limit"`
}

// Domain 单个领域
type Domain struct {
	Name             string   `yaml:"name"`
	Display          string   `yaml:"display"`
	Scope            string   `yaml:"scope"` // 一句话「大方向」——骨架生成(skeleton)的输入；为空则该领域不能生成骨架
	VaultSubpath     string   `yaml:"vault_subpath"`
	Keywords         []string `yaml:"keywords"`
	IsDefault        bool     `yaml:"is_default"`
	VaultThreshold   float64  `yaml:"vault_threshold"`   // 0 = 用 defaults
	ArchiveThreshold float64  `yaml:"archive_threshold"` // 0 = 用 defaults
	// Feed 标记该领域为「资讯库容器」（ADR-0005）：其 vault 子目录承载分领域分日资讯存档
	// （资讯/<领域>/<日期>.md），不产知识原子卡——故 digest/audit 的领域遍历跳过它、知识卡
	// 统计不计入，资讯不污染知识骨架（替代「collectDigestCards 路径排除」的更干净做法）。
	Feed bool `yaml:"feed"`
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
		d.Weights = Weights{Relevance: 0.30, Depth: 0.25, Actionability: 0.20, Durability: 0.10, Novelty: 0.15}
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
	if d.SkeletonModel == "" {
		d.SkeletonModel = d.DigestModel
	}
	if d.MatchModel == "" {
		d.MatchModel = d.ScoreModel
	}
	if d.GapfillModel == "" {
		d.GapfillModel = d.SkeletonModel
	}
	if d.VerifyModel == "" {
		d.VerifyModel = d.MatchModel
	}
	if d.PromoteModel == "" {
		d.PromoteModel = d.MatchModel
	}
	if len(d.FeedContentTypes) == 0 {
		d.FeedContentTypes = []string{"news", "release"}
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
		d.ContentTruncate = 12000
	}
	if d.MaxCardsPerArticle <= 0 {
		d.MaxCardsPerArticle = 5
	}
	if d.EnableSemanticDedup == nil {
		d.EnableSemanticDedup = boolPtr(true)
	}
	if d.MapSnapshotOnRefresh == nil {
		d.MapSnapshotOnRefresh = boolPtr(true)
	}
	if d.TopicMocEnabled == nil {
		d.TopicMocEnabled = boolPtr(true)
	}
	if d.TopicMinCards <= 0 {
		d.TopicMinCards = 5
	}
	if d.SkeletonChangeApprovalThreshold <= 0 {
		d.SkeletonChangeApprovalThreshold = 5
	}
	if d.GapFillPerRun <= 0 {
		d.GapFillPerRun = 10
	}
	if d.GapFillOrder == "" {
		d.GapFillOrder = "breadth"
	}
	if d.GapFillEnabledAll == nil {
		d.GapFillEnabledAll = boolPtr(false)
	}
	if d.GapFillDefault == nil {
		d.GapFillDefault = boolPtr(false)
	}
	if d.AuditOnRun == nil {
		d.AuditOnRun = boolPtr(true)
	}
	if d.Retry.MaxAttempts <= 0 {
		d.Retry.MaxAttempts = 4
	}
	if d.Retry.InitialBackoffSeconds <= 0 {
		d.Retry.InitialBackoffSeconds = 20
	}
	if d.Retry.MaxBackoffSeconds <= 0 {
		d.Retry.MaxBackoffSeconds = 300
	}
	if d.Retry.StopRunOnRateLimit == nil {
		d.Retry.StopRunOnRateLimit = boolPtr(true)
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

func boolPtr(v bool) *bool { return &v }

func (d *Defaults) GetEnableSemanticDedup() bool {
	if d.EnableSemanticDedup != nil {
		return *d.EnableSemanticDedup
	}
	return true
}

func (d *Defaults) GetMapSnapshotOnRefresh() bool {
	if d.MapSnapshotOnRefresh != nil {
		return *d.MapSnapshotOnRefresh
	}
	return true
}

func (d *Defaults) GetTopicMocEnabled() bool {
	if d.TopicMocEnabled != nil {
		return *d.TopicMocEnabled
	}
	return true
}

func (d *Defaults) GetAuditOnRun() bool {
	if d.AuditOnRun != nil {
		return *d.AuditOnRun
	}
	return true
}

func (r *Retry) GetStopRunOnRateLimit() bool {
	if r.StopRunOnRateLimit != nil {
		return *r.StopRunOnRateLimit
	}
	return true
}

// GapFillEnabledFor 判断某领域是否开启缺口填充：每领域开关(gap_fill_enabled.<域>)优先；
// 未显式配置则用总开关(gap_fill_enabled_all)，再退到新领域默认(gap_fill_default)。
// 全配置驱动——改 domains.yaml 即调，无需重编。
func (d *Defaults) GapFillEnabledFor(domain string) bool {
	if v, ok := d.GapFillEnabled[domain]; ok {
		return v
	}
	if d.GapFillEnabledAll != nil && *d.GapFillEnabledAll {
		return true
	}
	if d.GapFillDefault != nil {
		return *d.GapFillDefault
	}
	return false
}
