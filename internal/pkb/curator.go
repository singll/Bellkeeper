package pkb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

// Options pkb-curate 运行选项
type Options struct {
	ConfigDir string // config/pkb（含 domains.yaml + prompts/）
	DryRun    bool
	PerRun    int // 0 = 用 domains.yaml defaults.per_run
}

// Curator 知识库维护编排器（一次性 CLI，跑完即退，无后台 goroutine）。
type Curator struct {
	client            *Client
	domains           *DomainsConfig
	basePath          string // /mnt/knowledge
	scorePrompt       string
	reconstructPrompt string
	dryRun            bool
	perRun            int
}

// NewCurator 装配 Curator：加载 config/pkb + 构造 HTTP 客户端。
func NewCurator(cfg *config.Config, opts Options) (*Curator, error) {
	domains, err := LoadDomains(filepath.Join(opts.ConfigDir, "domains.yaml"))
	if err != nil {
		return nil, err
	}

	scorePrompt, err := os.ReadFile(filepath.Join(opts.ConfigDir, "prompts", "score.md"))
	if err != nil {
		return nil, fmt.Errorf("read score prompt: %w", err)
	}
	reconstructPrompt, err := os.ReadFile(filepath.Join(opts.ConfigDir, "prompts", "reconstruct.md"))
	if err != nil {
		return nil, fmt.Errorf("read reconstruct prompt: %w", err)
	}

	llmBase := cfg.Classify.LLMProxyURL
	if llmBase == "" {
		return nil, fmt.Errorf("classify.llm_proxy_url is empty; pkb-curate needs it as the LLM backend")
	}
	// 重构生成长卡片、rebuild 全量索引可能远超 classify.timeout(10s)；用独立较长超时（打分快，不受上限影响）
	timeout := 300 * time.Second
	client := NewClient(llmBase, cfg.Server.APIKey, timeout)

	perRun := opts.PerRun
	if perRun <= 0 {
		perRun = domains.Defaults.PerRun
	}

	return &Curator{
		client:            client,
		domains:           domains,
		basePath:          cfg.Knowledge.BasePath,
		scorePrompt:       string(scorePrompt),
		reconstructPrompt: string(reconstructPrompt),
		dryRun:            opts.DryRun,
		perRun:            perRun,
	}, nil
}

// runSummary 本轮处理统计
type runSummary struct {
	processed int
	vault     int
	archive   int
	discard   int
	failed    int
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

	articles, err := c.client.ListRaw(c.perRun)
	if err != nil {
		return fmt.Errorf("list raw articles: %w", err)
	}
	fmt.Printf("[pkb-curate] 取到 %d 篇 raw 待处理\n", len(articles))

	var sum runSummary
	for i, art := range articles {
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(articles), art.Title)
		if err := c.processOne(art, &sum); err != nil {
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

	fmt.Printf("\n[pkb-curate] 本轮完成：处理 %d / vault %d / archive %d / discard %d / 失败 %d\n",
		sum.processed, sum.vault, sum.archive, sum.discard, sum.failed)
	return nil
}

// processOne 处理单篇：读正文 → 打分 → 决策分流。
func (c *Curator) processOne(art ArticleMeta, sum *runSummary) error {
	if art.FilePath == "" {
		return fmt.Errorf("article %s has empty file_path", art.DocumentID)
	}
	bodyBytes, err := os.ReadFile(art.FilePath)
	if err != nil {
		return fmt.Errorf("read body %s: %w", art.FilePath, err)
	}
	body := string(bodyBytes)

	score, err := c.scoreArticle(art, body)
	if err != nil {
		return err
	}
	final := score.FinalScore(c.domains.Defaults.Weights)
	domain := c.domains.ResolveDomain(score.MatchedDomains)
	sum.processed++

	fmt.Printf("    打分 rel=%d depth=%d action=%d → final=%.1f 领域=%s\n",
		score.Relevance, score.Depth, score.Actionability, final, domain.Name)

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
	return os.WriteFile(art.FilePath, []byte(updated), 0644)
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
	return nil
}

// reconstructToVault 高分：检索 wikilink 候选 → 重构成卡片 → 写入 vault 子目录（raw 原文留底）。
func (c *Curator) reconstructToVault(art ArticleMeta, body string, score *ScoreResult, final float64, domain Domain) error {
	if c.dryRun {
		fmt.Printf("    (dry-run) 跳过重构与写盘；将写入 %s/\n", domain.VaultSubpath)
		return nil
	}
	date := time.Now().Format("20060102")

	candidates, err := c.client.SearchTitles(domain.Display, []string{"vault"}, 8)
	if err != nil {
		fmt.Printf("    ⚠ 检索 wikilink 候选失败（继续，无候选）: %v\n", err)
		candidates = nil
	}

	card, err := c.reconstructCard(art, body, score, domain, candidates, date)
	if err != nil {
		return err
	}

	dir := filepath.Join(c.basePath, domain.VaultSubpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir vault subpath: %w", err)
	}
	dst := filepath.Join(dir, fmt.Sprintf("%s_%s.md", date, sanitizeFilename(art.Title)))
	if err := os.WriteFile(dst, []byte(card), 0644); err != nil {
		return fmt.Errorf("write vault card: %w", err)
	}
	fmt.Printf("    → %s\n", dst)
	return nil
}

// scoreFields 生成要写入 frontmatter 的打分字段（pkb_ 前缀，避免与既有字段冲突）。
func (c *Curator) scoreFields(score *ScoreResult, final float64, domain Domain, decision string) map[string]string {
	return map[string]string{
		"pkb_score":     fmt.Sprintf("%.1f", final),
		"pkb_decision":  decision,
		"pkb_domain":    domain.Name,
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
