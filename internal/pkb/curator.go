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
	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/repository"
)

// Options pkb-curate 运行选项
type Options struct {
	ConfigDir string // config/pkb（含 domains.yaml + prompts/）
	DryRun    bool
	Rescan    bool // 全量重扫：包含已处理条目（默认 false=只取未处理，幂等）
	PerRun    int  // 0 = 用 domains.yaml defaults.per_run
	LLMJobs   *llmgateway.LLMJobQueueService
	Context   context.Context
	// DomainRepo 供缺口填充 G3 冷却让路查 crawl_domain_profiles.next_allowed_at；仅 fill 子命令注入，其余命令为 nil。
	DomainRepo *repository.CrawlDomainProfileRepository
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
	matchPrompt           string
	proposePrompt         string
	gapfillPrompt         string
	verifyPrompt          string
	feedPrompt            string
	promotePrompt         string
	scorePromptName       string
	reconstructName       string
	digestPromptName      string
	digestTopicPromptName string
	skeletonPromptName    string
	matchPromptName       string
	proposePromptName     string
	gapfillPromptName     string
	verifyPromptName      string
	feedPromptName        string
	promotePromptName     string
	dryRun                bool
	rescan                bool
	perRun                int
	ctx                   context.Context
	scoreCalls            int
	reconstructCalls      int
	digestCalls           int
	vaultCount            map[string]int // 本轮各领域已进 vault 数（领域配额护栏，每轮 Run 重置）
	lastSummary           runSummary
	llmJobs               *llmgateway.LLMJobQueueService
	domainRepo            *repository.CrawlDomainProfileRepository
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
	matchPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Match)
	if err != nil {
		return nil, err
	}
	proposePrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.SkeletonPropose)
	if err != nil {
		return nil, err
	}
	gapfillPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Gapfill)
	if err != nil {
		return nil, err
	}
	verifyPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Verify)
	if err != nil {
		return nil, err
	}
	feedPrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Feed)
	if err != nil {
		return nil, err
	}
	promotePrompt, err := loadPromptFile(opts.ConfigDir, registry.Active.Promote)
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
		matchPrompt:           matchPrompt,
		proposePrompt:         proposePrompt,
		gapfillPrompt:         gapfillPrompt,
		verifyPrompt:          verifyPrompt,
		feedPrompt:            feedPrompt,
		promotePrompt:         promotePrompt,
		scorePromptName:       registry.Active.Score,
		reconstructName:       registry.Active.Reconstruct,
		digestPromptName:      registry.Active.Digest,
		digestTopicPromptName: registry.Active.DigestTopic,
		skeletonPromptName:    registry.Active.Skeleton,
		matchPromptName:       registry.Active.Match,
		proposePromptName:     registry.Active.SkeletonPropose,
		gapfillPromptName:     registry.Active.Gapfill,
		verifyPromptName:      registry.Active.Verify,
		feedPromptName:        registry.Active.Feed,
		promotePromptName:     registry.Active.Promote,
		dryRun:                opts.DryRun,
		rescan:                opts.Rescan,
		perRun:                perRun,
		ctx:                   ctx,
		vaultCount:            map[string]int{},
		llmJobs:               opts.LLMJobs,
		domainRepo:            opts.DomainRepo,
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

	c.vaultCount = map[string]int{} // 领域配额每轮重置
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
	final := score.FinalScore(c.domains.Defaults)
	domain := c.domains.ResolveDomain(score.MatchedDomains)
	sum.processed++

	fmt.Printf("    打分 rel=%d depth=%d action=%d durable=%d novelty=%d atomic=%d type=%s → final=%.1f 领域=%s\n",
		score.Relevance, score.Depth, score.Actionability, score.Durability, score.Novelty, score.AtomicPotential, score.ContentType, final, domain.Name)
	if score.Reason != "" {
		fmt.Printf("    依据=%s\n", score.Reason)
	}

	decision, gate := c.decide(score, final, domain)
	c.appendReviewLedger(art, score, final, domain, decision, gate)
	switch decision {
	case "discard":
		if gate == "hard_floor" {
			fmt.Printf("    决策=discard（相关度 %d < 硬地板 %.0f，离题直接弃）\n", score.Relevance, c.domains.Defaults.RelevanceHardFloor)
		} else {
			fmt.Printf("    决策=discard（final %.1f < archive 阈值，保留 raw 仅标记）\n", final)
		}
		sum.discard++
		return c.markDiscard(art, score, final, domain)
	case "archive":
		switch gate {
		case "gate":
			fmt.Printf("    决策=archive（相关度门：rel %d < 门 %.0f，达 vault 线但离题，封顶降级）\n", score.Relevance, domain.RelevanceGateOr(c.domains.Defaults))
		case "quota":
			fmt.Printf("    决策=archive（领域配额：%s 本轮 vault 已达上限 %d，降级）\n", domain.Name, domain.VaultQuotaPerRun)
		default:
			fmt.Printf("    决策=archive\n")
		}
		if err := c.moveToArchive(art, body, score, final, domain); err != nil {
			return err
		}
		sum.archive++
		return nil
	default: // vault
		fmt.Printf("    决策=vault（原子化重构）\n")
		if err := c.reconstructToVault(art, body, score, final, domain); err != nil {
			return err
		}
		c.vaultCount[domain.Name]++
		sum.vault++
		return nil
	}
}

// decide 决策分流：相关度硬地板 → discard；常规阈值 → discard/archive/vault；达 vault 线但
// 相关度不足门 → 封顶降级 archive（堵离题高分噪音）；vault 超领域配额 → 降级 archive。
// 返回 (决策, 门命中标记)；标记 hard_floor/gate/quota/"" 供日志与拒收台账。
func (c *Curator) decide(score *ScoreResult, final float64, domain Domain) (string, string) {
	def := c.domains.Defaults
	rel := float64(score.Relevance)

	// 1) 相关度硬地板：与配置领域基本无关 → 直接 discard，无论其它维度多高（堵离群科普噪音根因）。
	if def.RelevanceHardFloor > 0 && rel < def.RelevanceHardFloor {
		return "discard", "hard_floor"
	}
	// 2) 常规阈值分流。
	switch {
	case final < domain.ArchiveThresholdOr(def):
		return "discard", ""
	case final < domain.VaultThresholdOr(def):
		return "archive", ""
	}
	// 3) 达 vault 线：相关度门 + 领域配额两道封顶（保留可溯源，降级 archive 而非弃）。
	if gate := domain.RelevanceGateOr(def); gate > 0 && rel < gate {
		return "archive", "gate"
	}
	if domain.VaultQuotaPerRun > 0 && c.vaultCount[domain.Name] >= domain.VaultQuotaPerRun {
		return "archive", "quota"
	}
	return "vault", ""
}

// appendReviewLedger 把被拒(discard)/降级(archive) 条目连同评分 append 到
// vault/_拒收台账/<YYYY-MM>.md，供定期审阈值（漏召→放宽 gate/阈值；噪音漏进→收紧）。
// vault 正常入库不记；dry-run 或台账关闭时跳过。落账失败仅告警、不中断分流。
func (c *Curator) appendReviewLedger(art ArticleMeta, score *ScoreResult, final float64, domain Domain, decision, gate string) {
	if c.dryRun || !c.domains.Defaults.GetReviewLedgerEnabled() || decision == "vault" {
		return
	}
	now := time.Now()
	dir := filepath.Join(c.basePath, "vault", "_拒收台账")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("    ⚠ 拒收台账 mkdir 失败（跳过记账，不影响分流）: %v\n", err)
		return
	}
	path := filepath.Join(dir, now.Format("2006-01")+".md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := fmt.Sprintf("# 拒收台账 %s\n\n> PKB 漏斗被拒/降级条目（连同评分），供定期审阈值与查漏召。"+
			"门：hard_floor=离题直接弃 / gate=达 vault 线但相关度不足降级 / quota=领域配额降级 / 空=常规阈值。\n\n"+
			"| 时间 | 决策 | 门 | 领域 | rel | dep | act | dur | nov | atom | type | final | 标题 | 来源 |\n"+
			"|------|------|----|------|-----|-----|-----|-----|-----|------|------|-------|------|------|\n", now.Format("2006-01"))
		if err := os.WriteFile(path, []byte(header), 0644); err != nil {
			fmt.Printf("    ⚠ 拒收台账写表头失败（跳过记账）: %v\n", err)
			return
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("    ⚠ 拒收台账打开失败（跳过记账）: %v\n", err)
		return
	}
	defer f.Close()
	row := fmt.Sprintf("| %s | %s | %s | %s | %d | %d | %d | %d | %d | %d | %s | %.1f | %s | %s |\n",
		now.Format("2006-01-02 15:04"), decision, gate, domain.Name,
		score.Relevance, score.Depth, score.Actionability, score.Durability, score.Novelty, score.AtomicPotential,
		ledgerCell(score.ContentType), final, ledgerCell(art.Title), ledgerCell(art.URL))
	if _, err := f.WriteString(row); err != nil {
		fmt.Printf("    ⚠ 拒收台账写入失败（跳过记账）: %v\n", err)
	}
}

// ledgerCell 清理台账单元格：替换会破坏 Markdown 表格的 | 与换行。
func ledgerCell(s string) string {
	s = strings.ReplaceAll(s, "|", "／")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
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
