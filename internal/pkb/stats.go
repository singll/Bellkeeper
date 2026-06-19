package pkb

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DomainStat 是单个领域的状态概览（供前端「知识骨架」总览展示）。
type DomainStat struct {
	Name           string `json:"name"`
	Display        string `json:"display"`
	Feed           bool   `json:"feed"`
	IsDefault      bool   `json:"is_default"`
	HasScope       bool   `json:"has_scope"`       // 是否配了 scope（知识域才能生成骨架）
	HasSkeleton    bool   `json:"has_skeleton"`    // _index.md 是否存在（已播种骨架）
	Cards          int    `json:"cards"`           // 原子卡数（type:pkb_card）
	CardsToday     int    `json:"cards_today"`     // 当天新增（frontmatter 入库日期）
	CardsWeek      int    `json:"cards_week"`      // 近 7 天新增
	SkeletonGaps   int    `json:"skeleton_gaps"`   // 骨架 [缺口] 节点数
	SkeletonFilled int    `json:"skeleton_filled"` // 骨架已挂卡节点数（[[..]]）
	Waitlist       int    `json:"waitlist"`        // 待归位卡数
	LowConfidence  int    `json:"low_confidence"`  // 低置信卡数（unverified/llm-only）
	LastDigestAt   string `json:"last_digest_at"`  // 最近 digest 生成时间（RFC3339，空=无）
	// SkeletonPending 标识该域是否正在排队/生成骨架（由 handler 据 scheduler 队列填，
	// DomainStatsOverview 本身不设——它只扫文件系统）。
	SkeletonPending bool `json:"skeleton_pending"`
	// WaitlistHigh / LowConfidenceHigh 标识待归位/低置信卡数是否达「需要关注」阈值
	// （由 handler 据 config 阈值填，同 SkeletonPending 范式；DomainStatsOverview 本身不设）。
	WaitlistHigh      bool `json:"waitlist_high"`
	LowConfidenceHigh bool `json:"low_confidence_high"`
}

// DomainStatsOverview 扫描各领域 vault 目录，汇总每域的卡片/骨架/待归位/低置信/最近 digest
// 状态，供前端总览。纯文件系统只读、不调 LLM。放在 pkb 包以复用骨架/frontmatter 解析
// （service 包不能 import pkb——pkb 已 import service，会循环依赖）。
func DomainStatsOverview(basePath, domainsPath string) ([]DomainStat, error) {
	dc, err := LoadDomains(domainsPath)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -6) // 含今天共 7 天

	stats := make([]DomainStat, 0, len(dc.Domains))
	for _, dom := range dc.Domains {
		st := DomainStat{
			Name:      dom.Name,
			Display:   dom.Display,
			Feed:      dom.Feed,
			IsDefault: dom.IsDefault,
			HasScope:  strings.TrimSpace(dom.Scope) != "",
		}
		root := filepath.Join(basePath, dom.VaultSubpath)
		if _, statErr := os.Stat(root); statErr == nil {
			collectDomainStat(root, &st, todayStart, weekStart)
		}
		stats = append(stats, st)
	}
	return stats, nil
}

// collectDomainStat 扫描单个领域目录，填充状态字段。
func collectDomainStat(root string, st *DomainStat, todayStart, weekStart time.Time) {
	// 骨架 _index.md：缺口 / 已挂节点数。
	if data, err := os.ReadFile(filepath.Join(root, "_index.md")); err == nil {
		st.HasSkeleton = true
		tree := extractSection(string(data), "## 知识树")
		st.SkeletonGaps = strings.Count(tree, "[缺口]")
		st.SkeletonFilled = strings.Count(tree, "[[")
	}
	// 待归位 _待归位.md：含 wikilink 的行数。
	if data, err := os.ReadFile(filepath.Join(root, "_待归位.md")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "[[") {
				st.Waitlist++
			}
		}
	}
	// 原子卡：顶层 *.md（排除 _ 前缀文件与子目录 topics/digest/maps）。
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(root, e.Name()))
			if readErr != nil {
				continue
			}
			fm := parseFrontmatterMap(string(data))
			if fm["type"] != "pkb_card" {
				continue
			}
			st.Cards++
			if isLowConfidence(fm["verification"]) {
				st.LowConfidence++
			}
			if t, ok := cardIngestDate(fm); ok {
				if !t.Before(todayStart) {
					st.CardsToday++
				}
				if !t.Before(weekStart) {
					st.CardsWeek++
				}
			}
		}
	}
	// 最近 digest：digest/ 子目录最新 .md 的修改时间。
	if digs, err := os.ReadDir(filepath.Join(root, "digest")); err == nil {
		var latest time.Time
		for _, dg := range digs {
			if dg.IsDir() || filepath.Ext(dg.Name()) != ".md" {
				continue
			}
			if info, infoErr := dg.Info(); infoErr == nil && info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		}
		if !latest.IsZero() {
			st.LastDigestAt = latest.Format(time.RFC3339)
		}
	}
}

// cardIngestDate 取卡片入库日期（frontmatter，优先 ingest_date > pkb_scored_at > ingested_at > created）。
func cardIngestDate(fm map[string]string) (time.Time, bool) {
	for _, k := range []string{"ingest_date", "pkb_scored_at", "ingested_at", "created"} {
		if v := strings.TrimSpace(fm[k]); v != "" {
			if t, ok := parseFlexibleDate(v); ok {
				return t, true
			}
		}
	}
	return time.Time{}, false
}
