package pkb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DigestOptions controls one low-frequency global-structure synthesis pass.
type DigestOptions struct {
	Domain   string
	Period   string
	Since    string
	MaxCards int
	DryRun   bool
}

type digestCard struct {
	Title    string
	RelPath  string
	Score    float64
	Tags     string
	Date     time.Time
	Excerpt  string
	FilePath string
}

// RunDigest builds a domain-level digest note from existing high-value vault cards.
func (c *Curator) RunDigest(opts DigestOptions) error {
	period := opts.Period
	if period == "" {
		period = "weekly"
	}
	if period != "weekly" && period != "monthly" {
		return fmt.Errorf("period must be weekly or monthly")
	}
	maxCards := opts.MaxCards
	if maxCards <= 0 {
		maxCards = 50
	}

	domains := c.domains.Domains
	if opts.Domain != "" && opts.Domain != "all" {
		d, ok := c.domains.FindDomain(opts.Domain)
		if !ok {
			return fmt.Errorf("unknown domain: %s", opts.Domain)
		}
		domains = []Domain{d}
	}

	since, err := digestSince(period, opts.Since)
	if err != nil {
		return err
	}

	fmt.Printf("[pkb-digest] 模式=%s period=%s since=%s max_cards=%d model=%s prompt=%s\n",
		digestMode(opts.DryRun), period, since.Format("2006-01-02"), maxCards,
		c.domains.Defaults.DigestModel, c.digestPromptName)

	var generated int
	for _, domain := range domains {
		cards, err := c.collectDigestCards(domain, since, maxCards)
		if err != nil {
			fmt.Printf("[pkb-digest] ⚠ %s 收集失败: %v\n", domain.Name, err)
			continue
		}
		if len(cards) == 0 {
			fmt.Printf("[pkb-digest] %s 无符合条件卡片，跳过\n", domain.Name)
			continue
		}
		fmt.Printf("[pkb-digest] %s 候选卡片=%d\n", domain.Name, len(cards))
		if opts.DryRun {
			for _, card := range cards {
				fmt.Printf("  - %.1f %s (%s)\n", card.Score, card.Title, card.RelPath)
			}
			continue
		}
		if err := c.writeDigest(domain, period, cards); err != nil {
			if c.domains.Defaults.Retry.StopRunOnRateLimit && isRetryableLLMError(err) {
				fmt.Printf("[pkb-digest] ↷ LLM 免费池/上游仍在限流，本轮停止；剩余领域下轮继续: %v\n", err)
				break
			}
			if isBudgetExhausted(err) {
				fmt.Printf("[pkb-digest] ↷ 本轮 digest 预算已用尽；剩余领域下轮继续: %v\n", err)
				break
			}
			fmt.Printf("[pkb-digest] ⚠ %s 生成失败: %v\n", domain.Name, err)
			continue
		}
		generated++
	}

	if !opts.DryRun && generated > 0 {
		fmt.Printf("[pkb-digest] 触发 rebuild 与文件系统对齐...\n")
		if err := c.client.Rebuild(); err != nil {
			fmt.Printf("[pkb-digest] ⚠ rebuild 失败（digest 已写盘，可稍后手动 rebuild）: %v\n", err)
		}
	}
	fmt.Printf("[pkb-digest] 完成：生成 %d 篇 digest\n", generated)
	return nil
}

func digestMode(dryRun bool) string {
	if dryRun {
		return "DRY-RUN"
	}
	return "LIVE"
}

func digestSince(period, raw string) (time.Time, error) {
	if raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse since date: %w", err)
		}
		return t, nil
	}
	now := time.Now()
	switch period {
	case "monthly":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), nil
	default:
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := now.AddDate(0, 0, -(weekday - 1))
		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, now.Location()), nil
	}
}

func (c *Curator) collectDigestCards(domain Domain, since time.Time, maxCards int) ([]digestCard, error) {
	root := filepath.Join(c.basePath, domain.VaultSubpath)
	var cards []digestCard
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		relWithin, _ := filepath.Rel(root, path)
		if strings.HasPrefix(relWithin, "digest"+string(os.PathSeparator)) ||
			strings.HasPrefix(relWithin, "maps"+string(os.PathSeparator)) ||
			strings.HasPrefix(relWithin, "topics"+string(os.PathSeparator)) ||
			strings.HasPrefix(filepath.Base(path), "_") {
			return nil
		}
		card, ok := c.readDigestCard(path, since)
		if ok {
			cards = append(cards, card)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].Score != cards[j].Score {
			return cards[i].Score > cards[j].Score
		}
		return cards[i].Date.After(cards[j].Date)
	})
	if len(cards) > maxCards {
		cards = cards[:maxCards]
	}
	return cards, nil
}

func (c *Curator) readDigestCard(path string, since time.Time) (digestCard, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return digestCard{}, false
	}
	content := string(data)
	fm := parseFrontmatterMap(content)
	score := parseDigestScore(firstNonEmpty(fm["pkb_score"], fm["score"]))
	if score < 7.0 {
		return digestCard{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return digestCard{}, false
	}
	cardDate := info.ModTime()
	if rawDate := firstNonEmpty(fm["ingest_date"], fm["pkb_scored_at"]); rawDate != "" {
		if parsed, ok := parseFlexibleDate(rawDate); ok {
			cardDate = parsed
		}
	}
	if cardDate.Before(since) {
		return digestCard{}, false
	}
	rel, err := filepath.Rel(c.basePath, path)
	if err != nil {
		rel = path
	}
	title := firstNonEmpty(fm["title"], strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	return digestCard{
		Title:    strings.Trim(title, `"'`),
		RelPath:  rel,
		Score:    score,
		Tags:     fm["tags"],
		Date:     cardDate,
		Excerpt:  digestExcerpt(stripFrontmatter(content), 360),
		FilePath: path,
	}, true
}

func (c *Curator) writeDigest(domain Domain, period string, cards []digestCard) error {
	if max := c.domains.Defaults.Budget.MaxDigestCallsPerRun; max > 0 && c.digestCalls >= max {
		return fmt.Errorf("digest budget exhausted: %d/%d", c.digestCalls, max)
	}
	c.digestCalls++
	now := time.Now()
	prompt := c.digestPrompt
	prompt = strings.ReplaceAll(prompt, "{{domain_display}}", domain.Display)
	prompt = strings.ReplaceAll(prompt, "{{domain_name}}", domain.Name)
	prompt = strings.ReplaceAll(prompt, "{{period}}", period)
	prompt = strings.ReplaceAll(prompt, "{{generated_at}}", now.Format(time.RFC3339))
	prompt = strings.ReplaceAll(prompt, "{{card_count}}", strconv.Itoa(len(cards)))
	prompt = strings.ReplaceAll(prompt, "{{cards}}", renderDigestCards(cards))

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.DigestModel, "", prompt, c.domains.Defaults.DigestTemperature, "long_context")
	if err != nil {
		return fmt.Errorf("digest llm: %w", err)
	}
	card := stripCardFence(out)
	card = pruneWikilinks(card, digestTitles(cards))
	if err := validateDigest(card); err != nil {
		return err
	}
	dir := filepath.Join(c.basePath, domain.VaultSubpath, "digest")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir digest dir: %w", err)
	}
	dst := filepath.Join(dir, digestFilename(period, domain.Display, now))
	if err := os.WriteFile(dst, []byte(card+"\n"), 0644); err != nil {
		return fmt.Errorf("write digest: %w", err)
	}
	fmt.Printf("[pkb-digest] → %s\n", dst)
	return nil
}

func renderDigestCards(cards []digestCard) string {
	var b strings.Builder
	for i, card := range cards {
		b.WriteString(fmt.Sprintf("%d. 标题：%s\n", i+1, card.Title))
		b.WriteString(fmt.Sprintf("   路径：%s\n", card.RelPath))
		b.WriteString(fmt.Sprintf("   分数：%.1f\n", card.Score))
		if card.Tags != "" {
			b.WriteString(fmt.Sprintf("   标签：%s\n", card.Tags))
		}
		b.WriteString(fmt.Sprintf("   摘要：%s\n\n", card.Excerpt))
	}
	return strings.TrimSpace(b.String())
}

func digestFilename(period, domainDisplay string, t time.Time) string {
	switch period {
	case "monthly":
		return fmt.Sprintf("%s_%s月综述.md", t.Format("2006-01"), sanitizeFilename(domainDisplay))
	default:
		year, week := t.ISOWeek()
		return fmt.Sprintf("%04d-W%02d_%s周综述.md", year, week, sanitizeFilename(domainDisplay))
	}
}

func digestTitles(cards []digestCard) []string {
	titles := make([]string, 0, len(cards))
	for _, card := range cards {
		if card.Title != "" {
			titles = append(titles, card.Title)
		}
	}
	return titles
}

func validateDigest(card string) error {
	trimmed := strings.TrimSpace(card)
	if !strings.HasPrefix(trimmed, "---\n") {
		return fmt.Errorf("generated digest missing YAML frontmatter")
	}
	requiredKeys := []string{"title:", "type:", "domain:", "period:", "generated_at:", "source_cards:"}
	for _, key := range requiredKeys {
		if !strings.Contains(trimmed, "\n"+key) {
			return fmt.Errorf("generated digest missing frontmatter key %s", strings.TrimSuffix(key, ":"))
		}
	}
	requiredSections := []string{
		"## 本期核心变化",
		"## 主题簇",
		"## 值得沉淀的知识",
		"## 缺口与后续问题",
		"## 关联卡片",
	}
	for _, section := range requiredSections {
		if !strings.Contains(trimmed, section) {
			return fmt.Errorf("generated digest missing section %s", section)
		}
	}
	return nil
}

func parseFrontmatterMap(content string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out
}

func stripFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}

var digestFinalScoreRe = regexp.MustCompile(`final=([0-9]+(?:\.[0-9]+)?)`)

func parseDigestScore(raw string) float64 {
	raw = strings.Trim(strings.TrimSpace(raw), `"'`)
	if raw == "" {
		return 0
	}
	if m := digestFinalScoreRe.FindStringSubmatch(raw); len(m) == 2 {
		raw = m[1]
	}
	score, _ := strconv.ParseFloat(raw, 64)
	return score
}

func parseFlexibleDate(raw string) (time.Time, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), `"'`)
	layouts := []string{time.RFC3339, "2006-01-02", "20060102"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func digestExcerpt(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, n)
}
