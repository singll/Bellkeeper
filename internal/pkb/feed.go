package pkb

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// FeedOptions 控制一次资讯库生成（资讯综述 + 落盘 + 晋升闸，ADR-0005 §5）。
type FeedOptions struct {
	Date        string // YYYY-MM-DD，空=今天
	DryRun      bool
	SkipPromote bool // 本轮只生成资讯库存档，跳过晋升闸（耐久知识点→知识库卡）
	// SkipDailyWrite 跳过每日资讯 md（writeFeedDaily）。资讯早报（GenerateBrief）已接管
	// vault/资讯/<date>.md 的唯一写入，feed 退化为「只做晋升闸」；置 true 避免覆盖早报落地。
	SkipDailyWrite bool
}

// feedItem 当日一条时效资讯（从 raw/archive 文件 frontmatter 采集，不存独立原子卡）。
type feedItem struct {
	domain  string // pkb_domain（领域 name）
	title   string
	excerpt string
	url     string
}

// RunFeed 生成资讯库当日存档（ADR-0005 §5.1）：遍历 raw+archive 取当日资讯类文章
// （pkb_type∈feed_content_types），按 pkb_domain 分组，每领域调 promote_model 综述「今天发生了什么」，
// 落 <资讯库根>/<领域 display>/<date>.md（一天一文件）。资讯不进知识骨架、不存独立原子卡（控 vault 膨胀）。
// 晋升闸（耐久知识点→知识库卡）在本流程之上由 Phase H-3 接入。
func (c *Curator) RunFeed(opts FeedOptions) error {
	date := strings.TrimSpace(opts.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("date 须为 YYYY-MM-DD: %w", err)
	}

	feedRoot, ok := c.feedArchiveRoot()
	if !ok {
		return fmt.Errorf("domains.yaml 未配置资讯库容器领域（需有一个领域设 feed: true，如 news）")
	}

	items := c.collectFeedItems(date)
	byDomain := groupFeedItems(items)

	fmt.Printf("[pkb-feed] 模式=%s 日期=%s 资讯条目=%d 领域=%d promote_model=%s prompt=%s 资讯库根=%s\n",
		digestMode(opts.DryRun), date, len(items), len(byDomain),
		c.domains.Defaults.PromoteModel, c.feedPromptName, feedRoot)

	if len(items) == 0 {
		fmt.Printf("[pkb-feed] 当日无资讯类文章（pkb_type∈%v；需主流程 pkb-curate 已打分），无需生成\n",
			c.domains.Defaults.FeedContentTypes)
		return nil
	}

	// 稳定输出顺序（领域名排序），便于 dry-run 阅读与测试断言
	domainNames := make([]string, 0, len(byDomain))
	for d := range byDomain {
		domainNames = append(domainNames, d)
	}
	sort.Strings(domainNames)

	// 资讯改「一天一文件」：各领域各自综述（保质量），按小节合并进 vault/资讯/<date>.md。
	// 领域→小节：FeedSection 优先，兜底域(misc)/资讯容器域(news) 归「其他」，消除「最新资讯」子目录与领域重复。
	sectionByName := map[string]*feedSection{}
	var sectionOrder []string
	var promotedTotal int
	for _, dn := range domainNames {
		group := byDomain[dn]
		domain, ok := c.domains.FindDomain(dn)
		if !ok {
			domain = c.domains.DefaultDomain()
		}
		secName := feedSectionName(domain)
		fmt.Printf("\n[pkb-feed] 领域 %s(%s)→小节「%s」资讯条目=%d\n", domain.Display, dn, secName, len(group))
		if opts.DryRun {
			for _, it := range group {
				fmt.Printf("  - %s（%s）\n", it.title, it.url)
			}
			continue
		}

		// 每日综述 md（资讯库存档）：早报已接管为唯一写入者时跳过（SkipDailyWrite），避免覆盖早报落地。
		// 综述失败只跳过该领域「小节」，不再连带跳过下方晋升（两者独立）。
		if !opts.SkipDailyWrite {
			summary, err := c.summarizeFeed(domain, date, group)
			if err != nil {
				fmt.Printf("[pkb-feed] ⚠ %s 综述失败（跳过该领域小节，不影响晋升/整批）: %v\n", domain.Display, err)
			} else if s, exists := sectionByName[secName]; exists {
				s.summary += "\n\n" + strings.TrimSpace(summary) // 同小节合并（misc+news→其他）
				s.count += len(group)
			} else {
				sectionByName[secName] = &feedSection{name: secName, summary: strings.TrimSpace(summary), count: len(group)}
				sectionOrder = append(sectionOrder, secName)
			}
		}

		// 晋升闸（ADR-0005 §5.2）：从当日资讯识别耐久知识点，走缺口填充同一 V2 路径晋升为知识库卡。
		// 仅对知识领域晋升（feed 容器领域的资讯多为事件性、无对应知识骨架，不晋升）。独立于每日 md。
		if !domain.Feed && c.domains.Defaults.GetPromoteEnabled() && !opts.SkipPromote {
			promotedTotal += c.promoteFromFeed(domain, date, group)
		}
	}

	if opts.DryRun {
		fmt.Printf("\n[pkb-feed] DRY-RUN：仅列出将综述/晋升的资讯条目，不调用 LLM/不抓取/不写盘\n")
		return nil
	}

	written := 0
	switch {
	case opts.SkipDailyWrite:
		fmt.Printf("\n[pkb-feed] ⏭️ 跳过每日资讯 md（早报已接管 vault/资讯/<date>.md 写入；本轮 feed 只做晋升）\n")
	case len(sectionOrder) > 0:
		dst, err := c.writeFeedDaily(feedRoot, date, orderedSections(sectionByName, sectionOrder), len(items))
		if err != nil {
			fmt.Printf("[pkb-feed] ⚠ 当日资讯落盘失败: %v\n", err)
		} else {
			fmt.Printf("    → %s\n", dst)
			written = 1
		}
	}

	if promotedTotal > 0 {
		fmt.Printf("\n[pkb-feed] 晋升 %d 个耐久知识点入知识库，触发 rebuild 对齐索引...\n", promotedTotal)
		if err := c.client.Rebuild(); err != nil {
			fmt.Printf("[pkb-feed] ⚠ rebuild 失败（卡已落盘，可稍后手动 rebuild）: %v\n", err)
		}
	}
	fmt.Printf("\n[pkb-feed] 完成：生成 %d 份当日资讯（%d 小节），晋升 %d 个知识点\n", written, len(sectionOrder), promotedTotal)
	return nil
}

// feedSection 当日资讯的一个领域小节（合并写入 vault/资讯/<date>.md）。
type feedSection struct {
	name    string
	summary string
	count   int
}

// feedSectionName 领域→资讯小节名：FeedSection 优先；兜底域(misc)/资讯容器域(news) 归「其他」；
// 其余用 Display。消除旧「最新资讯」子目录与领域内容重复。
func feedSectionName(domain Domain) string {
	if domain.FeedSection != "" {
		return domain.FeedSection
	}
	if domain.IsDefault || domain.Feed {
		return "其他"
	}
	return domain.Display
}

// orderedSections 按出现顺序组装小节，「其他」始终垫底。
func orderedSections(byName map[string]*feedSection, order []string) []feedSection {
	out := make([]feedSection, 0, len(order))
	var other *feedSection
	for _, name := range order {
		if name == "其他" {
			other = byName[name]
			continue
		}
		out = append(out, *byName[name])
	}
	if other != nil {
		out = append(out, *other)
	}
	return out
}

// feedArchiveRoot 返回资讯库容器领域（feed:true）的 vault 子路径，作为资讯存档根（如 vault/资讯）。
func (c *Curator) feedArchiveRoot() (string, bool) {
	for _, d := range c.domains.Domains {
		if d.Feed {
			return d.VaultSubpath, true
		}
	}
	return "", false
}

// collectFeedItems 遍历 raw + archive 层，按 frontmatter 取当日(ingested_at/pkb_scored_at)资讯类(pkb_type)文章。
// 依赖主流程 pkb-curate 已给文章打分（frontmatter 才有 pkb_type/pkb_domain）；🔶未打分的当天文章本轮不入综述（下轮补）。
func (c *Curator) collectFeedItems(date string) []feedItem {
	var items []feedItem
	feedTypes := feedTypeSet(c.domains.Defaults.FeedContentTypes)
	for _, layer := range []string{"raw", "archive"} {
		root := filepath.Join(c.basePath, layer)
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(d.Name()) != ".md" {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			content := string(data)
			fm := parseFrontmatterMap(content)
			pkbType := strings.ToLower(strings.Trim(fm["pkb_type"], `"'`))
			if !feedTypes[pkbType] {
				return nil
			}
			if feedItemDate(fm) != date {
				return nil
			}
			items = append(items, feedItem{
				domain:  strings.Trim(firstNonEmpty(fm["pkb_domain"], "misc"), `"'`),
				title:   strings.Trim(firstNonEmpty(fm["title"], strings.TrimSuffix(d.Name(), ".md")), `"'`),
				excerpt: digestExcerpt(stripFrontmatter(content), 200),
				url:     strings.Trim(firstNonEmpty(fm["url"], fm["source"]), `"'`),
			})
			return nil
		})
	}
	return items
}

// feedItemDate 取「当日」判定日期：优先入库时间 ingested_at，回退 pkb_scored_at（RFC3339），截到 YYYY-MM-DD。
func feedItemDate(fm map[string]string) string {
	for _, key := range []string{"ingested_at", "pkb_scored_at"} {
		v := strings.Trim(fm[key], `"'`)
		if v == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t.Format("2006-01-02")
		}
		if len(v) >= 10 {
			return v[:10] // 已是 YYYY-MM-DD 或带日期前缀
		}
	}
	return ""
}

func feedTypeSet(types []string) map[string]bool {
	m := make(map[string]bool, len(types))
	for _, t := range types {
		m[strings.ToLower(strings.TrimSpace(t))] = true
	}
	return m
}

// groupFeedItems 按领域(pkb_domain)分组。
func groupFeedItems(items []feedItem) map[string][]feedItem {
	by := make(map[string][]feedItem)
	for _, it := range items {
		by[it.domain] = append(by[it.domain], it)
	}
	return by
}

// summarizeFeed 调 promote_model 把某领域当日资讯条目综述成「今天发生了什么」。
func (c *Curator) summarizeFeed(domain Domain, date string, items []feedItem) (string, error) {
	prompt := c.feedPrompt
	prompt = strings.ReplaceAll(prompt, "{{domain_display}}", domain.Display)
	prompt = strings.ReplaceAll(prompt, "{{date}}", date)
	prompt = strings.ReplaceAll(prompt, "{{items}}", renderFeedItems(items))

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.PromoteModel, "", prompt,
		c.domains.Defaults.DigestTemperature, "summary")
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(textutil.StripFence(out))
	if summary == "" {
		return "", fmt.Errorf("综述返回空内容")
	}
	return summary, nil
}

// renderFeedItems 渲染资讯条目列表喂综述提示词（标题 + 来源 + 摘要）。
func renderFeedItems(items []feedItem) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString("- ")
		b.WriteString(it.title)
		if it.url != "" {
			b.WriteString(fmt.Sprintf("（%s）", it.url))
		}
		if it.excerpt != "" {
			b.WriteString("：")
			b.WriteString(it.excerpt)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// writeFeedDaily 落当日资讯为「一天一文件」vault/资讯/<date>.md，正文按领域分 ## 小节（原子写）。
// frontmatter type: pkb_feed 标记非知识卡；跨天只新增文件不删历史，单天重跑覆盖（当天全量综述，幂等）。
func (c *Curator) writeFeedDaily(feedRoot, date string, sections []feedSection, totalItems int) (string, error) {
	dir := filepath.Join(c.basePath, feedRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir feed dir: %w", err)
	}
	dst := filepath.Join(dir, date+".md")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: 资讯 %s\n", date))
	b.WriteString("type: pkb_feed\n")
	b.WriteString(fmt.Sprintf("date: %s\n", date))
	b.WriteString(fmt.Sprintf("generated_at: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("item_count: %d\n", totalItems))
	b.WriteString("tags: [pkb-feed]\n")
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("# %s 资讯\n\n", date))
	for _, s := range sections {
		b.WriteString(fmt.Sprintf("## %s\n\n", s.name))
		b.WriteString(strings.TrimSpace(s.summary))
		b.WriteString("\n\n")
	}
	content := strings.TrimRight(b.String(), "\n") + "\n"

	tmp := dst + ".tmp.md"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename: %w", err)
	}
	return dst, nil
}

type promoteCandidate struct {
	concept    string
	durability int
	novelty    int
	event      bool
}

// promoteFromFeed 晋升闸（ADR-0005 §5.2）：promote_model 从当日资讯识别耐久知识点候选，过闸
// （非事件 && durability≥阈值）后，每个知识点复用缺口填充 fillOneGap 走同一 V2 路径（起草→真抓取
// 核实→落卡，禁止旁路核实）晋升为知识库正经原子卡并归位到骨架。返回晋升成功数。
func (c *Curator) promoteFromFeed(domain Domain, date string, items []feedItem) int {
	candidates, err := c.identifyDurableKnowledge(domain, date, items)
	if err != nil {
		fmt.Printf("[pkb-feed] ⚠ %s 晋升判定失败（跳过晋升，资讯存档不受影响）: %v\n", domain.Display, err)
		return 0
	}
	if len(candidates) == 0 {
		return 0
	}
	dir := filepath.Join(c.basePath, domain.VaultSubpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("[pkb-feed] ⚠ %s 晋升 mkdir 失败（跳过）: %v\n", domain.Display, err)
		return 0
	}
	durMin := c.domains.Defaults.PromoteDurabilityMin
	var promoted int
	for _, cand := range candidates {
		if !shouldPromote(cand, durMin) {
			fmt.Printf("    ⏭️ 不晋升「%s」（event=%v durability=%d，阈值%.0f；事件性/价值不足，留资讯库自然过期）\n",
				cand.concept, cand.event, cand.durability, durMin)
			continue
		}
		outcome, err := c.fillOneGap(domain, gapNode{Concept: cand.concept}, dir)
		if err != nil {
			fmt.Printf("    ✗ 晋升「%s」失败（跳过，不中断）: %v\n", cand.concept, err)
			continue
		}
		if outcome.skipped != "" {
			fmt.Printf("    ⏭️ 晋升「%s」跳过：%s\n", cand.concept, outcome.skipped)
			continue
		}
		if outcome.written {
			markFeedPromoted(filepath.Join(dir, sanitizeFilename(cand.concept)+".md"), date)
			promoted++
			fmt.Printf("    ⭐ 晋升「%s」入知识库 verification=%s confidence=%s\n",
				cand.concept, outcome.verification, outcome.confidence)
		}
	}
	if promoted > 0 {
		// 复用 Phase F 归位把新晋升卡挂回骨架（缺口→已填，或进待归位待后续 propose 回流）。
		if err := c.placeCardsOntoSkeleton(domain, false, true); err != nil {
			fmt.Printf("[pkb-feed] ⚠ %s 晋升后归位失败（卡已落盘，下轮 digest/match 重试）: %v\n", domain.Display, err)
		}
	}
	return promoted
}

// identifyDurableKnowledge 调 promote_model 从当日资讯条目识别耐久知识点候选（事件性资讯标 event=yes 会被过滤）。
func (c *Curator) identifyDurableKnowledge(domain Domain, date string, items []feedItem) ([]promoteCandidate, error) {
	prompt := c.promotePrompt
	prompt = strings.ReplaceAll(prompt, "{{domain_display}}", domain.Display)
	prompt = strings.ReplaceAll(prompt, "{{date}}", date)
	prompt = strings.ReplaceAll(prompt, "{{items}}", renderFeedItems(items))
	out, err := c.chatCompletionWithRetry(c.domains.Defaults.PromoteModel, "", prompt,
		c.domains.Defaults.ScoreTemperature, "summary")
	if err != nil {
		return nil, err
	}
	return parsePromoteCandidates(out), nil
}

// parsePromoteCandidates 解析 promote_model 输出：每行
// `CONCEPT: x | DURABILITY: n | NOVELTY: n | EVENT: yes|no | REASON: ...`；NONE / 无 CONCEPT 的行忽略。
func parsePromoteCandidates(out string) []promoteCandidate {
	var cands []promoteCandidate
	for _, line := range strings.Split(textutil.StripFence(out), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.EqualFold(t, "NONE") {
			continue
		}
		if !strings.Contains(strings.ToUpper(t), "CONCEPT:") {
			continue
		}
		fields := map[string]string{}
		for _, part := range strings.Split(t, "|") {
			kv := strings.SplitN(part, ":", 2)
			if len(kv) != 2 {
				continue
			}
			fields[strings.ToUpper(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
		}
		concept := strings.Trim(fields["CONCEPT"], `"'`)
		if concept == "" {
			continue
		}
		cands = append(cands, promoteCandidate{
			concept:    concept,
			durability: atoiSafe(fields["DURABILITY"]),
			novelty:    atoiSafe(fields["NOVELTY"]),
			event:      strings.HasPrefix(strings.ToLower(fields["EVENT"]), "y"),
		})
	}
	return cands
}

// shouldPromote 晋升把闸（ADR-0005 §5.2）：非事件性且 durability 达阈值的耐久知识点才晋升；
// 事件性资讯（event=true）一律不晋升，留资讯库自然过期。
func shouldPromote(cand promoteCandidate, durabilityMin float64) bool {
	return !cand.event && float64(cand.durability) >= durabilityMin
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// markFeedPromoted 给晋升落盘的知识卡补晋升来源标记（区别于普通缺口填充/重构卡）。
// 卡路径不可预测时（如核实降级走 F2 多卡改名）静默跳过，不影响已落卡。
func markFeedPromoted(path, date string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	updated := upsertFrontmatter(string(data), map[string]string{
		"pkb_promoted":  "true",
		"promoted_from": "feed",
		"promoted_date": date,
	})
	_ = os.WriteFile(path, []byte(updated), 0644)
}
