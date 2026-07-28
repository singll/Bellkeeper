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

	"github.com/singll/bellkeeper/internal/pkg/textutil"
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
	Title         string
	AtomicConcept string
	CardType      string
	RelPath       string
	Score         float64
	Tags          string
	Date          time.Time
	Excerpt       string
	Relations     string
	FilePath      string
}

// digestWriteMode 控制写入模式
type digestWriteMode string

const (
	digestModeRoot  digestWriteMode = "root"
	digestModeTopic digestWriteMode = "topic"
)

// RunDigest builds a domain-level knowledge map from existing high-value vault cards.
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

	fmt.Printf("[pkb-digest] 模式=%s period=%s since=%s max_cards=%d model=%s prompt=%s topic_moc=%v\n",
		digestMode(opts.DryRun), period, since.Format("2006-01-02"), maxCards,
		c.domains.Defaults.DigestModel, c.digestPromptName, c.domains.Defaults.GetTopicMocEnabled())

	var generated int
	for _, domain := range domains {
		if domain.Feed {
			// 资讯库容器（ADR-0005）：vault 子目录是分领域分日资讯存档，不产知识原子卡——
			// 跳过 digest（资讯不进知识骨架/综述，亦不被 collectDigestCards 遍历）。
			continue
		}
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
				fmt.Printf("  - %.1f %s (%s)\n", card.Score, card.AtomicConcept, card.RelPath)
			}
			continue
		}

		existingMap := c.loadExistingIndex(domain)

		if c.domains.Defaults.GetMapSnapshotOnRefresh() && existingMap != "" {
			if err := c.snapshotIndex(domain); err != nil {
				fmt.Printf("[pkb-digest] ⚠ 快照旧版失败（继续）: %v\n", err)
			}
		}

		if err := c.writeDigestRoot(domain, period, cards, existingMap); err != nil {
			if c.domains.Defaults.Retry.GetStopRunOnRateLimit() && isRetryableLLMError(err) {
				fmt.Printf("[pkb-digest] ↷ LLM 免费池/上游仍在限流，本轮停止；剩余领域下轮继续: %v\n", err)
				break
			}
			if isBudgetExhausted(err) {
				fmt.Printf("[pkb-digest] ↷ 本轮 digest 预算已用尽；剩余领域下轮继续: %v\n", err)
				break
			}
			fmt.Printf("[pkb-digest] ⚠ %s 生成根索引失败: %v\n", domain.Name, err)
			continue
		}
		generated++

		if c.domains.Defaults.GetTopicMocEnabled() {
			topics := c.extractRootTopics(domain)
			for _, topic := range topics {
				topicCards := c.collectTopicCards(domain, topic, cards)
				minCards := c.domains.Defaults.TopicMinCards
				if minCards <= 0 {
					minCards = 5
				}
				if len(topicCards) < minCards {
					continue
				}
				if err := c.writeDigestTopic(domain, topic, topicCards); err != nil {
					fmt.Printf("[pkb-digest] ⚠ 主题 MOC %q 生成失败: %v\n", topic, err)
					continue
				}
				generated++
			}
		}

		// ADR-0004：digest 写完根索引后，用最新骨架对全部原子卡做确定性归位——
		// 把 LLM 生成的知识树覆盖为「骨架渲染树」（结构以骨架为准，不再凭卡片猜树），
		// 并产出/更新待归位区。snapshot=false（本轮开头已快照 pre-digest 索引），
		// rebuild 由本轮末尾统一触发。无骨架的领域内部 no-op，digest 行为不变。
		if err := c.placeCardsOntoSkeleton(domain, false, false); err != nil {
			fmt.Printf("[pkb-digest] ⚠ %s 归位失败（digest 根索引已写盘）: %v\n", domain.Name, err)
		}
	}

	if !opts.DryRun && generated > 0 {
		fmt.Printf("[pkb-digest] 触发 rebuild 与文件系统对齐...\n")
		if err := c.client.Rebuild(); err != nil {
			fmt.Printf("[pkb-digest] ⚠ rebuild 失败（digest 已写盘，可稍后手动 rebuild）: %v\n", err)
		}
	}
	fmt.Printf("[pkb-digest] 完成：生成 %d 篇\n", generated)
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
		card, ok := c.readDigestCard(path, since, domain)
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

func (c *Curator) readDigestCard(path string, since time.Time, domain Domain) (digestCard, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return digestCard{}, false
	}
	content := string(data)
	fm := parseFrontmatterMap(content)
	score := parseDigestScore(firstNonEmpty(fm["pkb_score"], fm["score"]))
	if score < domain.VaultThresholdOr(c.domains.Defaults) {
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
	atomicConcept := fm["atomic_concept"]
	cardType := firstNonEmpty(fm["card_type"], fm["type"])
	body := stripFrontmatter(content)
	relations := extractRelationsSection(body)
	return digestCard{
		Title:         strings.Trim(title, `"'`),
		AtomicConcept: strings.Trim(atomicConcept, `"'`),
		CardType:      strings.Trim(cardType, `"'`),
		RelPath:       rel,
		Score:         score,
		Tags:          fm["tags"],
		Date:          cardDate,
		Excerpt:       digestExcerpt(body, 360),
		Relations:     relations,
		FilePath:      path,
	}, true
}

func (c *Curator) writeDigestRoot(domain Domain, period string, cards []digestCard, existingMap string) error {
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
	prompt = strings.ReplaceAll(prompt, "{{existing_map}}", existingMap)
	if existingMap == "" {
		prompt = removeEmptySection(prompt, "## 已有体系结构")
	}

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.DigestModel, "", prompt, c.domains.Defaults.DigestTemperature, "long_context")
	if err != nil {
		return fmt.Errorf("digest llm: %w", err)
	}
	card := textutil.StripFence(out)
	card = pruneWikilinks(card, digestTitles(cards))
	card = normalizeMapFrontmatter(card, domain, now) // 不信 LLM 复述：强制 generated_at/domain、补闭合、剥离误抄的「## 元信息」
	if err := validateDigestWithMode(card, digestModeRoot); err != nil {
		return err
	}

	dir := filepath.Join(c.basePath, domain.VaultSubpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir vault dir: %w", err)
	}

	nextPath := filepath.Join(dir, "_index.next.md")
	if err := os.WriteFile(nextPath, []byte(card+"\n"), 0644); err != nil {
		return fmt.Errorf("write _index.next.md: %w", err)
	}

	indexPath := filepath.Join(dir, "_index.md")
	if err := os.Rename(nextPath, indexPath); err != nil {
		_ = os.Remove(nextPath)
		return fmt.Errorf("rename _index.next.md→_index.md: %w", err)
	}
	fmt.Printf("[pkb-digest] → %s\n", indexPath)
	return nil
}

func (c *Curator) writeDigestTopic(domain Domain, topic string, cards []digestCard) error {
	if max := c.domains.Defaults.Budget.MaxDigestCallsPerRun; max > 0 && c.digestCalls >= max {
		return fmt.Errorf("digest budget exhausted: %d/%d", c.digestCalls, max)
	}
	c.digestCalls++
	now := time.Now()
	prompt := c.digestTopicPrompt
	prompt = strings.ReplaceAll(prompt, "{{domain_display}}", domain.Display)
	prompt = strings.ReplaceAll(prompt, "{{domain_name}}", domain.Name)
	prompt = strings.ReplaceAll(prompt, "{{period}}", "")
	prompt = strings.ReplaceAll(prompt, "{{generated_at}}", now.Format(time.RFC3339))
	prompt = strings.ReplaceAll(prompt, "{{card_count}}", strconv.Itoa(len(cards)))
	prompt = strings.ReplaceAll(prompt, "{{cards}}", renderDigestCards(cards))
	prompt = strings.ReplaceAll(prompt, "{{existing_map}}", "")

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.DigestModel, "", prompt, c.domains.Defaults.DigestTemperature, "long_context")
	if err != nil {
		return fmt.Errorf("topic moc llm: %w", err)
	}
	card := textutil.StripFence(out)
	card = pruneWikilinks(card, digestTitles(cards))
	card = normalizeMapFrontmatter(card, domain, now) // 同根索引：规整元信息、补闭合、剥离误抄段
	if err := validateDigestWithMode(card, digestModeTopic); err != nil {
		return err
	}

	topicsDir := filepath.Join(c.basePath, domain.VaultSubpath, "topics")
	if err := os.MkdirAll(topicsDir, 0755); err != nil {
		return fmt.Errorf("mkdir topics dir: %w", err)
	}
	dst := filepath.Join(topicsDir, sanitizeFilename(topic)+".md")
	if err := os.WriteFile(dst, []byte(card+"\n"), 0644); err != nil {
		return fmt.Errorf("write topic moc: %w", err)
	}
	fmt.Printf("[pkb-digest] → %s\n", dst)
	return nil
}

// loadExistingIndex 读取已有的 _index.md 作为增量更新上下文。
func (c *Curator) loadExistingIndex(domain Domain) string {
	path := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// snapshotIndex 将当前 _index.md 快照到 digest/ 子目录。
func (c *Curator) snapshotIndex(domain Domain) error {
	src := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read index for snapshot: %w", err)
	}
	digestDir := filepath.Join(c.basePath, domain.VaultSubpath, "digest")
	if err := os.MkdirAll(digestDir, 0755); err != nil {
		return fmt.Errorf("mkdir digest for snapshot: %w", err)
	}
	snapName := fmt.Sprintf("%s_快照.md", time.Now().Format("20060102_1504"))
	dst := filepath.Join(digestDir, snapName)
	return os.WriteFile(dst, data, 0644)
}

// extractRootTopics 从已生成的 _index.md 的 frontmatter root_concepts 提取顶层主题列表。
func (c *Curator) extractRootTopics(domain Domain) []string {
	path := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fm := parseFrontmatterMap(string(data))
	raw := fm["root_concepts"]
	if raw == "" {
		return nil
	}
	raw = strings.Trim(raw, "[]")
	parts := strings.Split(raw, ",")
	var topics []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		if p != "" {
			topics = append(topics, p)
		}
	}
	return topics
}

// collectTopicCards 从全部卡片中筛选属于指定主题的卡片（按 atomic_concept 前缀/包含匹配）。
// 匹配 0 张时返回空（调用方应跳过该主题，不生成伪主题 MOC）。
func (c *Curator) collectTopicCards(domain Domain, topic string, allCards []digestCard) []digestCard {
	var matched []digestCard
	for _, card := range allCards {
		concept := card.AtomicConcept
		if concept == "" {
			concept = card.Title
		}
		if strings.Contains(concept, topic) || strings.Contains(card.Tags, topic) {
			matched = append(matched, card)
		}
	}
	return matched
}

// normalizeMapFrontmatter 规整 digest/骨架 _index 的 LLM 输出，不信任 LLM 复述的元信息：
// ① 剥离正文里误抄的「## 元信息」段（提示词末尾的上下文块常被照抄进输出）；
// ② 补 frontmatter 闭合 ---（LLM 偶尔漏写第二个 ---，Obsidian 会解析不出属性）；
// ③ 强制 generated_at=now、domain=真实领域（修 generated_at 幻觉未来时间 bug）。
// 顺序：先剥离误抄段 → 补闭合（否则 upsert 无法定位 frontmatter 边界）→ 覆盖字段。
func normalizeMapFrontmatter(card string, domain Domain, now time.Time) string {
	card = stripMetaSection(card)
	card = ensureFrontmatterClosed(card)
	card = upsertFrontmatter(card, map[string]string{
		"generated_at": now.Format(time.RFC3339),
		"domain":       domain.Name,
	})
	return card
}

// stripMetaSection 剥离正文里误抄的「## 元信息」章节（到下一个 ## 标题或文末止）。
func stripMetaSection(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			skip = t == "## 元信息"
			if skip {
				continue
			}
		}
		if skip {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

// ensureFrontmatterClosed 若首行是 --- 但缺闭合 ---，在 frontmatter 结束处（第一个空行或 # 标题前）补一行 ---。
// frontmatter 内规范无空行、root_concepts 用行内数组，故「首个空行/标题」即边界，安全。
func ensureFrontmatterClosed(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content // 无 frontmatter，不处理
	}
	if hasClosedFrontmatter(content) {
		return content
	}
	end := len(lines)
	for i := 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "#") {
			end = i
			break
		}
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:end]...)
	out = append(out, "---")
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// hasClosedFrontmatter 首行 --- 且后续存在闭合 --- 时返回 true。
func hasClosedFrontmatter(content string) bool {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return true
		}
	}
	return false
}

// validateDigestWithMode 按 mode 校验 digest 输出。
func validateDigestWithMode(card string, mode digestWriteMode) error {
	trimmed := strings.TrimSpace(card)
	if !strings.HasPrefix(trimmed, "---\n") {
		return fmt.Errorf("generated digest missing YAML frontmatter")
	}
	// frontmatter 必须闭合（第二个 ---）：规整后应已闭合，此为双保险，堵「缺闭合 --- 致 Obsidian 解析不出属性」。
	if !hasClosedFrontmatter(trimmed) {
		return fmt.Errorf("generated digest frontmatter not closed (missing second ---)")
	}
	requiredKeys := []string{"title:", "type:", "domain:", "generated_at:"}
	for _, key := range requiredKeys {
		if !strings.Contains(trimmed, "\n"+key) {
			return fmt.Errorf("generated digest missing frontmatter key %s", strings.TrimSuffix(key, ":"))
		}
	}
	switch mode {
	case digestModeRoot:
		v2Sections := []string{
			"## 体系概览",
			"## 知识树",
			"## 核心脉络",
			"## 新增与变化",
			"## 缺口与探索方向",
		}
		for _, section := range v2Sections {
			if !strings.Contains(trimmed, section) {
				return fmt.Errorf("generated root index missing section %s", section)
			}
		}
	case digestModeTopic:
		topicRequiredKeys := []string{"type: pkb_topic", "parent:", "member_concepts:"}
		for _, key := range topicRequiredKeys {
			if !strings.Contains(trimmed, "\n"+key) && !strings.HasPrefix(trimmed, key) {
				return fmt.Errorf("generated topic moc missing or incorrect: %s", strings.TrimSuffix(key, ":"))
			}
		}
		requiredSections := []string{
			"## 知识树",
			"## 核心脉络",
		}
		for _, section := range requiredSections {
			if !strings.Contains(trimmed, section) {
				return fmt.Errorf("generated topic moc missing section %s", section)
			}
		}
	default:
		v1Sections := []string{
			"## 本期核心变化",
			"## 主题簇",
			"## 值得沉淀的知识",
			"## 缺口与后续问题",
			"## 关联卡片",
		}
		v2Sections := []string{
			"## 体系概览",
			"## 知识树",
			"## 核心脉络",
			"## 新增与变化",
			"## 缺口与探索方向",
		}
		v1Ok := true
		for _, section := range v1Sections {
			if !strings.Contains(trimmed, section) {
				v1Ok = false
				break
			}
		}
		v2Ok := true
		for _, section := range v2Sections {
			if !strings.Contains(trimmed, section) {
				v2Ok = false
				break
			}
		}
		if !v1Ok && !v2Ok {
			return fmt.Errorf("generated digest missing required sections (neither v1 nor v2 format)")
		}
	}
	return nil
}

func renderDigestCards(cards []digestCard) string {
	var b strings.Builder
	for i, card := range cards {
		concept := card.AtomicConcept
		if concept == "" {
			concept = card.Title
		}
		b.WriteString(fmt.Sprintf("%d. atomic_concept: %s\n", i+1, concept))
		b.WriteString(fmt.Sprintf("   title: %s\n", card.Title))
		if card.CardType != "" {
			b.WriteString(fmt.Sprintf("   card_type: %s\n", card.CardType))
		} else {
			b.WriteString("   card_type: pkb_card\n")
		}
		b.WriteString(fmt.Sprintf("   分数：%.1f\n", card.Score))
		if card.Tags != "" {
			b.WriteString(fmt.Sprintf("   标签：%s\n", card.Tags))
		}
		b.WriteString(fmt.Sprintf("   摘要：%s\n", card.Excerpt))
		if card.Relations != "" {
			b.WriteString(fmt.Sprintf("   关系：%s\n", card.Relations))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func digestTitles(cards []digestCard) []string {
	titles := make([]string, 0, len(cards)*2)
	for _, card := range cards {
		if card.AtomicConcept != "" {
			titles = append(titles, card.AtomicConcept)
		}
		if card.Title != "" {
			titles = append(titles, card.Title)
		}
	}
	return titles
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

var relationsHeaderRe = regexp.MustCompile(`(?i)^## (?:与其他知识的关系|关联)\s*$`)

// extractRelationsSection 从卡片正文中提取"与其他知识的关系"章节内容。
func extractRelationsSection(body string) string {
	lines := strings.Split(body, "\n")
	inSection := false
	var sectionLines []string
	for _, line := range lines {
		if relationsHeaderRe.MatchString(strings.TrimSpace(line)) {
			inSection = true
			continue
		}
		if inSection {
			if strings.HasPrefix(strings.TrimSpace(line), "## ") {
				break
			}
			sectionLines = append(sectionLines, line)
		}
	}
	result := strings.TrimSpace(strings.Join(sectionLines, "\n"))
	if result == "" || result == "（暂无关联）" {
		return ""
	}
	return result
}

// removeEmptySection 移除提示词中标题后紧跟空内容的章节（标题行 + 后续空白行直到下一个 ## 或文末）。
func removeEmptySection(prompt, sectionTitle string) string {
	lines := strings.Split(prompt, "\n")
	var out []string
	skip := false
	for _, line := range lines {
		if !skip && strings.HasPrefix(strings.TrimSpace(line), sectionTitle) {
			skip = true
			continue
		}
		if skip {
			if strings.HasPrefix(strings.TrimSpace(line), "## ") {
				skip = false
				out = append(out, line)
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			skip = false
			out = append(out, line)
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
