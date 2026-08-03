package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// 资讯早报（news brief）—— 与运维日报同源但职责不同：
// 早报取「滚动时间窗内真实入库的资讯文章」，分领域列出并附 LLM 全局总结/看点，08:00 推送；
// 运维日报取系统统计+PKB，晚间推送，仅内嵌少量资讯链接指回早报（见 daily_report_render.go）。

// newsCategoryOrder 决定早报分组的展示顺序（技术在前、泛资讯在后）。
var newsCategoryOrder = []string{"编程", "人工智能", "网络安全", "资讯"}

// NewsItem 是一条资讯（去重后一篇文章一条）。
type NewsItem struct {
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Domain   string    `json:"domain,omitempty"`
	Category string    `json:"category"`
	At       time.Time `json:"at"`
	Score    float64   `json:"score,omitempty"` // LLM 重要性打分（0-10），未打分=0
}

// NewsGroup 是同一领域的资讯集合。
type NewsGroup struct {
	Category string     `json:"category"`
	Items    []NewsItem `json:"items"`
}

// NewsBriefData 是资讯早报的结构化数据。
type NewsBriefData struct {
	Date        string         `json:"date"`
	WindowStart string         `json:"window_start"`
	WindowEnd   string         `json:"window_end"`
	WindowHours int            `json:"window_hours"`
	Total       int            `json:"total"`
	Groups      []NewsGroup    `json:"groups"`
	AISummary   string         `json:"ai_summary,omitempty"`
	Errors      []CollectError `json:"errors,omitempty"`
}

// normalizeNewsCategory 将 RSS 源/标签上五花八门的分类名归一到早报四大领域。
func normalizeNewsCategory(c string) string {
	switch strings.TrimSpace(c) {
	case "编程", "人工智能", "网络安全", "资讯":
		return strings.TrimSpace(c)
	}
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "development", "dev", "programming", ".net", "backend", "frontend", "编程开发":
		return "编程"
	case "ai", "ml", "机器学习", "人工智能/机器学习", "llm":
		return "人工智能"
	case "security", "vulnerability", "vuln", "安全", "网络安全/漏洞":
		return "网络安全"
	case "news", "资讯", "科技", "泛科技", "":
		return "资讯"
	}
	return "资讯"
}

// buildDomainCategoryMap 用当前活跃 RSS 源建「文章来源域名 → 早报领域」映射。
// 直连源（http(s)）的 URL host 通常与入库文章的 source_domain 一致，命中率高；未命中的归「资讯」。
func (s *DailyReportService) buildDomainCategoryMap() map[string]string {
	m := make(map[string]string)
	if s.rssRepo == nil {
		return m
	}
	feeds, err := s.rssRepo.GetActiveIncludingPaused()
	if err != nil {
		log.Printf("[NewsBrief] build domain map: list feeds failed: %v", err)
		return m
	}
	for _, f := range feeds {
		cat := normalizeNewsCategory(f.Category)
		host := feedHost(f.URL)
		if host != "" {
			m[host] = cat
		}
	}
	return m
}

// feedHost 从 feed URL 提取归一化 host（去 www.）；RSSHub 相对路径无 host，返回空。
func feedHost(raw string) string {
	if raw == "" || strings.HasPrefix(raw, "/") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// techDomainCategory 已知技术站点 → 早报领域的静态映射，补足 RSS 源 host 匹配的盲区：
// 文章 source_domain 常是原始链接域名（如 github.com），与聚合源/feedburner/RSSHub 的 host
// 不一致，只靠 RSS 源匹配会让大量技术内容漏进「资讯」兜底。此表把明确的技术站点直接归位。
var techDomainCategory = map[string]string{
	// 编程 / 开发
	"github.com": "编程", "github.blog": "编程", "gitlab.com": "编程",
	"stackoverflow.com": "编程", "devblogs.microsoft.com": "编程", "dev.to": "编程",
	"infoq.com": "编程", "martinfowler.com": "编程", "go.dev": "编程",
	"blog.golang.org": "编程", "rust-lang.org": "编程", "lwn.net": "编程",
	"phoronix.com": "编程", "thenewstack.io": "编程", "hackaday.com": "编程",
	"johndcook.com": "编程",
	// 人工智能
	"arxiv.org": "人工智能", "huggingface.co": "人工智能", "paperswithcode.com": "人工智能",
	"openai.com": "人工智能", "anthropic.com": "人工智能", "deepmind.com": "人工智能",
	"ai.googleblog.com": "人工智能", "pytorch.org": "人工智能", "simonwillison.net": "人工智能",
	// 网络安全
	"thehackernews.com": "网络安全", "bleepingcomputer.com": "网络安全",
	"krebsonsecurity.com": "网络安全", "portswigger.net": "网络安全",
	"unit42.paloaltonetworks.com": "网络安全", "sans.org": "网络安全",
	"cisa.gov": "网络安全", "snyk.io": "网络安全", "darkreading.com": "网络安全",
	"securityweek.com": "网络安全", "schneier.com": "网络安全",
}

// noiseDomains 纯 UGC 社交/视频源：作为早报「资讯头条」质量偏低，采集时跳过（内容仍在知识库/资讯库）。
var noiseDomains = map[string]struct{}{
	"youtube.com": {}, "m.youtube.com": {}, "twitter.com": {}, "x.com": {},
	"facebook.com": {}, "instagram.com": {}, "tiktok.com": {}, "t.co": {},
}

// normHost 归一化域名：小写 + 去 www. 前缀，供域名字典/noise 集合命中。
func normHost(raw string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "www.")
}

// isNoiseDomain 判断域名是否为早报应跳过的纯 UGC 社交/视频源。
func isNoiseDomain(host string) bool {
	if host == "" {
		return false
	}
	_, ok := noiseDomains[host]
	return ok
}

// categoryForArticle 判定一篇入库文章归哪个早报领域：优先按来源域名匹配 RSS 源分类，
// 再查已知技术站点字典（把技术从「资讯」捞回），退化到关联标签名归一（仅当能归入技术领域时），
// 最终退化为「资讯」。
func categoryForArticle(a model.ArticleTag, domCat map[string]string) string {
	d := normHost(a.SourceDomain)
	if d != "" {
		if c, ok := domCat[d]; ok {
			return c
		}
		if c, ok := techDomainCategory[d]; ok {
			return c
		}
	}
	if a.Tag.Name != "" {
		// 标签仅用于把内容归入技术领域；标签本身归「资讯」时不算数，交由后续域名/兜底判定。
		if c := normalizeNewsCategory(a.Tag.Name); c != "资讯" {
			return c
		}
	}
	return "资讯"
}

// collectNewsBetween 采集 [start, end) 窗口内入库的资讯，按 DocumentID 去重并分领域归类。
func (s *DailyReportService) collectNewsBetween(start, end time.Time) (*NewsBriefData, error) {
	if s.articleRepo == nil {
		return nil, fmt.Errorf("article repo not configured")
	}
	rows, err := s.articleRepo.ListSince(start, 1000)
	if err != nil {
		return nil, fmt.Errorf("list articles since %s: %w", start.Format(time.RFC3339), err)
	}
	domCat := s.buildDomainCategoryMap()

	seen := make(map[string]struct{})
	byCat := make(map[string][]NewsItem)
	for _, a := range rows {
		// ListSince 只保证 >= start；窗口右界在此裁剪。
		if !a.CreatedAt.Before(end) {
			continue
		}
		if a.ArticleTitle == "" || a.ArticleURL == "" {
			continue
		}
		if isNoiseDomain(normHost(a.SourceDomain)) {
			continue // 社交/视频等纯 UGC 源不进早报（内容仍在知识库/资讯库）
		}
		key := a.DocumentID
		if key == "" {
			key = a.ArticleURL
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		cat := categoryForArticle(a, domCat)
		byCat[cat] = append(byCat[cat], NewsItem{
			Title:    strings.TrimSpace(a.ArticleTitle),
			URL:      a.ArticleURL,
			Domain:   a.SourceDomain,
			Category: cat,
			At:       a.CreatedAt,
		})
	}

	data := &NewsBriefData{
		Date:        end.In(s.loc).Format("2006-01-02"),
		WindowStart: start.In(s.loc).Format("01-02 15:04"),
		WindowEnd:   end.In(s.loc).Format("01-02 15:04"),
	}
	total := 0
	for _, cat := range newsCategoryOrder {
		items := byCat[cat]
		if len(items) == 0 {
			continue
		}
		// 全量候选按序入组；限量交由后续「重要性打分过滤 + 每组上限」（scoreAndFilterGroups）统一处理。
		data.Groups = append(data.Groups, NewsGroup{Category: cat, Items: items})
		total += len(items)
	}
	data.Total = total
	return data, nil
}

// topNews 从各领域按时间倒序取跨领域最新 n 条（供晚间日报「今日资讯速览」内嵌）。
func topNews(data *NewsBriefData, n int) []NewsItem {
	var all []NewsItem
	for _, g := range data.Groups {
		all = append(all, g.Items...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
	if n > 0 && len(all) > n {
		all = all[:n]
	}
	return all
}

// buildNewsSummaryPrompt 构建早报「今日总结」的 LLM 提示词（分领域头条快照）。抽为纯函数：GenerateBrief
// 先构建 prompt 字符串快照、再让总结 LLM 调用与打分并行（避免与打分改写 data.Groups 竞态；总耗时≈max）。
func buildNewsSummaryPrompt(data *NewsBriefData) string {
	var sb strings.Builder
	for _, g := range data.Groups {
		sb.WriteString(fmt.Sprintf("【%s】\n", g.Category))
		for i, it := range g.Items {
			if i >= 15 {
				sb.WriteString(fmt.Sprintf("  （%s 还有 %d 条略）\n", g.Category, len(g.Items)-15))
				break
			}
			src := it.Domain
			if src != "" {
				src = " — " + src
			}
			sb.WriteString(fmt.Sprintf("  - %s%s\n", it.Title, src))
		}
	}

	return fmt.Sprintf(`你是一名资深技术情报编辑。下面是过去约 24 小时（截至今晨）入库的技术资讯标题，已按领域分组。
请据此写一段简洁的中文「今日总结」，用于每日资讯早报开篇。要求：

1. 先写 2-4 句总体概述，点出今天技术圈（编程/AI/网络安全为主）最值得关注的动向或主线；
2. 再用「看点」小标题列出 3-6 条一句话要点，每条突出一个具体事件/趋势，可点名涉及的公司/项目/技术；
3. 若有跨越技术之外但确属重大的事件，可用 1 条带过，但整体以技术为主，不要被泛资讯盖过；
4. 只输出总结正文，不要复述全部标题、不要寒暄、不要加「以下是」之类的开场白；
5. 输出格式（严格）：

<概述段落>

**看点**
- <要点1>
- <要点2>
- ...

今日资讯标题（按领域）：
%s`, sb.String())
}

// chatPool 用指定模型池跑一次补全：优先走 llm_jobs 持久队列，无队列时直调 Gateway 兜底。
// 抽出供早报「今日总结」(pool-summary，质量优先) 与资讯「重要性打分」(pool-chat-free，快，分类任务)复用。
func (s *DailyReportService) chatPool(ctx context.Context, poolModel, callerID, idempotencyKey, prompt string, temperature float64) (string, error) {
	messages := []llmclient.ChatMessage{{Role: "user", Content: prompt}}

	if s.llmJobs != nil {
		opts := llmgateway.EnqueueLLMChatOptions{
			TaskType:    "summary",
			CallerID:    callerID,
			Model:       poolModel,
			Messages:    messages,
			Temperature: temperature,
			Priority:    30,
		}
		if idempotencyKey != "" {
			opts.IdempotencyKey = idempotencyKey
		}
		job, err := s.llmJobs.EnqueueChat(opts)
		if err != nil {
			return "", fmt.Errorf("enqueue llm job: %w", err)
		}
		// Wait 跟随调用方 ctx 的截止时间（GenerateBrief Handler 给 300s 预算）。此前内层固定 180s
		// 比 Handler 预算还短：worker=1 早高峰（08:00 早报 1 总结+4 打分 + pkb-curate cron 抢单 worker）
		// 排队可能 >180s，job 实际已成功却因内层 Wait 提前 context deadline exceeded 回退（AI 总结暂不可用）。
		// 仅当 ctx 无截止时兜底 290s 上限，避免无预算调用方永久阻塞。
		waitCtx := ctx
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			waitCtx, cancel = context.WithTimeout(ctx, 290*time.Second)
			defer cancel()
		}
		done, err := s.llmJobs.Wait(waitCtx, job.ID, time.Second)
		if err != nil {
			return "", fmt.Errorf("wait llm job: %w", err)
		}
		if done.Status != model.LLMJobSuccess {
			return "", llmgateway.LLMJobTerminalError(done)
		}
		return strings.TrimSpace(done.ResponseText), nil
	}

	if s.llm == nil {
		return "", fmt.Errorf("llm client not available")
	}
	callCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	resp, err := s.llm.Chat(callCtx, llmclient.ChatRequest{
		Model:       poolModel,
		Messages:    messages,
		Temperature: temperature,
	}, llmclient.ChatOptions{
		CallerID: callerID,
		TaskType: "summary",
	})
	if err != nil {
		return "", fmt.Errorf("llm summarize: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

// scoreAndFilterGroups 对各领域资讯做「重要性打分过滤 + 每组上限」（ADR：资讯早报只留重大/关键）：
// 每组按时间倒序取候选上限 → LLM 打 0-10 重要性分 → 过阈值 → 按分排序取每组上限。各组并行打分
// （互不相干、只改各自 Items）；打分失败/关闭时回退按时间取前 N（显式 🔶 记日志，不静默、不阻断）。
// 打分后重算 Total 并剔除空组。
func (s *DailyReportService) scoreAndFilterGroups(ctx context.Context, data *NewsBriefData) {
	if data == nil || data.Total == 0 {
		return
	}
	cfg := s.cfg.NewsBrief
	cap := cfg.PerGroupCap
	if cap <= 0 {
		cap = 30
	}
	candCap := cfg.ScoreCandidatesCap
	if candCap <= 0 {
		candCap = 80
	}

	var wg sync.WaitGroup
	for gi := range data.Groups {
		wg.Add(1)
		go func(g *NewsGroup) {
			defer wg.Done()
			s.scoreOneGroup(ctx, g, cfg, cap, candCap)
		}(&data.Groups[gi])
	}
	wg.Wait()

	// 重算 Total + 剔除空组（某组当日无重要资讯时整组消失，符合「只留重要」预期）。
	kept := data.Groups[:0]
	total := 0
	for _, g := range data.Groups {
		if len(g.Items) == 0 {
			continue
		}
		kept = append(kept, g)
		total += len(g.Items)
	}
	data.Groups = kept
	data.Total = total
}

// scoreOneGroup 处理单个领域组：按时间倒序取候选上限，打分启用则 LLM 打分+过滤+取每组上限，
// 否则/打分失败则回退按时间取前 cap。就地写回 g.Items。
func (s *DailyReportService) scoreOneGroup(ctx context.Context, g *NewsGroup, cfg config.NewsBriefConfig, cap, candCap int) {
	if len(g.Items) == 0 {
		return
	}
	sort.Slice(g.Items, func(i, j int) bool { return g.Items[i].At.After(g.Items[j].At) })
	cands := g.Items
	if len(cands) > candCap {
		cands = cands[:candCap]
	}

	if !cfg.ImportanceScoring {
		if len(cands) > cap {
			cands = cands[:cap]
		}
		g.Items = cands
		return
	}

	scores, err := s.scoreNewsImportance(ctx, g.Category, cands)
	if err != nil || len(scores) == 0 {
		log.Printf("[NewsBrief] 🔶 %s 重要性打分失败/空（回退按时间取前 %d，不过滤）: %v", g.Category, cap, err)
		if len(cands) > cap {
			cands = cands[:cap]
		}
		g.Items = cands
		return
	}
	g.Items = selectImportantNews(cands, scores, cfg.ImportanceThreshold, cap)
}

// selectImportantNews 纯逻辑：保留分数≥threshold 的条目（附上分数），按分降序稳定排序，取前 cap。
func selectImportantNews(items []NewsItem, scores map[int]float64, threshold float64, cap int) []NewsItem {
	kept := make([]NewsItem, 0, len(items))
	for i := range items {
		sc, ok := scores[i]
		if !ok || sc < threshold {
			continue // 未打分或低于阈值→不重要，丢弃
		}
		it := items[i]
		it.Score = sc
		kept = append(kept, it)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Score > kept[j].Score })
	if cap > 0 && len(kept) > cap {
		kept = kept[:cap]
	}
	return kept
}

// scoreNewsImportance 让 LLM 给某领域候选资讯逐条打 0-10 重要性分，返回「候选下标(0基)→分数」。
func (s *DailyReportService) scoreNewsImportance(ctx context.Context, category string, items []NewsItem) (map[int]float64, error) {
	var sb strings.Builder
	for i, it := range items {
		src := it.Domain
		if src != "" {
			src = " — " + src
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n", i+1, it.Title, src))
	}

	prompt := fmt.Sprintf(`你是一名资深技术情报编辑，正在为「%s」领域筛选每日资讯早报。
下面是过去约 24 小时入库的资讯标题（含来源）。请给每一条打一个「重要性」整数分（0-10）：
- 8-10：重大发布/严重漏洞(高危CVE)/重要研究突破/影响面很广的行业动向；
- 6-7：值得关注的实质进展、有信息量的技术更新；
- 3-5：常规小更新、一般博客、深度不足；
- 0-2：软文/营销/标题党/招聘/纯八卦/与技术关系很弱/重复边角料。
判据是「对关注技术的读者是否重大、关键、值得知道」。请充分使用整个 0-10 区间，不要给所有条目打相近的分。

只输出打分，每行一条，格式严格（不要理由、不要寒暄、不要复述标题、不要遗漏任何序号）：
<序号>: <0-10 整数>

例如：
1: 8
2: 3
3: 9

资讯列表：
%s`, category, sb.String())

	// 打分是「分类」任务：用快模型池（默认 pool-chat-free），避免慢的 pool-summary(~100s+) 拖超时回退。
	// 打分按候选「下标」映射，对候选集敏感：不设幂等键，每次新鲜打分，避免同日候选集变化时幂等复用
	// 旧分数按下标错配到新列表（总结是整体性的、不敏感，故其幂等键保留）。
	scoreModel := s.cfg.NewsBrief.ScoreModel
	if scoreModel == "" {
		scoreModel = "pool-chat-balanced"
	}
	out, err := s.chatPool(ctx, scoreModel, "news-importance-service", "", prompt, 0.2)
	if err != nil {
		return nil, err
	}
	return parseNewsImportanceScores(out, len(items)), nil
}

// newsScoreLineRe 匹配打分行「<序号>: <分数>」（半/全角冒号，分数可带小数）。
var newsScoreLineRe = regexp.MustCompile(`(?m)^\s*(\d+)\s*[:：]\s*(\d+(?:\.\d+)?)`)

// parseNewsImportanceScores 解析 LLM 打分输出为「候选下标(0基)→分数」；越界序号忽略，分数封顶 10。
func parseNewsImportanceScores(out string, n int) map[int]float64 {
	scores := make(map[int]float64)
	for _, m := range newsScoreLineRe.FindAllStringSubmatch(textutil.StripFence(out), -1) {
		idx, err := strconv.Atoi(m[1])
		if err != nil || idx < 1 || idx > n {
			continue
		}
		val, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		if val > 10 {
			val = 10
		}
		scores[idx-1] = val
	}
	return scores
}
