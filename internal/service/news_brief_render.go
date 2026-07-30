package service

import (
	"fmt"
	"strings"
)

// newsCategoryIcon 领域图标，让早报分组一眼可辨。
var newsCategoryIcon = map[string]string{
	"编程":   "💻",
	"人工智能": "🤖",
	"网络安全": "🛡️",
	"资讯":   "📰",
}

// RenderNewsBrief 渲染资讯早报为 Matrix markdown：开篇 AI 总结/看点 + 分领域头条链接。
// 结构清晰、可扫读、突出重点；正文只给标题+链接+来源，不塞全文（全文在知识库/原站）。
func RenderNewsBrief(data *NewsBriefData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## ☀️ 每日资讯早报 · %s\n", data.Date))
	sb.WriteString(fmt.Sprintf("> 🕗 汇总时段 %s → %s（近 %dh）· 共 %d 条\n",
		data.WindowStart, data.WindowEnd, data.WindowHours, data.Total))

	if data.Total == 0 {
		sb.WriteString("\n今日时间窗内暂无新入库资讯。\n")
		return sb.String()
	}

	if strings.TrimSpace(data.AISummary) != "" {
		sb.WriteString("\n### 🧠 今日总结\n")
		sb.WriteString(strings.TrimSpace(data.AISummary) + "\n")
	}

	for _, g := range data.Groups {
		icon := newsCategoryIcon[g.Category]
		if icon == "" {
			icon = "•"
		}
		sb.WriteString(fmt.Sprintf("\n### %s %s（%d）\n", icon, g.Category, len(g.Items)))
		// 条目已由重要性打分过滤 + 每组上限收敛，此处整组列出（不再折叠「还有 N 条」——
		// 看不到被省的条目，折叠提示无意义）。
		for _, it := range g.Items {
			if it.Domain != "" {
				sb.WriteString(fmt.Sprintf("- [%s](%s) · %s\n", newsEscape(it.Title), it.URL, it.Domain))
			} else {
				sb.WriteString(fmt.Sprintf("- [%s](%s)\n", newsEscape(it.Title), it.URL))
			}
		}
	}

	if len(data.Errors) > 0 {
		sb.WriteString("\n#### ⚠️ 数据获取异常\n")
		for _, e := range data.Errors {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Source, e.Error))
		}
	}

	return sb.String()
}

// newsEscape 转义标题里会破坏 markdown 链接的方括号，避免链接显示错乱。
func newsEscape(title string) string {
	r := strings.NewReplacer("[", "【", "]", "】")
	return r.Replace(strings.TrimSpace(title))
}
