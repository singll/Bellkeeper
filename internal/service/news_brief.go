package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/model"
)

// 资讯早报（news brief）—— 与运维日报同源但职责不同：
// 早报取「滚动时间窗内真实入库的资讯文章」，分领域列出并附 LLM 全局总结/看点，08:00 推送；
// 运维日报取系统统计+PKB，晚间推送，仅内嵌少量资讯链接指回早报（见 daily_report_render.go）。

// newsCategoryOrder 决定早报分组的展示顺序（技术在前、泛资讯在后）。
var newsCategoryOrder = []string{"编程", "人工智能", "网络安全", "资讯"}

// newsBriefPerGroupCap 单领域早报正文最多展示条数，超出在 render 层折叠为「还有 N 条」。
const newsBriefPerGroupCap = 12

// NewsItem 是一条资讯（去重后一篇文章一条）。
type NewsItem struct {
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Domain   string    `json:"domain,omitempty"`
	Category string    `json:"category"`
	At       time.Time `json:"at"`
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

// categoryForArticle 判定一篇入库文章归哪个早报领域：优先按来源域名匹配 RSS 源分类，
// 退化到关联标签名归一，再退化为「资讯」。
func categoryForArticle(a model.ArticleTag, domCat map[string]string) string {
	d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(a.SourceDomain)), "www.")
	if d != "" {
		if c, ok := domCat[d]; ok {
			return c
		}
	}
	if a.Tag.Name != "" {
		return normalizeNewsCategory(a.Tag.Name)
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
	total := 0
	for _, a := range rows {
		// ListSince 只保证 >= start；窗口右界在此裁剪。
		if !a.CreatedAt.Before(end) {
			continue
		}
		if a.ArticleTitle == "" || a.ArticleURL == "" {
			continue
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
		total++
	}

	data := &NewsBriefData{
		Date:        end.In(s.loc).Format("2006-01-02"),
		WindowStart: start.In(s.loc).Format("01-02 15:04"),
		WindowEnd:   end.In(s.loc).Format("01-02 15:04"),
		Total:       total,
	}
	for _, cat := range newsCategoryOrder {
		items := byCat[cat]
		if len(items) == 0 {
			continue
		}
		data.Groups = append(data.Groups, NewsGroup{Category: cat, Items: items})
	}
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

// generateNewsSummary 让 LLM 基于当日分领域头条产出「全局概述 + 看点」，作为早报开篇。
// 返回一段可直接渲染的中文 markdown 文本（概述段 + 若干「看点」要点）。
func (s *DailyReportService) generateNewsSummary(ctx context.Context, data *NewsBriefData) (string, error) {
	if data.Total == 0 {
		return "", nil
	}

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

	prompt := fmt.Sprintf(`你是一名资深技术情报编辑。下面是过去约 24 小时（截至今晨）入库的技术资讯标题，已按领域分组。
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

	messages := []llmclient.ChatMessage{{Role: "user", Content: prompt}}

	if s.llmJobs != nil {
		job, err := s.llmJobs.EnqueueChat(llmgateway.EnqueueLLMChatOptions{
			TaskType:       "summary",
			CallerID:       "news-brief-service",
			Model:          "pool-summary",
			Messages:       messages,
			Temperature:    0.4,
			Priority:       30,
			IdempotencyKey: llmgateway.LLMJobIdempotencyKey("news-brief-summary", data.Date),
		})
		if err != nil {
			return "", fmt.Errorf("enqueue llm job: %w", err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
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
	callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	resp, err := s.llm.Chat(callCtx, llmclient.ChatRequest{
		Model:       "pool-summary",
		Messages:    messages,
		Temperature: 0.4,
	}, llmclient.ChatOptions{
		CallerID: "news-brief-service",
		TaskType: "summary",
	})
	if err != nil {
		return "", fmt.Errorf("llm summarize: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}
