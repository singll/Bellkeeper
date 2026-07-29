package service

import (
	"fmt"
	"sort"
	"strings"
)

func RenderDailyReport(data *DailyReportData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("### %s Bellkeeper 日报\n", data.Date))

	sb.WriteString(renderHealthSection(data))
	sb.WriteString(renderCrawlSection(data))
	sb.WriteString(renderRSSIngestSection(data))
	sb.WriteString(renderFileIngestSection(data))
	sb.WriteString(renderClassifySection(data))
	sb.WriteString(renderPKBSection(data))
	sb.WriteString(renderFeedArchiveSection(data))
	sb.WriteString(renderNewsGlanceSection(data))
	sb.WriteString(renderLLMSection(data))
	sb.WriteString(renderFailureSection(data))
	sb.WriteString(renderTodoSection(data))
	sb.WriteString(renderAISummarySection(data))
	sb.WriteString(renderErrorsSection(data))

	return sb.String()
}

func renderHealthSection(data *DailyReportData) string {
	if data.Health == nil {
		return renderErrorSection("服务状态", "health")
	}

	var sb strings.Builder
	sb.WriteString("\n#### 服务状态\n")

	services := data.Health.Services
	if len(services) == 0 {
		sb.WriteString("- ⚠️ 无服务状态数据\n")
		return sb.String()
	}

	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		svc := services[name]
		var icon string
		status := svc.Status
		switch svc.Status {
		case "up":
			icon = "✅"
		case "down":
			icon = "❌"
		case "unhealthy":
			icon = "⚠️"
		case "degraded":
			icon = "⚠️"
		case "disabled":
			icon = "⏸️"
		default:
			icon = "❓"
		}
		line := fmt.Sprintf("- %s **%s**: %s", icon, name, status)
		if svc.LatencyMs > 0 {
			line += fmt.Sprintf(" (%dms)", svc.LatencyMs)
		}
		if svc.Error != "" {
			line += fmt.Sprintf(" — %s", svc.Error)
		}
		sb.WriteString(line + "\n")
	}

	return sb.String()
}

func renderCrawlSection(data *DailyReportData) string {
	if data.Crawl == nil {
		return renderErrorSection("今日爬取", "crawl")
	}

	var sb strings.Builder
	sb.WriteString("\n#### 今日爬取\n")

	c := data.Crawl
	sb.WriteString(fmt.Sprintf("- 新增: %d | 成功: %d | 失败: %d\n", c.TodayNew, c.TodaySuccess, c.TodayFailed))
	if c.TodayNew > 0 {
		sb.WriteString(fmt.Sprintf("- 成功率: %.1f%%\n", c.TodayRate*100))
	}
	sb.WriteString(fmt.Sprintf("- Feed: 总计 %d | 活跃 %d | 暂停 %d\n", c.FeedsTotal, c.FeedsActive, c.FeedsPaused))
	if c.LastCrawlAt != nil {
		sb.WriteString(fmt.Sprintf("- 最近爬取: %s\n", c.LastCrawlAt.Format("15:04:05")))
	}

	return sb.String()
}

func renderRSSIngestSection(data *DailyReportData) string {
	if data.RSSIngest == nil {
		return renderErrorSection("RSS 入库", "rss_ingest")
	}

	var sb strings.Builder
	sb.WriteString("\n#### RSS 入库\n")
	r := data.RSSIngest
	sb.WriteString(fmt.Sprintf("- 成功: %d | 重复: %d | 失败: %d\n", r.Success, r.Duplicate, r.Failure))
	return sb.String()
}

func renderFileIngestSection(data *DailyReportData) string {
	if data.FileIngest == nil {
		return renderErrorSection("文件入库", "file_ingest")
	}

	var sb strings.Builder
	sb.WriteString("\n#### 文件入库\n")
	f := data.FileIngest
	sb.WriteString(fmt.Sprintf("- 成功: %d | 失败: %d\n", f.Success, f.Failure))
	return sb.String()
}

func renderClassifySection(data *DailyReportData) string {
	if data.Classify == nil {
		return renderErrorSection("内容分类", "classify")
	}

	var sb strings.Builder
	sb.WriteString("\n#### 内容分类\n")
	cl := data.Classify
	sb.WriteString(fmt.Sprintf("- 成功: %d | 失败: %d\n", cl.Success, cl.Failure))
	return sb.String()
}

func renderPKBSection(data *DailyReportData) string {
	if data.PKB == nil {
		return renderErrorSection("知识库", "pkb")
	}

	var sb strings.Builder
	sb.WriteString("\n#### 知识库\n")
	p := data.PKB
	sb.WriteString(fmt.Sprintf("- 知识树: %d | 总卡片: %d | 今日新增: %d | 摘要: %d\n",
		p.Trees, p.Cards, p.CardsToday, p.Digests))

	if len(data.PKBCards) > 0 {
		sb.WriteString("- 今日新卡片:\n")
		for i, card := range data.PKBCards {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("  - ...还有 %d 张\n", len(data.PKBCards)-10))
				break
			}
			domain := card.Domain
			if domain != "" {
				sb.WriteString(fmt.Sprintf("  - [%s] %s (%.0f)\n", domain, card.Title, card.Score))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s (%.0f)\n", card.Title, card.Score))
			}
		}
	}

	return sb.String()
}

func renderLLMSection(data *DailyReportData) string {
	if data.LLM == nil {
		return renderErrorSection("LLM 统计", "llm")
	}

	var sb strings.Builder
	sb.WriteString("\n#### LLM 24h 统计\n")
	l := data.LLM
	sb.WriteString(fmt.Sprintf("- 请求: %d | 错误: %d | 限流: %d\n", l.Requests24h, l.Errors24h, l.RateLimits24h))
	sb.WriteString(fmt.Sprintf("- Token: %d | 费用: ¥%.2f\n", l.Tokens24h, float64(l.CostCents24h)/100.0))
	if l.Requests24h > 0 {
		sb.WriteString(fmt.Sprintf("- 成功率: %.1f%% | 平均耗时: %.0fms\n",
			l.SuccessRate*100, l.AvgDurationMs))
	}
	return sb.String()
}

func renderFailureSection(data *DailyReportData) string {
	if len(data.Failures) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n#### 失败明细\n")
	for i, f := range data.Failures {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("- ...还有 %d 条\n", len(data.Failures)-10))
			break
		}
		if f.RefID != "" {
			sb.WriteString(fmt.Sprintf("- %s (ref: %s)\n", f.Summary, f.RefID))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", f.Summary))
		}
	}
	return sb.String()
}

func renderAISummarySection(data *DailyReportData) string {
	if data.AISummary == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n#### AI 亮点总结\n")
	sb.WriteString(data.AISummary + "\n")
	return sb.String()
}

func renderErrorsSection(data *DailyReportData) string {
	if len(data.Errors) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n#### ⚠️ 数据获取异常\n")
	for _, e := range data.Errors {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Source, e.Error))
	}
	return sb.String()
}

func renderErrorSection(title, source string) string {
	return fmt.Sprintf("\n#### %s\n- ⚠️ 数据获取失败: %s\n", title, source)
}

func renderTodoSection(data *DailyReportData) string {
	return "\n#### 待办\n- ⏭️ Memos 待办统计暂未接入（待实现 Memos collector）\n"
}

// renderFeedArchiveSection 渲染日报与资讯库的弱联动（ADR-0005 §5.1）：列出当日各领域的资讯存档链接。
// 资讯条目本身活在 vault/资讯/<领域>/<日期>.md（不入日报正文），此处仅聚合链接、便于跳转查看。
func renderFeedArchiveSection(data *DailyReportData) string {
	if len(data.FeedArchives) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n#### 今日资讯存档\n")
	for _, fa := range data.FeedArchives {
		sb.WriteString(fmt.Sprintf("- [%s](%s)\n", fa.Domain, fa.RelPath))
	}
	return sb.String()
}

// renderNewsGlanceSection 晚间日报内嵌「今日资讯速览」：只列当日最新数条头条链接，
// 指回当日 08:00 的资讯早报（全文/分领域在早报），避免与早报重复铺陈。
func renderNewsGlanceSection(data *DailyReportData) string {
	if len(data.NewsTop) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n#### 今日资讯速览\n")
	for _, it := range data.NewsTop {
		if it.Domain != "" {
			sb.WriteString(fmt.Sprintf("- [%s](%s) · %s\n", newsEscape(it.Title), it.URL, it.Domain))
		} else {
			sb.WriteString(fmt.Sprintf("- [%s](%s)\n", newsEscape(it.Title), it.URL))
		}
	}
	sb.WriteString("- 📩 完整分领域资讯见今晨「每日资讯早报」\n")
	return sb.String()
}
