package pkb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// SkeletonOptions 控制一次知识骨架生成（针对单个领域）。
type SkeletonOptions struct {
	Domain string // 领域 name 或 display（必填，须在 domains.yaml 配有 scope）
	TOC    string // 可选：权威源参考目录文本，喂入提示词校正层级与覆盖
	DryRun bool
}

// RunSkeleton 按领域的 scope（大方向）生成初次知识骨架——一棵多层目标概念树，
// 每个节点初始为 `[缺口]`（ADR-0004：骨架是结构唯一真相源，digest 后续把卡片渲染挂上去）。
//
// 写盘复用原子计划护栏 6：覆盖前先快照旧 _index.md → 写 _index.next.md → 原子 rename。
// 存量卡链接由后续「归位」(match) 步骤重新挂载；主题 MOC(topics/<主题>.md) 随 digest 在
// 卡片累积后生成，因此本步只写根 _index.md。
func (c *Curator) RunSkeleton(opts SkeletonOptions) error {
	domain, ok := c.domains.FindDomain(opts.Domain)
	if !ok {
		return fmt.Errorf("unknown domain: %s", opts.Domain)
	}
	scope := strings.TrimSpace(domain.Scope)
	if scope == "" {
		return fmt.Errorf("领域 %s 未配置 scope（大方向）；先在 config/pkb/domains.yaml 给它加一行 `scope:` 再生成骨架", domain.Name)
	}

	fmt.Printf("[pkb-skeleton] 模式=%s 领域=%s(%s) model=%s prompt=%s\n",
		digestMode(opts.DryRun), domain.Display, domain.Name,
		c.domains.Defaults.SkeletonModel, c.skeletonPromptName)
	fmt.Printf("[pkb-skeleton] scope：%s\n", scope)

	indexPath := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	if opts.DryRun {
		fmt.Printf("[pkb-skeleton] DRY-RUN：仅打印 scope 与目标路径，不调用 LLM、不写盘\n")
		fmt.Printf("[pkb-skeleton] 目标：%s\n", indexPath)
		return nil
	}

	now := time.Now()
	prompt := c.skeletonPrompt
	prompt = strings.ReplaceAll(prompt, "{{domain_display}}", domain.Display)
	prompt = strings.ReplaceAll(prompt, "{{domain_name}}", domain.Name)
	prompt = strings.ReplaceAll(prompt, "{{scope}}", scope)
	prompt = strings.ReplaceAll(prompt, "{{generated_at}}", now.Format(time.RFC3339))
	toc := strings.TrimSpace(opts.TOC)
	prompt = strings.ReplaceAll(prompt, "{{toc}}", toc)
	if toc == "" {
		prompt = removeEmptySection(prompt, "## 权威源参考目录")
	}

	out, err := c.chatCompletionWithRetry(c.domains.Defaults.SkeletonModel, "", prompt, c.domains.Defaults.DigestTemperature, "long_context")
	if err != nil {
		return fmt.Errorf("skeleton llm: %w", err)
	}
	card := textutil.StripFence(out)
	if err := validateDigestWithMode(card, digestModeRoot); err != nil {
		return fmt.Errorf("骨架输出不合法（缺 frontmatter 或必需章节）: %w", err)
	}

	dir := filepath.Join(c.basePath, domain.VaultSubpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir vault dir: %w", err)
	}

	// 护栏 6：覆盖前快照旧 _index.md（存量卡链接将由后续「归位」重新挂载，快照可回滚）
	if c.loadExistingIndex(domain) != "" {
		if err := c.snapshotIndex(domain); err != nil {
			fmt.Printf("[pkb-skeleton] ⚠ 快照旧 _index.md 失败（继续）: %v\n", err)
		} else {
			fmt.Printf("[pkb-skeleton] 已快照旧 _index.md → %s/digest/（存量卡链接将由后续「归位」重新挂载）\n", domain.VaultSubpath)
		}
	}

	if err := writeFileAtomic(indexPath, card); err != nil {
		return err
	}
	fmt.Printf("[pkb-skeleton] → %s\n", indexPath)

	fmt.Printf("[pkb-skeleton] 触发 rebuild 与文件系统对齐...\n")
	if err := c.client.Rebuild(); err != nil {
		fmt.Printf("[pkb-skeleton] ⚠ rebuild 失败（骨架已写盘，可稍后手动 rebuild）: %v\n", err)
	}

	gaps := strings.Count(card, "[缺口]")
	fmt.Printf("[pkb-skeleton] 完成：根骨架已落盘，%d 个缺口待填充/归位；主题 MOC(topics/) 将随 digest 在卡片累积后生成\n", gaps)
	return nil
}

// MatchOptions 控制一次归位（针对单个领域）。
type MatchOptions struct {
	Domain string // 领域 name 或 display（必填，须已有骨架 _index.md）
	DryRun bool
}

// matchCard 归位输入的一张原子卡（轻量，只取归位判定所需字段）。
type matchCard struct {
	Concept string
	Title   string
	Excerpt string
	RelPath string
}

// RunMatch 把领域内所有原子卡归位到知识骨架节点上（ADR-0004 的「渲染骨架」）：
// 用 match_model（快强档）判定每张卡属于哪个骨架节点，挂上去（节点 [缺口] → [[卡]]）；
// 判不上的进待归位区 _待归位.md。骨架结构原样保留——只重算各节点的挂载标记，不重写树、
// 不动 frontmatter 与散文章节。写盘复用护栏 6（快照旧 _index.md → 写 .next → 原子 rename）。
func (c *Curator) RunMatch(opts MatchOptions) error {
	domain, ok := c.domains.FindDomain(opts.Domain)
	if !ok {
		return fmt.Errorf("unknown domain: %s", opts.Domain)
	}
	indexPath := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("领域 %s 尚无骨架 _index.md（先跑 `pkb-curate skeleton %s` 生成）: %w", domain.Name, domain.Name, err)
	}

	if err := c.placeCardsOntoSkeleton(domain, opts.DryRun, true); err != nil {
		return err
	}
	if opts.DryRun {
		return nil
	}

	fmt.Printf("[pkb-match] 触发 rebuild 与文件系统对齐...\n")
	if err := c.client.Rebuild(); err != nil {
		fmt.Printf("[pkb-match] ⚠ rebuild 失败（归位已写盘，可稍后手动 rebuild）: %v\n", err)
	}
	fmt.Printf("[pkb-match] 完成\n")
	return nil
}

// placeCardsOntoSkeleton 执行一次确定性归位（无 rebuild，供 RunMatch 与 RunDigest 复用）：
// 读 _index.md → 解析骨架节点 → match_model 判定 → 渲染树（缺口→[[卡]]）→ 写 _index.md + _待归位.md。
// 领域无骨架（无 _index.md 或无节点）则记录并 no-op 返回 nil，使 digest 在旧领域上行为不变。
// snapshot=true 时覆盖前快照旧 _index.md（digest 流程已自行快照，传 false 避免重复）。
func (c *Curator) placeCardsOntoSkeleton(domain Domain, dryRun, snapshot bool) error {
	indexPath := filepath.Join(c.basePath, domain.VaultSubpath, "_index.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("[pkb-match] %s 无骨架 _index.md，跳过归位\n", domain.Name)
			return nil
		}
		return fmt.Errorf("read _index.md: %w", err)
	}
	indexContent := string(data)

	nodes := parseSkeletonNodes(indexContent)
	if len(nodes) == 0 {
		fmt.Printf("[pkb-match] %s 的 _index.md 无骨架节点，跳过归位\n", domain.Name)
		return nil
	}

	cards, err := c.collectAllCards(domain)
	if err != nil {
		return fmt.Errorf("collect cards: %w", err)
	}

	fmt.Printf("[pkb-match] 模式=%s 领域=%s(%s) 骨架节点=%d 原子卡=%d model=%s prompt=%s\n",
		digestMode(dryRun), domain.Display, domain.Name, len(nodes), len(cards),
		c.domains.Defaults.MatchModel, c.matchPromptName)

	if len(cards) == 0 {
		fmt.Printf("[pkb-match] 领域无原子卡，跳过归位\n")
		return nil
	}

	assignments, err := c.matchCardsToNodes(nodes, cards)
	if err != nil {
		return fmt.Errorf("match llm: %w", err)
	}

	nodeSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n] = true
	}
	placed := map[string][]string{}
	var unplaced []matchCard
	for _, card := range cards {
		node, ok := assignments[card.Concept]
		if !ok || node == "待归位" || !nodeSet[node] {
			unplaced = append(unplaced, card)
			continue
		}
		placed[node] = append(placed[node], card.Concept)
	}
	fmt.Printf("[pkb-match] 归位=%d 待归位=%d\n", len(cards)-len(unplaced), len(unplaced))

	if dryRun {
		for _, n := range nodes {
			if cs := placed[n]; len(cs) > 0 {
				fmt.Printf("  [节点] %s ← %s\n", n, strings.Join(cs, ", "))
			}
		}
		for _, u := range unplaced {
			fmt.Printf("  [待归位] %s (%s)\n", u.Concept, u.RelPath)
		}
		return nil
	}

	if snapshot && c.domains.Defaults.GetMapSnapshotOnRefresh() {
		if err := c.snapshotIndex(domain); err != nil {
			fmt.Printf("[pkb-match] ⚠ 快照旧 _index.md 失败（继续）: %v\n", err)
		}
	}

	rendered := renderSkeletonWithCards(indexContent, placed)
	if err := writeFileAtomic(indexPath, rendered); err != nil {
		return fmt.Errorf("write _index.md: %w", err)
	}
	fmt.Printf("[pkb-match] → %s（%d 节点已挂卡）\n", indexPath, len(placed))

	waitlistPath := filepath.Join(c.basePath, domain.VaultSubpath, "_待归位.md")
	if len(unplaced) > 0 {
		if err := writeFileAtomic(waitlistPath, renderWaitlist(domain, unplaced)); err != nil {
			return fmt.Errorf("write _待归位.md: %w", err)
		}
		fmt.Printf("[pkb-match] → %s（%d 张待归位）\n", waitlistPath, len(unplaced))
	} else if _, statErr := os.Stat(waitlistPath); statErr == nil {
		// 本轮无待归位卡：清掉旧待归位文件，避免陈旧残留
		if err := os.Remove(waitlistPath); err != nil {
			fmt.Printf("[pkb-match] ⚠ 移除空待归位文件失败: %v\n", err)
		}
	}
	return nil
}

// skeletonNodeRe 匹配知识树节点行：捕获缩进、概念名、挂载标记（[缺口] 或 [[..]]）。
var skeletonNodeRe = regexp.MustCompile(`^(\s*)-\s+(.+?)\s+(\[缺口\]|\[\[.*)$`)

// parseSkeletonNodes 从 _index.md 的「## 知识树」章节提取所有节点的概念名（去重保序）。
func parseSkeletonNodes(content string) []string {
	lines := strings.Split(content, "\n")
	inTree := false
	seen := map[string]bool{}
	var nodes []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			inTree = t == "## 知识树"
			continue
		}
		if !inTree {
			continue
		}
		m := skeletonNodeRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		concept := strings.TrimSpace(m[2])
		if concept != "" && !seen[concept] {
			seen[concept] = true
			nodes = append(nodes, concept)
		}
	}
	return nodes
}

// renderSkeletonWithCards 原样保留骨架结构，只重算「## 知识树」每个节点的挂载标记：
// 有归位卡的节点 → [[卡1]], [[卡2]]；无卡的节点 → [缺口]。frontmatter 与其余章节不动。
func renderSkeletonWithCards(content string, placed map[string][]string) string {
	lines := strings.Split(content, "\n")
	inTree := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			inTree = t == "## 知识树"
			continue
		}
		if !inTree {
			continue
		}
		m := skeletonNodeRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, concept := m[1], strings.TrimSpace(m[2])
		if cs := placed[concept]; len(cs) > 0 {
			links := make([]string, len(cs))
			for j, c := range cs {
				links[j] = "[[" + c + "]]"
			}
			lines[i] = fmt.Sprintf("%s- %s %s", indent, concept, strings.Join(links, ", "))
		} else {
			lines[i] = fmt.Sprintf("%s- %s [缺口]", indent, concept)
		}
	}
	return strings.Join(lines, "\n")
}

// renderWaitlist 生成待归位区 _待归位.md：未匹配到任何骨架节点的卡片清单。
// Slice 4 的涌现回流提议会读这里的同主题簇，据此为骨架提议加节点。
func renderWaitlist(domain Domain, cards []matchCard) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: 待归位 - %s\n", domain.Display))
	b.WriteString("type: pkb_waitlist\n")
	b.WriteString(fmt.Sprintf("domain: %s\n", domain.Name))
	b.WriteString(fmt.Sprintf("generated_at: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("count: %d\n", len(cards)))
	b.WriteString("---\n\n")
	b.WriteString("# 待归位卡片\n\n")
	b.WriteString("以下原子卡未匹配到当前知识骨架的任何节点。它们等待：\n")
	b.WriteString("- 据同主题簇为骨架提议加节点（涌现回流，走影响半径闸）；\n")
	b.WriteString("- 或人工调整领域 scope（大方向）后重新归位。\n\n")
	for _, c := range cards {
		concept := c.Concept
		line := fmt.Sprintf("- [[%s]]", concept)
		if c.Excerpt != "" {
			line += " — " + c.Excerpt
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// collectAllCards 走 vault 收集领域内全部原子卡（不设分数/日期门槛——归位面向所有已落卡）。
// 排除 _index/_待归位 等下划线文件与 digest/maps/topics 目录。
func (c *Curator) collectAllCards(domain Domain) ([]matchCard, error) {
	root := filepath.Join(c.basePath, domain.VaultSubpath)
	var cards []matchCard
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
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)
		fm := parseFrontmatterMap(content)
		concept := strings.Trim(firstNonEmpty(fm["atomic_concept"], fm["title"], strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))), `"'`)
		if concept == "" {
			return nil
		}
		rel, relErr := filepath.Rel(c.basePath, path)
		if relErr != nil {
			rel = path
		}
		cards = append(cards, matchCard{
			Concept: concept,
			Title:   strings.Trim(fm["title"], `"'`),
			Excerpt: digestExcerpt(stripFrontmatter(content), 200),
			RelPath: rel,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cards, nil
}

// matchCardsToNodes 用 match_model 判定每张卡归到哪个节点（或待归位），分批以控上下文。
// 返回 concept → 节点名（或「待归位」）；未在返回中的卡由调用方按待归位处理。
func (c *Curator) matchCardsToNodes(nodeConcepts []string, cards []matchCard) (map[string]string, error) {
	const batchSize = 40
	nodeList := renderNodeList(nodeConcepts)
	result := make(map[string]string, len(cards))
	for start := 0; start < len(cards); start += batchSize {
		end := start + batchSize
		if end > len(cards) {
			end = len(cards)
		}
		batch := cards[start:end]
		prompt := c.matchPrompt
		prompt = strings.ReplaceAll(prompt, "{{nodes}}", nodeList)
		prompt = strings.ReplaceAll(prompt, "{{cards}}", renderMatchCards(batch))

		out, err := c.chatCompletionWithRetry(c.domains.Defaults.MatchModel, "", prompt, c.domains.Defaults.ScoreTemperature, "summary")
		if err != nil {
			return nil, err
		}
		pairs, perr := parseMatchJSON(out)
		if perr != nil {
			fmt.Printf("[pkb-match] ⚠ 第 %d 批归位结果解析失败（这批按待归位处理）: %v\n", start/batchSize+1, perr)
			continue
		}
		for concept, node := range pairs {
			result[concept] = node
		}
	}
	return result, nil
}

func renderNodeList(concepts []string) string {
	var b strings.Builder
	for i, c := range concepts {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderMatchCards(cards []matchCard) string {
	var b strings.Builder
	for i, c := range cards {
		b.WriteString(fmt.Sprintf("%d. concept: %s\n", i+1, c.Concept))
		if c.Excerpt != "" {
			b.WriteString(fmt.Sprintf("   摘要：%s\n", c.Excerpt))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// parseMatchJSON 解析 match_model 输出的 [{"concept":..,"node":..}]，返回 concept→node。
func parseMatchJSON(out string) (map[string]string, error) {
	s := textutil.StripFence(strings.TrimSpace(out))
	start := strings.Index(s, "[")
	endRel := strings.LastIndex(s, "]")
	if start < 0 || endRel < 0 || endRel < start {
		return nil, fmt.Errorf("no JSON array in output")
	}
	var arr []struct {
		Concept string `json:"concept"`
		Node    string `json:"node"`
	}
	if err := json.Unmarshal([]byte(s[start:endRel+1]), &arr); err != nil {
		return nil, fmt.Errorf("unmarshal match json: %w", err)
	}
	m := make(map[string]string, len(arr))
	for _, p := range arr {
		concept := strings.TrimSpace(p.Concept)
		if concept != "" {
			m[concept] = strings.TrimSpace(p.Node)
		}
	}
	return m, nil
}

// writeFileAtomic 护栏 6 的原子写：先写 <path>.next → 原子 rename 覆盖目标。
func writeFileAtomic(path, content string) error {
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	nextPath := path + ".next"
	if err := os.WriteFile(nextPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(nextPath), err)
	}
	if err := os.Rename(nextPath, path); err != nil {
		_ = os.Remove(nextPath)
		return fmt.Errorf("rename %s→%s: %w", filepath.Base(nextPath), filepath.Base(path), err)
	}
	return nil
}
