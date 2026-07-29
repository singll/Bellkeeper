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
	VaultThreshold   float64 `yaml:"vault_threshold"`
	ArchiveThreshold float64 `yaml:"archive_threshold"`
	Weights          Weights `yaml:"weights"`
	// 打分门控（结构性改造：堵离题噪音、修 atomic_potential 死字段、可审阈值）
	RelevanceGate                   float64            `yaml:"relevance_gate"`             // relevance < 此值 → 封顶不进 vault（默认5；设负数关闭）
	RelevanceHardFloor              float64            `yaml:"relevance_hard_floor"`       // relevance < 此值 → 直接 discard（默认3；设负数关闭）
	ContentTypeAdjust               map[string]float64 `yaml:"content_type_adjust"`        // content_type→final 加减分；缺失的 key 回退内置默认（可只覆盖单项）
	AtomicPotentialBonus            float64            `yaml:"atomic_potential_bonus"`     // atomic_potential 达标时 final 上浮（默认0.3，防信息密集好文漏召）
	AtomicPotentialBonusMin         int                `yaml:"atomic_potential_bonus_min"` // 触发上浮的 atomic_potential 阈值（默认8；设>10 关闭）
	ReviewLedgerEnabled             *bool              `yaml:"review_ledger_enabled"`      // 拒收台账开关（默认true）
	SnapshotKeep                    int                `yaml:"snapshot_keep"`              // digest/ 每域滚动保留份数（默认5）
	SnapshotWeekly                  *bool              `yaml:"snapshot_weekly"`            // 同周快照覆盖为一份（默认true）
	ScoreModel                      string             `yaml:"score_model"`
	ReconstructModel                string             `yaml:"reconstruct_model"`
	DigestModel                     string             `yaml:"digest_model"`
	SkeletonModel                   string             `yaml:"skeleton_model"`         // 骨架生成用顶级推理档（高价值低频）；空则回退 digest_model
	MatchModel                      string             `yaml:"match_model"`            // 归位匹配用快强档（卡↔骨架节点高频判断）；空则回退 score_model
	GapfillModel                    string             `yaml:"gapfill_model"`          // 缺口填充起草用顶级推理档；空则回退 skeleton_model
	VerifyModel                     string             `yaml:"verify_model"`           // 缺口填充 V2 核实用快强档；空则回退 match_model
	PromoteModel                    string             `yaml:"promote_model"`          // 资讯综述/晋升判定用快强档；空则回退 match_model
	FeedContentTypes                []string           `yaml:"feed_content_types"`     // 视为「资讯」的 content_type(pkb_type)，默认 [news, release]
	PromoteEnabled                  *bool              `yaml:"promote_enabled"`        // 资讯晋升闸总开关（耐久知识点→知识库卡，走同一 V2 路径）；默认 true
	PromoteDurabilityMin            float64            `yaml:"promote_durability_min"` // 晋升所需最低 durability（事件性一律不晋升）；默认 7.0
	ScoreTemperature                float64            `yaml:"score_temperature"`
	ReconstructTemperature          float64            `yaml:"reconstruct_temperature"`
	DigestTemperature               float64            `yaml:"digest_temperature"`
	PerRun                          int                `yaml:"per_run"`
	ContentTruncate                 int                `yaml:"content_truncate"`
	LLMTokenEnv                     string             `yaml:"llm_token_env"`
	MaxCardsPerArticle              int                `yaml:"max_cards_per_article"`
	EnableSemanticDedup             *bool              `yaml:"enable_semantic_dedup"`
	MapSnapshotOnRefresh            *bool              `yaml:"map_snapshot_on_refresh"`
	TopicMocEnabled                 *bool              `yaml:"topic_moc_enabled"`
	TopicMinCards                   int                `yaml:"topic_min_cards"`
	SkeletonChangeApprovalThreshold int                `yaml:"skeleton_change_approval_threshold"` // 骨架变更影响半径≤此值=小动作自动应用，>此值=大动作走 Matrix 批准
	ProposeMaxPerRound              int                `yaml:"propose_max_per_round"`              // 单轮涌现回流 propose 喂入的待归位卡上限（默认40）；超额按概念排序取前N、其余下轮，防一次喂满上下文致提议质量差
	GapFillPerRun                   int                `yaml:"gap_fill_per_run"`                   // 缺口填充每领域每轮上限（默认10）
	GapFillOrder                    string             `yaml:"gap_fill_order"`                     // breadth=自顶向下广度优先（先填根/主题层缺口）
	GapFillEnabled                  map[string]bool    `yaml:"gap_fill_enabled"`                   // 每领域开关（优先级最高，打样先只开一个域）
	GapFillEnabledAll               *bool              `yaml:"gap_fill_enabled_all"`               // 一键总开关
	GapFillDefault                  *bool              `yaml:"gap_fill_default"`                   // 未列入 gap_fill_enabled 的新领域默认开/关
	AuditOnRun                      *bool              `yaml:"audit_on_run"`
	Budget                          Budget             `yaml:"budget"`
	Retry                           Retry              `yaml:"retry"`
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
	VaultThreshold   float64  `yaml:"vault_threshold"`     // 0 = 用 defaults
	ArchiveThreshold float64  `yaml:"archive_threshold"`   // 0 = 用 defaults
	RelevanceGate    float64  `yaml:"relevance_gate"`      // 0 = 用 defaults（相关度门）
	VaultQuotaPerRun int      `yaml:"vault_quota_per_run"` // 每领域每轮 vault 上限，0=不限
	FeedSection      string   `yaml:"feed_section"`        // 资讯合并小节名，空=用 Display
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
	if d.RelevanceGate == 0 {
		d.RelevanceGate = 5.0
	}
	if d.RelevanceHardFloor == 0 {
		d.RelevanceHardFloor = 3.0
	}
	if d.AtomicPotentialBonus == 0 {
		d.AtomicPotentialBonus = 0.3
	}
	if d.AtomicPotentialBonusMin <= 0 {
		d.AtomicPotentialBonusMin = 8
	}
	if d.ReviewLedgerEnabled == nil {
		d.ReviewLedgerEnabled = boolPtr(true)
	}
	if d.SnapshotKeep <= 0 {
		d.SnapshotKeep = 5
	}
	if d.SnapshotWeekly == nil {
		d.SnapshotWeekly = boolPtr(true)
	}
	// content_type 加减分：补齐内置默认（用户在 yaml 里可只覆盖单项，其余保持默认）
	if d.ContentTypeAdjust == nil {
		d.ContentTypeAdjust = map[string]float64{}
	}
	for k, v := range defaultContentTypeAdjust {
		if _, ok := d.ContentTypeAdjust[k]; !ok {
			d.ContentTypeAdjust[k] = v
		}
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
	if d.PromoteEnabled == nil {
		d.PromoteEnabled = boolPtr(true)
	}
	if d.PromoteDurabilityMin <= 0 {
		d.PromoteDurabilityMin = 7.0
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
	if d.ProposeMaxPerRound <= 0 {
		d.ProposeMaxPerRound = 40
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

// RelevanceGateOr 领域相关度门优先（非0即用，含设负数关闭），否则用全局默认。
func (d Domain) RelevanceGateOr(def Defaults) float64 {
	if d.RelevanceGate != 0 {
		return d.RelevanceGate
	}
	return def.RelevanceGate
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

// defaultContentTypeAdjust content_type→final 加减分内置默认（可被 domains.yaml content_type_adjust 单项覆盖）。
// 资讯/营销降权、教程/论文/代码提权——把原先硬编码在 FinalScore 的调整搬出来，可调不改代码。
var defaultContentTypeAdjust = map[string]float64{
	"marketing": -2.0, "news": -1.0, "release": -0.5,
	"tutorial": 0.5, "paper": 0.5, "reference": 0.5,
	"code": 0.7, "poc": 0.7,
}

// GetReviewLedgerEnabled 拒收台账开关（默认开）：记录被拒/降级条目连同评分，供定期审阈值/查漏召。
func (d *Defaults) GetReviewLedgerEnabled() bool {
	if d.ReviewLedgerEnabled != nil {
		return *d.ReviewLedgerEnabled
	}
	return true
}

// GetSnapshotWeekly 同周 digest 快照覆盖为一份（默认开）：堵「一天多份全树快照」的冗余。
func (d *Defaults) GetSnapshotWeekly() bool {
	if d.SnapshotWeekly != nil {
		return *d.SnapshotWeekly
	}
	return true
}

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

// GetPromoteEnabled 资讯晋升闸总开关（默认开）：关掉则 feed 只生成资讯库存档、不晋升耐久知识点。
func (d *Defaults) GetPromoteEnabled() bool {
	if d.PromoteEnabled != nil {
		return *d.PromoteEnabled
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

// SetDomainScope 外科式地把 domains.yaml 中某领域的 scope（一句话「大方向」）改为新值：
// 逐行定位 → 替换/插入 scope 行，保留全部注释与排版（domains.yaml 注释密集，整体
// yaml.Marshal 回写会丢注释，故不用）。这是 Phase I 调方向掌舵面「设领域大方向」的落点。
// 资讯流（feed）/兜底（is_default）领域不生成骨架、不设 scope，调用会被拒。
// 改完下次 `pkb-curate skeleton <领域>` 运行即读到新值（无需重启/重编）。
func SetDomainScope(path, name, scope string) error {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return fmt.Errorf("scope 不能为空")
	}
	if strings.ContainsAny(scope, "\r\n") {
		return fmt.Errorf("scope 必须单行（一句话大方向）")
	}

	// 先用真解析校验领域存在且可设 scope（拒资讯流/兜底领域）。
	dc, err := LoadDomains(path)
	if err != nil {
		return err
	}
	var target *Domain
	for i := range dc.Domains {
		if dc.Domains[i].Name == name {
			target = &dc.Domains[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("领域 %s 不存在于 %s", name, path)
	}
	if target.Feed {
		return fmt.Errorf("领域 %s 是资讯流容器（feed），不生成骨架、不设 scope", name)
	}
	if target.IsDefault {
		return fmt.Errorf("领域 %s 是兜底领域（is_default），不生成骨架、不设 scope", name)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read domains config %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	// 定位目标 `- name: <name>` 块起点与 dash 缩进。
	blockStart, dashIndent := -1, 0
	for i, line := range lines {
		if ind, ok := matchDomainNameLine(line, name); ok {
			blockStart, dashIndent = i, ind
			break
		}
	}
	if blockStart < 0 {
		// LoadDomains 找到了但逐行没匹配到——格式异常（如 name 写在折叠块里），拒绝盲改。
		return fmt.Errorf("无法在 %s 定位领域 %s 的 `- name:` 行（格式异常，未改动）", path, name)
	}
	contentIndent := dashIndent + 2

	// 块范围 [blockStart+1, blockEnd)：到下一个 indent<=dashIndent 的非空非注释行为止。
	blockEnd := len(lines)
	for i := blockStart + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if leadingSpaces(lines[i]) <= dashIndent {
			blockEnd = i
			break
		}
	}

	rendered := strings.Repeat(" ", contentIndent) + "scope: " + yamlScalar(scope)

	// 块内找已有 scope 行 → 替换；否则在 display 行（无则 name 行）后插入，保持 name/display/scope 序。
	scopeLine, displayLine := -1, -1
	for i := blockStart + 1; i < blockEnd; i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "scope:") {
			scopeLine = i
			break
		}
		if strings.HasPrefix(t, "display:") {
			displayLine = i
		}
	}

	if scopeLine >= 0 {
		lines[scopeLine] = rendered
	} else {
		insertAfter := blockStart
		if displayLine >= 0 {
			insertAfter = displayLine
		}
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:insertAfter+1]...)
		out = append(out, rendered)
		out = append(out, lines[insertAfter+1:]...)
		lines = out
	}

	newContent := strings.Join(lines, "\n")

	// 安全网：回写前用真解析校验改后内容——目标 scope 正确、领域数不变，否则不写盘。
	var check DomainsConfig
	if err := yaml.Unmarshal([]byte(newContent), &check); err != nil {
		return fmt.Errorf("改后 YAML 解析失败（未写盘）：%w", err)
	}
	if len(check.Domains) != len(dc.Domains) {
		return fmt.Errorf("改后校验失败：领域数量 %d→%d（未写盘）", len(dc.Domains), len(check.Domains))
	}
	found := false
	for _, d := range check.Domains {
		if d.Name == name {
			if strings.TrimSpace(d.Scope) != scope {
				return fmt.Errorf("改后校验失败：领域 %s scope 不符（未写盘）", name)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("改后校验失败：领域 %s 丢失（未写盘）", name)
	}

	return writeFileAtomic(path, newContent)
}

// matchDomainNameLine 判断一行是否为 `<indent>- name: <name>`（容忍行尾注释/引号），返回 dash 缩进。
func matchDomainNameLine(line, name string) (int, bool) {
	ind := leadingSpaces(line)
	rest := line[ind:]
	if !strings.HasPrefix(rest, "- ") {
		return 0, false
	}
	rest = strings.TrimSpace(rest[2:])
	if !strings.HasPrefix(rest, "name:") {
		return 0, false
	}
	val := strings.TrimSpace(strings.TrimPrefix(rest, "name:"))
	if idx := strings.Index(val, " #"); idx >= 0 { // 去行尾注释
		val = strings.TrimSpace(val[:idx])
	}
	val = strings.Trim(val, `"'`)
	return ind, val == name
}

// leadingSpaces 数一行的前导空格数（YAML 缩进）。
func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

// yamlScalar 把字符串渲染成安全的 YAML 标量：plain-safe 则原样输出（不给中文句子平添引号），
// 否则双引号转义（含 `: ` / 前导指示符 / 引号 / 行尾注释等会破坏 plain 解析的情形）。
func yamlScalar(s string) string {
	if isPlainSafeYAML(s) {
		return s
	}
	esc := strings.ReplaceAll(s, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return `"` + esc + `"`
}

// isPlainSafeYAML 判断字符串能否作为 YAML plain 标量原样写出而不改变语义。
func isPlainSafeYAML(s string) bool {
	if s == "" || s != strings.TrimSpace(s) {
		return false
	}
	if strings.Contains(s, ": ") || strings.HasSuffix(s, ":") {
		return false
	}
	if strings.Contains(s, " #") {
		return false
	}
	if strings.ContainsAny(s, "\"'\t") {
		return false
	}
	switch s[0] { // 前导 YAML 指示符（中文/字母字节 >=0x80 或普通字母不在此列，安全）
	case '-', '?', ':', ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', '%', '@', '`', ' ':
		return false
	}
	return true
}

// AddDomain 向 domains.yaml 追加一个新知识领域（最小字段：display + scope）。name 取 display
// （标识符，允许中文）、vault_subpath = vault/<display>、keywords 留空（可后续补）。外科式追加到
// domains: 段末尾，保留全部注释。新域不自动生成骨架——由前端「生成骨架」或下次自动维护触发。
func AddDomain(path, display, scope string) error {
	display = strings.TrimSpace(display)
	scope = strings.TrimSpace(scope)
	if display == "" {
		return fmt.Errorf("领域显示名不能为空")
	}
	if scope == "" {
		return fmt.Errorf("领域大方向（scope）不能为空")
	}
	if strings.ContainsAny(display, "\r\n\t/\\:") || strings.Contains(display, "..") {
		return fmt.Errorf("领域显示名含非法字符（不可含 / \\ : 制表/换行 或 ..）")
	}
	if strings.ContainsAny(scope, "\r\n") {
		return fmt.Errorf("scope 必须单行")
	}

	dc, err := LoadDomains(path)
	if err != nil {
		return err
	}
	name := display // 标识符取显示名（允许中文 key）
	for _, d := range dc.Domains {
		if d.Name == name || d.Display == display {
			return fmt.Errorf("领域 %q 已存在", display)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read domains config %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	// 定位顶级 domains: 段 + 探测现有领域块的 dash 缩进。
	domainsKeyLine, dashIndent := -1, -1
	for i, line := range lines {
		if leadingSpaces(line) == 0 && strings.TrimSpace(line) == "domains:" {
			domainsKeyLine = i
			continue
		}
		if domainsKeyLine >= 0 && dashIndent < 0 {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "- ") {
				dashIndent = leadingSpaces(line)
			}
		}
	}
	if domainsKeyLine < 0 {
		return fmt.Errorf("domains.yaml 未找到顶级 domains: 段")
	}
	if dashIndent < 0 {
		dashIndent = 2 // 空列表，用默认缩进
	}
	contentIndent := dashIndent + 2

	// 追加点 = domains: 段结束（下一个顶级 key 或 EOF），回退过尾部空行。
	insertAt := len(lines)
	for i := domainsKeyLine + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if leadingSpaces(lines[i]) == 0 {
			insertAt = i
			break
		}
	}
	for insertAt > domainsKeyLine+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}

	dash := strings.Repeat(" ", dashIndent)
	cind := strings.Repeat(" ", contentIndent)
	block := []string{
		"",
		dash + "- name: " + yamlScalar(name),
		cind + "display: " + yamlScalar(display),
		cind + "scope: " + yamlScalar(scope),
		cind + "vault_subpath: " + yamlScalar("vault/"+display),
		cind + "keywords: []",
	}
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:insertAt]...)
	out = append(out, block...)
	out = append(out, lines[insertAt:]...)
	newContent := strings.Join(out, "\n")

	// 安全网：回写前真解析校验领域数 +1、新域字段正确。
	var check DomainsConfig
	if err := yaml.Unmarshal([]byte(newContent), &check); err != nil {
		return fmt.Errorf("改后 YAML 解析失败（未写盘）：%w", err)
	}
	if len(check.Domains) != len(dc.Domains)+1 {
		return fmt.Errorf("改后校验失败：领域数 %d→%d（未写盘）", len(dc.Domains), len(check.Domains))
	}
	ok := false
	for _, d := range check.Domains {
		if d.Name == name && d.Display == display && strings.TrimSpace(d.Scope) == scope {
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("改后校验失败：新域 %q 字段不符（未写盘）", display)
	}
	return writeFileAtomic(path, newContent)
}

// DeleteDomain 从 domains.yaml 移除一个领域条目（整块）。仅删配置——vault 下卡片/骨架文件
// 原样保留（浏览归 Obsidian、文件不丢）。兜底（is_default）与资讯流容器（feed）不可删。
func DeleteDomain(path, name string) error {
	dc, err := LoadDomains(path)
	if err != nil {
		return err
	}
	var target *Domain
	for i := range dc.Domains {
		if dc.Domains[i].Name == name {
			target = &dc.Domains[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("领域 %s 不存在", name)
	}
	if target.IsDefault {
		return fmt.Errorf("领域 %s 是兜底领域（is_default），不可删除", name)
	}
	if target.Feed {
		return fmt.Errorf("领域 %s 是资讯流容器（feed），不可删除", name)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read domains config %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	blockStart, dashIndent := -1, 0
	for i, line := range lines {
		if ind, ok := matchDomainNameLine(line, name); ok {
			blockStart, dashIndent = i, ind
			break
		}
	}
	if blockStart < 0 {
		return fmt.Errorf("无法在 %s 定位领域 %s（格式异常，未改动）", path, name)
	}
	// 块结束 = 下一个同缩进 `- ` 兄弟块 或 顶级 dedent key 或 EOF（块内/块后空行随块删）。
	blockEnd := len(lines)
	for i := blockStart + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ls := leadingSpaces(lines[i])
		if ls < dashIndent || (ls == dashIndent && strings.HasPrefix(t, "- ")) {
			blockEnd = i
			break
		}
	}

	out := make([]string, 0, len(lines))
	out = append(out, lines[:blockStart]...)
	out = append(out, lines[blockEnd:]...)
	newContent := strings.Join(out, "\n")

	var check DomainsConfig
	if err := yaml.Unmarshal([]byte(newContent), &check); err != nil {
		return fmt.Errorf("改后 YAML 解析失败（未写盘）：%w", err)
	}
	if len(check.Domains) != len(dc.Domains)-1 {
		return fmt.Errorf("改后校验失败：领域数 %d→%d（未写盘）", len(dc.Domains), len(check.Domains))
	}
	for _, d := range check.Domains {
		if d.Name == name {
			return fmt.Errorf("改后校验失败：领域 %s 仍存在（未写盘）", name)
		}
	}
	return writeFileAtomic(path, newContent)
}

// SetDomainDisplay 改某领域的显示名（display），仅改显示——内部 name / vault_subpath / 分类
// 关键词不动（用户决策：重命名仅改 display，零迁移）。外科式替换 display 行，保留注释。
func SetDomainDisplay(path, name, display string) error {
	display = strings.TrimSpace(display)
	if display == "" {
		return fmt.Errorf("显示名不能为空")
	}
	if strings.ContainsAny(display, "\r\n") {
		return fmt.Errorf("显示名必须单行")
	}
	dc, err := LoadDomains(path)
	if err != nil {
		return err
	}
	exists := false
	for _, d := range dc.Domains {
		if d.Name == name {
			exists = true
		} else if d.Display == display {
			return fmt.Errorf("显示名 %q 已被领域 %s 占用", display, d.Name)
		}
	}
	if !exists {
		return fmt.Errorf("领域 %s 不存在", name)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read domains config %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")

	blockStart, dashIndent := -1, 0
	for i, line := range lines {
		if ind, ok := matchDomainNameLine(line, name); ok {
			blockStart, dashIndent = i, ind
			break
		}
	}
	if blockStart < 0 {
		return fmt.Errorf("无法在 %s 定位领域 %s（格式异常，未改动）", path, name)
	}
	contentIndent := dashIndent + 2
	blockEnd := len(lines)
	for i := blockStart + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if leadingSpaces(lines[i]) <= dashIndent {
			blockEnd = i
			break
		}
	}

	rendered := strings.Repeat(" ", contentIndent) + "display: " + yamlScalar(display)
	displayLine := -1
	for i := blockStart + 1; i < blockEnd; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "display:") {
			displayLine = i
			break
		}
	}
	if displayLine >= 0 {
		lines[displayLine] = rendered
	} else {
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:blockStart+1]...)
		out = append(out, rendered)
		out = append(out, lines[blockStart+1:]...)
		lines = out
	}
	newContent := strings.Join(lines, "\n")

	var check DomainsConfig
	if err := yaml.Unmarshal([]byte(newContent), &check); err != nil {
		return fmt.Errorf("改后 YAML 解析失败（未写盘）：%w", err)
	}
	for _, d := range check.Domains {
		if d.Name == name {
			if strings.TrimSpace(d.Display) != display {
				return fmt.Errorf("改后校验失败：display 不符（未写盘）")
			}
			return writeFileAtomic(path, newContent)
		}
	}
	return fmt.Errorf("改后校验失败：领域 %s 丢失（未写盘）", name)
}
