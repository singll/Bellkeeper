package pkb

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// GapFillOptions 控制一次缺口填充（针对单个领域）。
type GapFillOptions struct {
	Domain string
	DryRun bool
	PerRun int // 0 = 用 domains.yaml defaults.gap_fill_per_run
}

// gapNode 骨架知识树里一个标 [缺口] 的待填节点。
type gapNode struct {
	Concept string
	Depth   int // 缩进深度（自顶向下广度优先排序用：浅在前）
}

// gapDraft gapfill_model 起草输出（元信息头 + 卡草稿）。
type gapDraft struct {
	card       string
	volatility string   // stable | volatile（前沿/易变缺口走 F2 定向爬，见 Phase G-3）
	confidence string   // 模型自评（最终置信度以 V2 核实结果为准）
	sources    []string // 提议的权威源 URL
}

// gapFillOutcome 单个缺口填充结果（落卡的核实状态，或跳过原因）。
type gapFillOutcome struct {
	concept      string
	verification string // verified | unverified | llm-only
	confidence   string // high | medium | low
	source       string // 核实/提议的权威源 URL，或 "gap-fill"（无源）
	written      bool
	skipped      string // 非空=本缺口被跳过的原因
}

// RunGapFill 自顶向下把骨架 [缺口] 节点补成真原子卡（ADR-0004 Phase G / 计划 §4）：
// gapfill_model 起草草稿+提议权威源 → G3 冷却让路 → Extract 真抓 → verify_model 判该页是否支撑
// → 落卡（带 source/verification/confidence）→ 复用 Phase F 归位把卡挂回该缺口节点（缺口 → 已填）。
// 严格 V2 核实（Q7，不可旁路）：禁止「自报即 verified」，抓不到/不支撑一律降级。
func (c *Curator) RunGapFill(opts GapFillOptions) error {
	domain, ok := c.domains.FindDomain(opts.Domain)
	if !ok {
		return fmt.Errorf("unknown domain: %s", opts.Domain)
	}
	if !c.domains.Defaults.GapFillEnabledFor(domain.Name) {
		return fmt.Errorf("领域 %s 未开启缺口填充；在 config/pkb/domains.yaml 设 gap_fill_enabled.%s: true（或 gap_fill_enabled_all: true）后再跑",
			domain.Name, domain.Name)
	}

	indexPath := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("领域 %s 尚无骨架 _index.md（先跑 `pkb-curate skeleton %s`）: %w", domain.Name, domain.Name, err)
	}

	gaps := parseGapNodes(string(data))
	if c.domains.Defaults.GapFillOrder == "breadth" {
		sortGapsBreadthFirst(gaps)
	}
	perRun := opts.PerRun
	if perRun <= 0 {
		perRun = c.domains.Defaults.GapFillPerRun
	}
	dropped := 0
	if len(gaps) > perRun {
		dropped = len(gaps) - perRun
		gaps = gaps[:perRun]
	}

	fmt.Printf("[pkb-fill] 模式=%s 领域=%s(%s) 本轮缺口=%d(上限%d) order=%s gapfill_model=%s verify_model=%s prompt=%s/%s\n",
		digestMode(opts.DryRun), domain.Display, domain.Name, len(gaps), perRun,
		c.domains.Defaults.GapFillOrder, c.domains.Defaults.GapfillModel, c.domains.Defaults.VerifyModel,
		c.gapfillPromptName, c.verifyPromptName)
	if dropped > 0 {
		fmt.Printf("[pkb-fill] 本轮按上限截取，剩余 %d 个缺口留待下轮\n", dropped)
	}
	if len(gaps) == 0 {
		fmt.Printf("[pkb-fill] 无 [缺口] 节点，无需填充\n")
		return nil
	}

	if opts.DryRun {
		for _, g := range gaps {
			fmt.Printf("  [缺口] %s (深度%d)\n", g.Concept, g.Depth)
		}
		fmt.Printf("[pkb-fill] DRY-RUN：仅列出本轮将填充的缺口，不调用 LLM/不抓取/不写盘\n")
		return nil
	}

	dir := filepath.Join(c.basePath, domain.VaultSubpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir vault dir: %w", err)
	}

	var filled, lowConf int
	for i, g := range gaps {
		fmt.Printf("\n[%d/%d] 缺口：%s\n", i+1, len(gaps), g.Concept)
		outcome, err := c.fillOneGap(domain, g, dir)
		if err != nil {
			fmt.Printf("    ✗ 填充失败（跳过，不中断整批）: %v\n", err)
			continue
		}
		if outcome.skipped != "" {
			fmt.Printf("    ⏭️ 跳过：%s\n", outcome.skipped)
			continue
		}
		if outcome.written {
			filled++
			if isLowConfidence(outcome.verification) {
				lowConf++
			}
			fmt.Printf("    ✓ 已落卡 verification=%s confidence=%s source=%s\n",
				outcome.verification, outcome.confidence, outcome.source)
		}
	}

	fmt.Printf("\n[pkb-fill] 本轮填充 %d/%d 个缺口（低置信 %d）\n", filled, len(gaps), lowConf)

	if filled > 0 {
		// 复用 Phase F 归位：新卡 atomic_concept = 缺口节点名 → 必中该节点（缺口 → 已填）。
		fmt.Printf("[pkb-fill] 归位新卡到骨架...\n")
		if err := c.placeCardsOntoSkeleton(domain, false, true); err != nil {
			fmt.Printf("[pkb-fill] ⚠ 归位失败（卡已落盘，下轮 digest/match 会重试）: %v\n", err)
		}
		if err := c.client.Rebuild(); err != nil {
			fmt.Printf("[pkb-fill] ⚠ rebuild 失败（卡已落盘，可稍后手动 rebuild）: %v\n", err)
		}
	}
	return nil
}

// fillOneGap 执行单个缺口的填充循环（F1）：起草 → V2 核实 → 落卡。归位由调用方批量做。
func (c *Curator) fillOneGap(domain Domain, g gapNode, dir string) (gapFillOutcome, error) {
	out := gapFillOutcome{concept: g.Concept}

	slug := sanitizeFilename(g.Concept)
	dst := filepath.Join(dir, slug+".md")
	if _, statErr := os.Stat(dst); statErr == nil {
		out.skipped = "已存在同名卡（疑似已填，避免覆盖）"
		return out, nil
	}

	// 1. 起草（gapfill_model 顶级推理档）：自评易变性 + 写草稿 + 提议权威源
	draft, err := c.draftGapCard(domain, g.Concept)
	if err != nil {
		return out, fmt.Errorf("draft: %w", err)
	}

	// F2：易变/前沿缺口跳过草稿卡，改定向爬权威源走 reconstruct 原子化（§4.2，步骤 b 后合流）。
	// 易变知识模型记忆不可靠，以真实抓取内容为准；无提议源则回退 F1 草稿（下方）。
	if draft.volatility == "volatile" && len(draft.sources) > 0 {
		return c.fillGapByCrawl(domain, g, draft.sources, dir)
	}

	// F1：稳定缺口用草稿卡 + V2 核实
	if strings.TrimSpace(draft.card) == "" {
		return out, fmt.Errorf("draft 返回空卡")
	}

	// 2. V2 核实（Q7，不可旁路）：冷却让路(G3) → Extract 真抓 → verify_model 判支撑
	out.verification, out.confidence, out.source = c.verifyGapCard(g.Concept, draft.card, draft.sources)

	// 3. 落卡：强制 atomic_concept=缺口名（归位必中）+ 写 source/verification/confidence；清死链
	card := finalizeGapCard(draft.card, g.Concept, domain, out.verification, out.confidence, out.source)
	if err := validateCard(card); err != nil {
		return out, fmt.Errorf("草稿卡不合法（缺 frontmatter/章节）: %w", err)
	}
	card = pruneWikilinks(card, nil) // 缺口卡关联由后续归位/digest 重建，先清所有 wikilink 死链

	tmpPath := dst + ".tmp.md"
	if err := os.WriteFile(tmpPath, []byte(card), 0644); err != nil {
		return out, fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return out, fmt.Errorf("rename: %w", err)
	}
	fmt.Printf("    → %s\n", dst)
	out.written = true
	return out, nil
}

// draftGapCard 调 gapfill_model 起草缺口卡草稿（自评易变性/置信度 + 提议权威源）。
func (c *Curator) draftGapCard(domain Domain, concept string) (gapDraft, error) {
	prompt := c.gapfillPrompt
	prompt = strings.ReplaceAll(prompt, "{{domain_display}}", domain.Display)
	prompt = strings.ReplaceAll(prompt, "{{domain_name}}", domain.Name)
	prompt = strings.ReplaceAll(prompt, "{{scope}}", strings.TrimSpace(domain.Scope))
	prompt = strings.ReplaceAll(prompt, "{{concept}}", concept)
	prompt = strings.ReplaceAll(prompt, "{{date}}", time.Now().Format("20060102"))

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.GapfillModel, "", prompt,
		c.domains.Defaults.DigestTemperature, "long_context")
	if err != nil {
		return gapDraft{}, err
	}
	return parseGapDraft(out), nil
}

// verifyGapCard 对草稿卡执行 V2 核实，返回 (verification, confidence, source)。
// 无引用→llm-only/low；有引用但冷却/抓不到/不支撑→unverified/medium；抓到且支撑→verified/high。
func (c *Curator) verifyGapCard(concept, card string, sources []string) (string, string, string) {
	if len(sources) == 0 {
		return "llm-only", "low", "gap-fill" // 无引用：不抓取，最低置信
	}
	primary := sources[0]
	host := hostOf(primary)

	// G3：抓取前查冷却表 next_allowed_at；冷却中不抓（不排队、不写 crawl_failures），只降级卡。
	if c.domainRepo != nil && host != "" {
		cooling, err := c.domainRepo.IsCooling(host)
		if err != nil {
			fmt.Printf("    ⚠ 查询域名 %s 冷却状态失败（按未冷却处理）: %v\n", host, err)
		} else if cooling {
			fmt.Printf("    🔶 源域名 %s 冷却中，跳过抓取核实，降级为 unverified\n", host)
			return "unverified", "medium", primary
		}
	}

	res, err := c.client.Extract(primary)
	if err != nil || res == nil || !res.Success || strings.TrimSpace(res.Content) == "" {
		fmt.Printf("    🔶 抓取核实源失败/为空，降级为 unverified：%s\n", errOrEmpty(err))
		return "unverified", "medium", primary
	}

	supported, err := c.verifySupports(concept, card, res.Content)
	if err != nil {
		fmt.Printf("    🔶 核实判定失败，降级为 unverified: %v\n", err)
		return "unverified", "medium", primary
	}
	if supported {
		return "verified", "high", primary
	}
	fmt.Printf("    🔶 页面不支撑卡片，降级为 unverified\n")
	return "unverified", "medium", primary
}

// verifySupports 调 verify_model 判抓取到的页面是否实质支撑卡片。
func (c *Curator) verifySupports(concept, card, pageContent string) (bool, error) {
	prompt := c.verifyPrompt
	prompt = strings.ReplaceAll(prompt, "{{concept}}", concept)
	prompt = strings.ReplaceAll(prompt, "{{card_summary}}", gapCardSummary(card))
	prompt = strings.ReplaceAll(prompt, "{{page_content}}", truncateRunes(pageContent, c.domains.Defaults.ContentTruncate))

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.VerifyModel, "", prompt,
		c.domains.Defaults.ScoreTemperature, "summary")
	if err != nil {
		return false, err
	}
	return parseSupported(out), nil
}

// gapCardSummary 取卡的「定义与本质 + 关键细节」作核实摘要（喂 verify_model，控上下文）。
func gapCardSummary(card string) string {
	body := stripFrontmatter(card)
	s := strings.TrimSpace(extractSection(body, "## 定义与本质") + "\n" + extractSection(body, "## 关键细节"))
	return truncateRunes(s, 1200)
}

// finalizeGapCard 覆盖草稿卡 frontmatter 的核实结果与稳定字段（atomic_concept 强制=缺口名，确保归位必中）。
func finalizeGapCard(card, concept string, domain Domain, verification, confidence, source string) string {
	fields := map[string]string{
		"atomic_concept": concept,
		"type":           "pkb_card",
		"domains":        domain.Name,
		"source":         source,
		"verification":   verification,
		"confidence":     confidence,
		"ingest_date":    time.Now().Format("20060102"),
		"pkb_gap_fill":   "true",
	}
	if frontmatterValue(card, "score") == "" {
		fields["score"] = "7.0"
	}
	return upsertFrontmatter(card, fields)
}

// parseGapDraft 解析 gapfill_model 输出：元信息头（VOLATILITY/CONFIDENCE/SOURCES）+ ---CARD--- 后的卡体。
func parseGapDraft(out string) gapDraft {
	raw := textutil.StripFence(out)
	d := gapDraft{}
	head := raw
	if idx := strings.Index(raw, "---CARD---"); idx >= 0 {
		head = raw[:idx]
		d.card = strings.TrimSpace(raw[idx+len("---CARD---"):])
	} else {
		d.card = strings.TrimSpace(raw)
	}
	for _, line := range strings.Split(head, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "VOLATILITY:"):
			d.volatility = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(t, "VOLATILITY:")))
		case strings.HasPrefix(t, "CONFIDENCE:"):
			d.confidence = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(t, "CONFIDENCE:")))
		case strings.HasPrefix(t, "SOURCES:"):
			d.sources = parseSourceURLs(strings.TrimPrefix(t, "SOURCES:"))
		}
	}
	return d
}

// parseSourceURLs 解析逗号分隔的来源 URL 列表；NONE/空/非 http(s) 一律忽略。
func parseSourceURLs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "NONE") {
		return nil
	}
	var urls []string
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
			urls = append(urls, p)
		}
	}
	return urls
}

// parseSupported 解析 verify_model 输出首个 `SUPPORTED: yes/no` 行；解析不出则保守判不支撑。
func parseSupported(out string) bool {
	for _, line := range strings.Split(textutil.StripFence(out), "\n") {
		t := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(t, "supported:") {
			return strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(t, "supported:")), "yes")
		}
	}
	return false
}

// parseGapNodes 从 _index.md 的「## 知识树」提取标 [缺口] 的节点（concept + 缩进深度，去重保序）。
func parseGapNodes(content string) []gapNode {
	body := extractSection(content, "## 知识树")
	var gaps []gapNode
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		m := skeletonNodeRe.FindStringSubmatch(line)
		if m == nil || strings.TrimSpace(m[3]) != "[缺口]" {
			continue // 非节点行，或已挂卡（[[..]]）的非缺口节点
		}
		concept := strings.TrimSpace(m[2])
		if concept == "" || seen[concept] {
			continue
		}
		seen[concept] = true
		gaps = append(gaps, gapNode{Concept: concept, Depth: indentDepth(m[1])})
	}
	return gaps
}

// sortGapsBreadthFirst 按缩进深度升序稳定排序（自顶向下：先填根/主题层缺口）。
func sortGapsBreadthFirst(gaps []gapNode) {
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].Depth < gaps[j].Depth })
}

// indentDepth 折算缩进深度（空格记 1，tab 记 2），用于广度优先排序。
func indentDepth(indent string) int {
	n := 0
	for _, r := range indent {
		switch r {
		case '\t':
			n += 2
		case ' ':
			n++
		}
	}
	return n
}

// isLowConfidence 低置信卡 = 未核实或纯 LLM 编写（计划 §6 audit 指标口径）。
func isLowConfidence(verification string) bool {
	return verification == "unverified" || verification == "llm-only"
}

// hostOf 取 URL 的主机名（与爬虫 source_domain 口径一致：url.Hostname()），用于查域名冷却。
func hostOf(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func errOrEmpty(err error) string {
	if err != nil {
		return err.Error()
	}
	return "内容为空或抓取未成功"
}

// fillGapByCrawl 是易变缺口的 F2 路径（计划 §4.2）：定向爬权威源 → reconstruct 原子化。
// G3 冷却让路同 F1；抓到的内容经现有 reconstructCard 生成卡（基于真实页面，标 verified），
// 与 F1 在「落卡 + 归位」处合流。抓取失败/冷却中只跳过本缺口（不写 crawl_failures）。
func (c *Curator) fillGapByCrawl(domain Domain, g gapNode, sources []string, dir string) (gapFillOutcome, error) {
	out := gapFillOutcome{concept: g.Concept}
	primary := sources[0]
	host := hostOf(primary)
	fmt.Printf("    ↻ 易变缺口走定向爬（F2）：%s\n", primary)

	if c.domainRepo != nil && host != "" {
		cooling, err := c.domainRepo.IsCooling(host)
		if err != nil {
			fmt.Printf("    ⚠ 查询域名 %s 冷却状态失败（按未冷却处理）: %v\n", host, err)
		} else if cooling {
			out.skipped = fmt.Sprintf("源域名 %s 冷却中（易变缺口定向爬，留待下轮）", host)
			return out, nil
		}
	}

	res, err := c.client.Extract(primary)
	if err != nil || res == nil || !res.Success || strings.TrimSpace(res.Content) == "" {
		out.skipped = fmt.Sprintf("易变缺口定向爬抓取失败/为空：%s", errOrEmpty(err))
		return out, nil
	}

	cards, err := c.reconstructGapCards(domain, g.Concept, res, primary)
	if err != nil {
		return out, fmt.Errorf("reconstruct: %w", err)
	}
	if written := c.writeGapReconstructedCards(domain, g.Concept, cards, primary, dir); written > 0 {
		out.written = true
		out.verification, out.confidence, out.source = "verified", "high", primary
		fmt.Printf("    ✓ 定向爬原子化落 %d 张卡（verified，基于真实抓取）\n", written)
	} else {
		out.skipped = "reconstruct 未产出可落卡"
	}
	return out, nil
}

// reconstructGapCards 用现有 reconstructCard 把抓取到的权威源正文原子化成卡。
// 缺口为定向补全目标，合成一个高相关高价值打分（ContentType=knowledge）喂重构提示词。
func (c *Curator) reconstructGapCards(domain Domain, concept string, res *ExtractResult, sourceURL string) ([]string, error) {
	art := ArticleMeta{Title: firstNonEmpty(res.Title, concept), URL: sourceURL}
	score := &ScoreResult{
		Relevance:      9,
		Depth:          8,
		Actionability:  7,
		Durability:     7,
		Novelty:        6,
		ContentType:    "knowledge",
		MatchedDomains: []string{domain.Name},
	}
	cards, _, err := c.reconstructCard(art, res.Content, score, domain, nil, nil, time.Now().Format("20060102"))
	return cards, err
}

// writeGapReconstructedCards 落 F2 原子化产出的多张卡：第一张锚回缺口名（确保缺口→已填），
// 其余卡保留各自 concept（作补充卡，由后续归位/digest 入网）。全部标 verified（基于真实抓取）。
func (c *Curator) writeGapReconstructedCards(domain Domain, concept string, cards []string, sourceURL, dir string) int {
	written := 0
	for i, card := range cards {
		anchor := ""
		if i == 0 {
			anchor = concept
		}
		card = finalizeCrawledGapCard(card, anchor, domain, sourceURL)
		if err := validateCard(card); err != nil {
			fmt.Printf("    ⚠ 第 %d 张 reconstruct 卡校验失败（跳过）: %v\n", i+1, err)
			continue
		}
		card = pruneWikilinks(card, nil)

		cardConcept := firstNonEmpty(frontmatterValue(card, "atomic_concept"), concept)
		slug := sanitizeFilename(cardConcept)
		dst := filepath.Join(dir, slug+".md")
		if _, statErr := os.Stat(dst); statErr == nil {
			placed := false
			for n := 2; n <= 100; n++ {
				alt := filepath.Join(dir, fmt.Sprintf("%s-%d.md", slug, n))
				if _, e := os.Stat(alt); e != nil {
					dst = alt
					card = appendAlias(card, slug)
					placed = true
					break
				}
			}
			if !placed {
				fmt.Printf("    ⚠ %s 落盘位已满（跳过）\n", slug)
				continue
			}
		}
		tmp := dst + ".tmp.md"
		if err := os.WriteFile(tmp, []byte(card), 0644); err != nil {
			fmt.Printf("    ⚠ 写 %s 失败（跳过）: %v\n", dst, err)
			continue
		}
		if err := os.Rename(tmp, dst); err != nil {
			_ = os.Remove(tmp)
			fmt.Printf("    ⚠ rename %s 失败（跳过）: %v\n", dst, err)
			continue
		}
		fmt.Printf("    → %s\n", dst)
		written++
	}
	return written
}

// finalizeCrawledGapCard 为 F2 原子化卡盖上缺口填充标记：标 verified/high（基于真实抓取），
// 写回核实源 URL；anchorConcept 非空时强制 atomic_concept=缺口名（第一张卡，归位必中）。
func finalizeCrawledGapCard(card, anchorConcept string, domain Domain, sourceURL string) string {
	fields := map[string]string{
		"type":         "pkb_card",
		"domains":      domain.Name,
		"source":       sourceURL,
		"verification": "verified",
		"confidence":   "high",
		"ingest_date":  time.Now().Format("20060102"),
		"pkb_gap_fill": "true",
	}
	if anchorConcept != "" {
		fields["atomic_concept"] = anchorConcept
	}
	if frontmatterValue(card, "score") == "" {
		fields["score"] = "7.0"
	}
	return upsertFrontmatter(card, fields)
}
