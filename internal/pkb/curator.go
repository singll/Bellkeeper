package pkb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"
)

// Options pkb-curate 运行选项
type Options struct {
	ConfigDir string // config/pkb（含 domains.yaml + prompts/）
	DryRun    bool
	Rescan    bool // 全量重扫：包含已处理条目（默认 false=只取未处理，幂等）
	PerRun    int  // 0 = 用 domains.yaml defaults.per_run
	LLMJobs   *service.LLMJobQueueService
	Context   context.Context
}

// Curator 知识库维护编排器（一次性 CLI，跑完即退，无后台 goroutine）。
type Curator struct {
	client                *Client
	articleRepo           *repository.ArticleTagRepository
	domains               *DomainsConfig
	basePath              string // /mnt/knowledge
	scorePrompt           string
	reconstructPrompt     string
	digestPrompt          string
	digestTopicPrompt     string
	skeletonPrompt        string
	scorePromptName       string
	reconstructName       string
	digestPromptName      string
	digestTopicPromptName string
	skeletonPromptName    string
	dryRun                bool
	rescan                bool
	perRun                int
	ctx                   context.Context
	scoreCalls            int
	reconstructCalls      int
	digestCalls           int
	lastSummary           runSummary
	llmJobs               *service.LLMJobQueueService
}

// NewCurator 装配 Curator：加载 config/pkb + 构造 HTTP 客户端 + 注入 ArticleTag 仓库（幂等账本）。
func NewCurator(cfg *config.Config, opts Options, articleRepo *repository.ArticleTagRepository) (*Curator, error) {
	domains, err := LoadDomains(filepath.Join(opts.ConfigDir, "domains.yaml"))
	if err != nil {
		return nil, err
	}

	registry, err := LoadPromptRegistry(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	scorePrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Score)
	if err != nil {
		return nil, err
	}
	reconstructPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Reconstruct)
	if err != nil {
		return nil, err
	}
	digestPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Digest)
	if err != nil {
		return nil, err
	}
	digestTopicPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.DigestTopic)
	if err != nil {
		return nil, err
	}
	skeletonPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Skeleton)
	if err != nil {
		return nil, err
	}

	llmBase := cfg.Classify.LLMProxyURL
	if llmBase == "" {
		return nil, fmt.Errorf("classify.llm_proxy_url is empty; pkb-curate needs it as the LLM backend")
	}
	// 重构生成长卡片、rebuild 全量索引可能远超 classify.timeout(10s)；用独立较长超时（打分快，不受上限影响）
	timeout := 300 * time.Second
	llmKey := cfg.Server.APIKey
	if domains.Defaults.LLMTokenEnv != "" {
		if v := os.Getenv(domains.Defaults.LLMTokenEnv); v != "" {
			llmKey = v
		}
	}
	client := NewClient(llmBase, cfg.Server.APIKey, llmKey, timeout)

	perRun := opts.PerRun
	if perRun <= 0 {
		perRun = domains.Defaults.PerRun
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	return &Curator{
		client:                client,
		articleRepo:           articleRepo,
		domains:               domains,
		basePath:              cfg.Knowledge.BasePath,
		scorePrompt:           scorePrompt,
		reconstructPrompt:     reconstructPrompt,
		digestPrompt:          digestPrompt,
		digestTopicPrompt:     digestTopicPrompt,
		skeletonPrompt:        skeletonPrompt,
		scorePromptName:       registry.Active.Score,
		reconstructName:       registry.Active.Reconstruct,
		digestPromptName:      registry.Active.Digest,
		digestTopicPromptName: registry.Active.DigestTopic,
		skeletonPromptName:    registry.Active.Skeleton,
		dryRun:                opts.DryRun,
		rescan:                opts.Rescan,
		perRun:                perRun,
		ctx:                   ctx,
		llmJobs:               opts.LLMJobs,
	}, nil
}

// runSummary 本轮处理统计
type runSummary struct {
	processed int
	vault     int
	archive   int
	discard   int
	failed    int
	deferred  int
}

// Run 执行一轮维护：列举 → 读 → 打分 → 决策分流 → (高分)重构 → 落盘 → 重索引。
func (c *Curator) Run() error {
	mode := "LIVE"
	if c.dryRun {
		mode = "DRY-RUN（只打分打印，不写盘/不移动/不重索引）"
	}
	fmt.Printf("[pkb-curate] 模式=%s per_run=%d base_path=%s\n", mode, c.perRun, c.basePath)
	fmt.Printf("[pkb-curate] score_model=%s reconstruct_model=%s vault>=%.1f archive>=%.1f\n",
		c.domains.Defaults.ScoreModel, c.domains.Defaults.ReconstructModel,
		c.domains.Defaults.VaultThreshold, c.domains.Defaults.ArchiveThreshold)
	if c.llmJobs != nil {
		fmt.Printf("[pkb-curate] llm_mode=queued（经 llm_jobs 持久队列，由 server worker 调 LLM）\n")
	} else {
		fmt.Printf("[pkb-curate] llm_mode=direct（直接同步调 LLM Proxy）\n")
	}
	fmt.Printf("[pkb-curate] prompts score=%s reconstruct=%s budget(score=%d,reconstruct=%d) retry(attempts=%d,backoff=%ds,max=%ds,stop_on_rate_limit=%v)\n",
		c.scorePromptName, c.reconstructName,
		c.domains.Defaults.Budget.MaxScoreCallsPerRun,
		c.domains.Defaults.Budget.MaxReconstructCallsPerRun,
		c.domains.Defaults.Retry.MaxAttempts,
		c.domains.Defaults.Retry.InitialBackoffSeconds,
		c.domains.Defaults.Retry.MaxBackoffSeconds,
		c.domains.Defaults.Retry.GetStopRunOnRateLimit())

	articles, err := c.client.ListRaw(c.perRun, !c.rescan)
	if err != nil {
		return fmt.Errorf("list raw articles: %w", err)
	}
	fmt.Printf("[pkb-curate] 取到 %d 篇 raw 待处理（rescan=%v）\n", len(articles), c.rescan)

	var sum runSummary
	for i, art := range articles {
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(articles), art.Title)
		if err := c.processOne(art, &sum); err != nil {
			if c.domains.Defaults.Retry.GetStopRunOnRateLimit() && isRetryableLLMError(err) {
				sum.deferred = len(articles) - i
				fmt.Printf("    ↷ LLM 免费池/上游仍在限流，本轮停止；当前及后续 %d 篇保留在 raw 队列，下轮继续: %v\n", sum.deferred, err)
				break
			}
			if isBudgetExhausted(err) {
				sum.deferred = len(articles) - i
				fmt.Printf("    ↷ 本轮大模型预算已用尽；当前及后续 %d 篇保留在 raw 队列，下轮继续: %v\n", sum.deferred, err)
				break
			}
			sum.failed++
			fmt.Printf("    ✗ 处理失败（跳过，不中断整批）: %v\n", err)
		}
	}

	if !c.dryRun && (sum.vault > 0 || sum.archive > 0) {
		fmt.Printf("\n[pkb-curate] 触发 rebuild 与文件系统对齐...\n")
		if err := c.client.Rebuild(); err != nil {
			fmt.Printf("[pkb-curate] ⚠ rebuild 失败（已落盘文件不受影响，可稍后手动 rebuild）: %v\n", err)
		}
	}

	c.lastSummary = sum
	fmt.Printf("\n[pkb-curate] 本轮完成：处理 %d / vault %d / archive %d / discard %d / 失败 %d / 延期 %d\n",
		sum.processed, sum.vault, sum.archive, sum.discard, sum.failed, sum.deferred)
	if c.domains.Defaults.GetAuditOnRun() && !c.dryRun {
		fmt.Printf("[pkb-audit] 网健康度摘要: %s\n", c.AuditSummary())
	}
	return nil
}

// processOne 处理单篇：读正文 → 打分 → 决策分流。
func (c *Curator) processOne(art ArticleMeta, sum *runSummary) error {
	if art.FilePath == "" {
		return fmt.Errorf("article %s has empty file_path", art.DocumentID)
	}
	bodyBytes, err := os.ReadFile(art.FilePath)
	if err != nil {
		// 自愈①：raw 文件已不在（上轮分流移走但 DB 账本未对齐）→ 回填账本并跳过，不计失败
		if os.IsNotExist(err) {
			fmt.Printf("    ↪ raw 文件已不存在（疑似上轮已分流），回填账本并跳过\n")
			c.markProcessedDB(art.ID, "moved", 0, "", "")
			return nil
		}
		return fmt.Errorf("read body %s: %w", art.FilePath, err)
	}
	body := string(bodyBytes)

	// 自愈②：frontmatter 已有 pkb_decision（上轮处理过、仅 DB 账本未对齐）→ 回填并跳过，不重打分
	if dec := frontmatterValue(body, "pkb_decision"); dec != "" {
		fmt.Printf("    ↪ 已有 pkb_decision=%s，回填账本并跳过\n", dec)
		c.markProcessedDB(art.ID, dec, 0, "", "")
		return nil
	}

	score, err := c.scoreArticle(art, body)
	if err != nil {
		return err
	}
	final := score.FinalScore(c.domains.Defaults.Weights)
	domain := c.domains.ResolveDomain(score.MatchedDomains)
	sum.processed++

	fmt.Printf("    打分 rel=%d depth=%d action=%d durable=%d novelty=%d type=%s → final=%.1f 领域=%s\n",
		score.Relevance, score.Depth, score.Actionability, score.Durability, score.Novelty, score.ContentType, final, domain.Name)
	if score.Reason != "" {
		fmt.Printf("    依据=%s\n", score.Reason)
	}

	switch {
	case final < domain.ArchiveThresholdOr(c.domains.Defaults):
		fmt.Printf("    决策=discard（保留 raw，仅标记 frontmatter）\n")
		sum.discard++
		return c.markDiscard(art, score, final, domain)
	case final < domain.VaultThresholdOr(c.domains.Defaults):
		fmt.Printf("    决策=archive\n")
		if err := c.moveToArchive(art, body, score, final, domain); err != nil {
			return err
		}
		sum.archive++
		return nil
	default:
		fmt.Printf("    决策=vault（原子化重构）\n")
		if err := c.reconstructToVault(art, body, score, final, domain); err != nil {
			return err
		}
		sum.vault++
		return nil
	}
}

// markDiscard 低分：保留 raw 原文，仅在 frontmatter 标记决策（可溯源、可调阈值后重评）。
func (c *Curator) markDiscard(art ArticleMeta, score *ScoreResult, final float64, domain Domain) error {
	if c.dryRun {
		return nil
	}
	body, err := os.ReadFile(art.FilePath)
	if err != nil {
		return err
	}
	updated := upsertFrontmatter(string(body), c.scoreFields(score, final, domain, "discard"))
	if err := os.WriteFile(art.FilePath, []byte(updated), 0644); err != nil {
		return err
	}
	c.markProcessedDB(art.ID, "discard", final, "", "")
	return nil
}

// moveToArchive 中分：os.Rename raw→archive/，并在 frontmatter 写入打分/决策。
func (c *Curator) moveToArchive(art ArticleMeta, body string, score *ScoreResult, final float64, domain Domain) error {
	if c.dryRun {
		return nil
	}
	archiveDir := filepath.Join(c.basePath, "archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("mkdir archive: %w", err)
	}
	dst := filepath.Join(archiveDir, filepath.Base(art.FilePath))
	if err := os.Rename(art.FilePath, dst); err != nil {
		return fmt.Errorf("move to archive: %w", err)
	}
	updated := upsertFrontmatter(body, c.scoreFields(score, final, domain, "archive"))
	if err := os.WriteFile(dst, []byte(updated), 0644); err != nil {
		return fmt.Errorf("write archive frontmatter: %w", err)
	}
	fmt.Printf("    → %s\n", dst)
	c.markProcessedDB(art.ID, "archive", final, "archive", dst)
	return nil
}

// reconstructToVault 高分：检索 wikilink 候选 → 重构成多张原子卡片 → 写入 vault 子目录。
func (c *Curator) reconstructToVault(art ArticleMeta, body string, score *ScoreResult, final float64, domain Domain) error {
	if c.dryRun {
		fmt.Printf("    (dry-run) 跳过重构与写盘；将写入 %s/\n", domain.VaultSubpath)
		return nil
	}
	date := time.Now().Format("20060102")

	candidates, err := c.client.SearchTitles(art.Title+" "+domain.Display, []string{"vault"}, 15)
	if err != nil {
		fmt.Printf("    ⚠ 检索 wikilink 候选失败（继续，无候选）: %v\n", err)
		candidates = nil
	}

	var contentCandidates []ContentMatch
	if c.domains.Defaults.GetEnableSemanticDedup() {
		coreExcerpt := truncateRunes(stripFrontmatter(body), 200)
		cc, cerr := c.client.SearchContent(coreExcerpt, []string{"vault"}, 5)
		if cerr != nil {
			fmt.Printf("    ⚠ 语义查重失败（继续）: %v\n", cerr)
		} else {
			contentCandidates = cc
		}
	}

	mergedCandidates, mergedDisplayCandidates := mergeCandidates(candidates, contentCandidates)

	cards, reconStats, err := c.reconstructCard(art, body, score, domain, mergedCandidates, mergedDisplayCandidates, date)
	if err != nil {
		return err
	}

	dir := filepath.Join(c.basePath, domain.VaultSubpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir vault subpath: %w", err)
	}

	written := 0
	skipped := 0
	var errors []string

	for _, card := range cards {
		concept := frontmatterValue(card, "atomic_concept")
		if concept == "" {
			title := frontmatterValue(card, "title")
			if title == "" {
				title = art.Title
			}
			concept = title
		}
		slug := sanitizeFilename(concept)
		dst := filepath.Join(dir, slug+".md")

		if _, statErr := os.Stat(dst); statErr == nil {
			for n := 2; n <= 100; n++ {
				alt := fmt.Sprintf("%s-%d.md", slug, n)
				altPath := filepath.Join(dir, alt)
				if _, statErr2 := os.Stat(altPath); statErr2 != nil {
					dst = altPath
					card = appendAlias(card, slug)
					break
				}
			}
		}

		tmpPath := dst + ".tmp.md"
		if err := os.WriteFile(tmpPath, []byte(card), 0644); err != nil {
			errors = append(errors, fmt.Sprintf("write tmp %s: %v", tmpPath, err))
			skipped++
			continue
		}
		if err := os.Rename(tmpPath, dst); err != nil {
			_ = os.Remove(tmpPath)
			errors = append(errors, fmt.Sprintf("rename %s→%s: %v", tmpPath, dst, err))
			skipped++
			continue
		}
		fmt.Printf("    → %s\n", dst)
		written++
	}

	if rawBody, rerr := os.ReadFile(art.FilePath); rerr == nil {
		extraFields := c.scoreFields(score, final, domain, "vault")
		extraFields["pkb_cards_written"] = strconv.Itoa(written)
		extraFields["pkb_cards_skipped"] = strconv.Itoa(skipped)
		if reconStats.FailedValidate > 0 {
			errors = append(errors, fmt.Sprintf("validate_failed=%d", reconStats.FailedValidate))
		}
		if reconStats.Truncated > 0 {
			errors = append(errors, fmt.Sprintf("truncated_by_max_cards=%d", reconStats.Truncated))
		}
		if len(errors) > 0 {
			extraFields["pkb_reconstruct_errors"] = strings.Join(errors, "; ")
		}
		updated := upsertFrontmatter(string(rawBody), extraFields)
		if werr := os.WriteFile(art.FilePath, []byte(updated), 0644); werr != nil {
			fmt.Printf("    ⚠ 回写 raw frontmatter 失败（不影响 vault 卡片）: %v\n", werr)
		}
	} else if rerr != nil && !os.IsNotExist(rerr) {
		fmt.Printf("    ⚠ 读取 raw 回写失败: %v\n", rerr)
	}
	c.markProcessedDB(art.ID, "vault", final, "", "")
	return nil
}

// markProcessedDB 在 DB 账本中标记一篇已被处理（幂等核心）。dry-run 或无 repo 时跳过；
// 失败仅记录警告、不中断（文件已落位，下次运行靠自愈再对齐）。
func (c *Curator) markProcessedDB(id uint, decision string, score float64, newLayer, newFilePath string) {
	if c.dryRun || c.articleRepo == nil {
		return
	}
	if err := c.articleRepo.MarkPkbProcessed(id, decision, score, newLayer, newFilePath); err != nil {
		fmt.Printf("    ⚠ 标记 DB 处理状态失败（文件已落位，下次自愈）: %v\n", err)
	}
}

// frontmatterValue 从 markdown 顶部 YAML frontmatter 提取指定 key 的原始值（简单解析，仅供自愈判断）。
func frontmatterValue(content, key string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+":"))
		}
	}
	return ""
}

// scoreFields 生成要写入 frontmatter 的打分字段（pkb_ 前缀，避免与既有字段冲突）。
func (c *Curator) scoreFields(score *ScoreResult, final float64, domain Domain, decision string) map[string]string {
	return map[string]string{
		"pkb_score":     fmt.Sprintf("%.1f", final),
		"pkb_decision":  decision,
		"pkb_domain":    domain.Name,
		"pkb_type":      score.ContentType,
		"pkb_scored_at": time.Now().Format(time.RFC3339),
	}
}

// upsertFrontmatter 在 markdown 的 YAML frontmatter 中插入/更新若干字段；无 frontmatter 则新建。
func upsertFrontmatter(content string, fields map[string]string) string {
	lines := strings.Split(content, "\n")
	hasFM := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"

	if !hasFM {
		var b strings.Builder
		b.WriteString("---\n")
		for k, v := range fields {
			b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
		b.WriteString("---\n\n")
		b.WriteString(content)
		return b.String()
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return content // frontmatter 未闭合，保守不动
	}

	remaining := make(map[string]string, len(fields))
	for k, v := range fields {
		remaining[k] = v
	}
	for i := 1; i < end; i++ {
		key := strings.TrimSpace(strings.SplitN(lines[i], ":", 2)[0])
		if v, ok := remaining[key]; ok {
			lines[i] = fmt.Sprintf("%s: %s", key, v)
			delete(remaining, key)
		}
	}
	var insert []string
	for k, v := range remaining {
		insert = append(insert, fmt.Sprintf("%s: %s", k, v))
	}
	out := append([]string{}, lines[:end]...)
	out = append(out, insert...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

var unsafeFnameRe = regexp.MustCompile(`[/\\:*?"<>|\n\r\t]+`)

// sanitizeFilename 清理文件名非法字符，并按 rune 截断（避免截断到中文字符中间）。
func sanitizeFilename(s string) string {
	s = strings.TrimSpace(unsafeFnameRe.ReplaceAllString(s, "_"))
	if r := []rune(s); len(r) > 60 {
		s = strings.TrimSpace(string(r[:60]))
	}
	if s == "" {
		s = "untitled"
	}
	return s
}

// mergeCandidates 合并标题级候选与内容级候选，去重后返回两个列表：
// - 第一个只含裸 concept（供 buildValidLinkSet 做 prune 有效集）
// - 第二个含带摘要的展示串（供提示词渲染，让 LLM 看到上下文）
func mergeCandidates(titleCandidates []string, contentCandidates []ContentMatch) ([]string, []string) {
	seen := make(map[string]bool)
	var concepts []string
	var display []string
	addConcept := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			concepts = append(concepts, s)
		}
	}
	addDisplay := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			display = append(display, s)
		}
	}
	for _, t := range titleCandidates {
		addConcept(t)
		addDisplay(t)
	}
	for _, cc := range contentCandidates {
		addConcept(cc.Concept)
		if cc.Excerpt != "" {
			addDisplay(fmt.Sprintf("%s（%s）", cc.Concept, cc.Excerpt))
		} else {
			addDisplay(cc.Concept)
		}
	}
	return concepts, display
}

// appendAlias 在 frontmatter 的 aliases 中追加一个值（不覆盖已有 aliases）。
func appendAlias(card string, alias string) string {
	existing := frontmatterValue(card, "aliases")
	if existing == "" || existing == "[]" {
		return upsertFrontmatter(card, map[string]string{"aliases": "[" + alias + "]"})
	}
	existing = strings.Trim(existing, "[]")
	parts := strings.Split(existing, ",")
	for _, p := range parts {
		if strings.TrimSpace(strings.Trim(p, `"'`)) == alias {
			return card
		}
	}
	merged := "[" + strings.TrimSpace(existing) + ", " + alias + "]"
	return upsertFrontmatter(card, map[string]string{"aliases": merged})
}
