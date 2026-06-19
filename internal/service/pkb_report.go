package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/sanitizer"
)

const defaultPKBDailyLimit = 20

// PKBReportService exposes read-only PKB facts for daily report workflows.
type PKBReportService struct {
	basePath string
	activity *ActivityLogService
	loc      *time.Location
}

func NewPKBReportService(cfg config.KnowledgeConfig, dailyCfg config.DailyReportConfig, activity *ActivityLogService) *PKBReportService {
	loc := time.Local
	if dailyCfg.Timezone != "" {
		if loaded, err := time.LoadLocation(dailyCfg.Timezone); err == nil {
			loc = loaded
		}
	}
	return &PKBReportService{
		basePath: cfg.BasePath,
		activity: activity,
		loc:      loc,
	}
}

type PKBDailyReport struct {
	Date        string              `json:"date"`
	Cards       []PKBCardSummary    `json:"cards"`
	Digests     []PKBDigestSummary  `json:"digests"`
	Maintenance PKBMaintenanceStats `json:"maintenance"`
}

type PKBCardSummary struct {
	Title    string    `json:"title"`
	RelPath  string    `json:"rel_path"`
	Domain   string    `json:"domain"`
	Score    float64   `json:"score"`
	Tags     []string  `json:"tags"`
	Date     time.Time `json:"date"`
	Excerpt  string    `json:"excerpt"`
	Modified time.Time `json:"modified"`
}

type PKBDigestSummary struct {
	Title     string    `json:"title"`
	RelPath   string    `json:"rel_path"`
	Domain    string    `json:"domain"`
	Period    string    `json:"period"`
	Generated time.Time `json:"generated_at,omitempty"`
	Modified  time.Time `json:"modified"`
}

type PKBMaintenanceStats struct {
	Total int64               `json:"total"`
	Items []model.ActivityLog `json:"items"`
}

func (s *PKBReportService) Daily(date time.Time, limit int) (*PKBDailyReport, error) {
	if limit <= 0 {
		limit = defaultPKBDailyLimit
	}
	dateStr := date.Format("2006-01-02")

	cards, err := s.VaultCardsByDate(dateStr, limit)
	if err != nil {
		return nil, err
	}
	digests, err := s.LatestDigests()
	if err != nil {
		return nil, err
	}

	maintenance := PKBMaintenanceStats{}
	if s.activity != nil {
		start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		page, err := s.activity.List(ListActivityLogsQuery{
			Module: "pkb",
			Since:  start,
			Page:   1,
			Limit:  50,
		})
		if err != nil {
			return nil, fmt.Errorf("list pkb maintenance logs: %w", err)
		}
		maintenance.Total = page.Total
		maintenance.Items = page.Items
	}

	return &PKBDailyReport{
		Date:        dateStr,
		Cards:       cards,
		Digests:     digests,
		Maintenance: maintenance,
	}, nil
}

func (s *PKBReportService) ReportDate(raw string) (time.Time, bool) {
	loc := time.Local
	if s != nil && s.loc != nil {
		loc = s.loc
	}
	if raw == "" {
		now := time.Now().In(loc)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc), true
	}
	t, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func (s *PKBReportService) VaultCardsByDate(date string, limit int) ([]PKBCardSummary, error) {
	if limit <= 0 {
		limit = defaultPKBDailyLimit
	}
	var cards []PKBCardSummary
	root := filepath.Join(s.basePath, "vault")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipPKBReportDir(path, root, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isMarkdownFile(d.Name()) || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		card, ok := s.readVaultCard(path, date)
		if ok {
			cards = append(cards, card)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk vault cards: %w", err)
	}
	sort.Slice(cards, func(i, j int) bool {
		if !cards[i].Date.Equal(cards[j].Date) {
			return cards[i].Date.After(cards[j].Date)
		}
		if cards[i].Score != cards[j].Score {
			return cards[i].Score > cards[j].Score
		}
		return cards[i].Title < cards[j].Title
	})
	if len(cards) > limit {
		cards = cards[:limit]
	}
	return cards, nil
}

// PKBVaultStats holds aggregate counts of the PKB vault.
type PKBVaultStats struct {
	Trees      int   `json:"trees"`       // top-level knowledge trees (domains) under vault/
	Cards      int64 `json:"cards"`       // knowledge cards (markdown files, excluding daily/digest/maps/topics)
	CardsToday int64 `json:"cards_today"` // cards modified today
	Digests    int64 `json:"digests"`     // digest documents across all domains
}

// VaultStats walks the vault directory and returns aggregate counts.
func (s *PKBReportService) VaultStats() (*PKBVaultStats, error) {
	stats := &PKBVaultStats{}
	root := filepath.Join(s.basePath, "vault")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return nil, fmt.Errorf("read vault root: %w", err)
	}

	now := time.Now().In(s.loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") ||
			strings.HasPrefix(entry.Name(), "_") || entry.Name() == "daily" || entry.Name() == "资讯" {
			continue
		}
		stats.Trees++
		digestDir := filepath.Join(root, entry.Name(), "digest")
		digestEntries, err := os.ReadDir(digestDir)
		if err != nil {
			continue
		}
		for _, de := range digestEntries {
			if !de.IsDir() && isMarkdownFile(de.Name()) {
				stats.Digests++
			}
		}
	}

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipPKBReportDir(path, root, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isMarkdownFile(d.Name()) || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		stats.Cards++
		if info, infoErr := d.Info(); infoErr == nil && !info.ModTime().Before(dayStart) {
			stats.CardsToday++
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk vault stats: %w", walkErr)
	}
	return stats, nil
}

// PKBFeedArchive 是某领域当日资讯库存档的轻量引用（供日报「今日资讯存档」弱联动，ADR-0005 §5.1）。
type PKBFeedArchive struct {
	Domain  string `json:"domain"`
	RelPath string `json:"rel_path"`
}

// FeedArchivesByDate 列出当日各领域的资讯库存档 vault/资讯/<领域>/<date>.md（feed 容器目录，
// 目录名与 domains.yaml feed 领域 vault_subpath 末段一致）。供日报聚合链接当日资讯；无则返回空。
func (s *PKBReportService) FeedArchivesByDate(date string) ([]PKBFeedArchive, error) {
	feedRoot := filepath.Join(s.basePath, "vault", "资讯")
	entries, err := os.ReadDir(feedRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read feed root: %w", err)
	}
	var out []PKBFeedArchive
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		f := filepath.Join(feedRoot, e.Name(), date+".md")
		if _, statErr := os.Stat(f); statErr != nil {
			continue
		}
		rel, relErr := filepath.Rel(s.basePath, f)
		if relErr != nil {
			rel = f
		}
		out = append(out, PKBFeedArchive{Domain: e.Name(), RelPath: filepath.ToSlash(rel)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out, nil
}

// PKBFeedDay 是某一天的资讯库存档集合（供总览「资讯时间线」按日浏览）。
type PKBFeedDay struct {
	Date     string           `json:"date"`
	Archives []PKBFeedArchive `json:"archives"`
}

// PKBFeedArchiveContent 是单篇资讯库每日存档的只读渲染结果（ADR-0006 唯一例外：资讯库
// 每日存档允许 Web 只读渲染）。HTML 已服务端 goldmark 渲染 + bluemonday 清洗。
type PKBFeedArchiveContent struct {
	Date   string `json:"date"`
	Domain string `json:"domain"`
	Title  string `json:"title"`
	HTML   string `json:"html"`
}

const maxFeedTimelineDays = 60

// FeedTimeline 列出最近 days 天有资讯库存档的日子（自 before 前一天起往前数；before 空=从今天起）。
// 逐日复用 FeedArchivesByDate，仅保留有存档的天；before 供「往前翻全部历史」分页（传当前最旧日期，
// 取严格更早的那批）。供总览资讯时间线只读浏览（ADR-0006 例外条款）。
func (s *PKBReportService) FeedTimeline(days int, before string) ([]PKBFeedDay, error) {
	if days <= 0 {
		days = 14
	}
	if days > maxFeedTimelineDays {
		days = maxFeedTimelineDays
	}
	start, ok := s.ReportDate("")
	if !ok {
		return nil, fmt.Errorf("resolve today failed")
	}
	if before != "" {
		b, valid := s.ReportDate(before)
		if !valid {
			return nil, fmt.Errorf("invalid before date; expected YYYY-MM-DD")
		}
		start = b.AddDate(0, 0, -1) // 严格早于 before，支持分页续翻
	}
	out := make([]PKBFeedDay, 0, days)
	for i := 0; i < days; i++ {
		dateStr := start.AddDate(0, 0, -i).Format("2006-01-02")
		archives, err := s.FeedArchivesByDate(dateStr)
		if err != nil {
			return nil, err
		}
		if len(archives) == 0 {
			continue
		}
		out = append(out, PKBFeedDay{Date: dateStr, Archives: archives})
	}
	return out, nil
}

// FeedArchiveHTML 读取并只读渲染单篇资讯库每日存档 vault/资讯/<domain>/<date>.md。
// 严格校验 date/domain 防路径穿越；剥 frontmatter → goldmark → bluemonday 清洗。
// 仅用于资讯库存档（ADR-0006 例外），不得据此渲染知识卡片正文。
func (s *PKBReportService) FeedArchiveHTML(date, domain string) (PKBFeedArchiveContent, error) {
	if _, ok := s.ReportDate(date); !ok {
		return PKBFeedArchiveContent{}, fmt.Errorf("invalid date; expected YYYY-MM-DD")
	}
	if domain == "" || strings.ContainsAny(domain, `/\`) || strings.Contains(domain, "..") {
		return PKBFeedArchiveContent{}, fmt.Errorf("invalid domain")
	}
	path := filepath.Join(s.basePath, "vault", "资讯", domain, date+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PKBFeedArchiveContent{}, os.ErrNotExist
		}
		return PKBFeedArchiveContent{}, fmt.Errorf("read feed archive: %w", err)
	}
	body := stripReportFrontmatter(string(raw))
	title := firstMarkdownHeading(body)
	if title == "" {
		title = domain + " · " + date
	}
	html := sanitizer.SanitizeHTML(markdownToHTML(body))
	return PKBFeedArchiveContent{Date: date, Domain: domain, Title: title, HTML: html}, nil
}

// firstMarkdownHeading 取首个一级标题（# ）文本，无则空。
func firstMarkdownHeading(md string) string {
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "# "))
		}
	}
	return ""
}

func (s *PKBReportService) LatestDigests() ([]PKBDigestSummary, error) {
	root := filepath.Join(s.basePath, "vault")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []PKBDigestSummary{}, nil
		}
		return nil, fmt.Errorf("read vault root: %w", err)
	}

	var out []PKBDigestSummary
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "daily" {
			continue
		}
		digestDir := filepath.Join(root, entry.Name(), "digest")
		latest, ok, err := s.latestDigestInDir(entry.Name(), digestDir)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, latest)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Domain < out[j].Domain
	})
	return out, nil
}

func (s *PKBReportService) latestDigestInDir(domain, dir string) (PKBDigestSummary, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return PKBDigestSummary{}, false, nil
		}
		return PKBDigestSummary{}, false, fmt.Errorf("read digest dir %s: %w", dir, err)
	}
	var latest PKBDigestSummary
	for _, entry := range entries {
		if entry.IsDir() || !isMarkdownFile(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fm := parseReportFrontmatter(string(data))
		rel, err := filepath.Rel(s.basePath, path)
		if err != nil {
			rel = path
		}
		generated, _ := parseReportDate(firstReportValue(fm["generated_at"], fm["created"]))
		item := PKBDigestSummary{
			Title:     cleanReportValue(firstReportValue(fm["title"], strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))),
			RelPath:   filepath.ToSlash(rel),
			Domain:    cleanReportValue(firstReportValue(fm["domain"], domain)),
			Period:    cleanReportValue(fm["period"]),
			Generated: generated,
			Modified:  info.ModTime(),
		}
		if latest.RelPath == "" || item.Modified.After(latest.Modified) {
			latest = item
		}
	}
	return latest, latest.RelPath != "", nil
}

func (s *PKBReportService) readVaultCard(path, date string) (PKBCardSummary, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PKBCardSummary{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return PKBCardSummary{}, false
	}
	content := string(data)
	fm := parseReportFrontmatter(content)
	score := parseReportScore(firstReportValue(fm["pkb_score"], fm["score"]))
	if score <= 0 && cleanReportValue(fm["pkb_decision"]) != "vault" {
		return PKBCardSummary{}, false
	}

	cardDate := info.ModTime()
	if parsed, ok := parseReportDate(firstReportValue(fm["ingest_date"], fm["pkb_scored_at"], fm["created"])); ok {
		cardDate = parsed
	}
	if date != "" && cardDate.Format("2006-01-02") != date {
		return PKBCardSummary{}, false
	}

	rel, err := filepath.Rel(s.basePath, path)
	if err != nil {
		rel = path
	}
	domain := ""
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 2 && parts[0] == "vault" {
		domain = parts[1]
	}
	title := firstReportValue(fm["title"], strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	return PKBCardSummary{
		Title:    cleanReportValue(title),
		RelPath:  filepath.ToSlash(rel),
		Domain:   domain,
		Score:    score,
		Tags:     parseReportTags(fm["tags"]),
		Date:     cardDate,
		Excerpt:  reportExcerpt(stripReportFrontmatter(content), 280),
		Modified: info.ModTime(),
	}, true
}

func shouldSkipPKBReportDir(path, root, name string) bool {
	if path == root {
		return false
	}
	switch name {
	// "资讯" = 资讯库容器目录（ADR-0005，domains.yaml feed 领域 vault_subpath 末段）：
	// 分领域分日资讯存档不是知识原子卡，排除出 VaultCardsByDate/VaultStats 的知识卡遍历。
	case "daily", "digest", "maps", "topics", "资讯":
		return true
	default:
		return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
	}
}

func isMarkdownFile(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".md")
}

func parseReportFrontmatter(content string) map[string]string {
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

func stripReportFrontmatter(content string) string {
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

var reportFinalScoreRe = regexp.MustCompile(`final=([0-9]+(?:\.[0-9]+)?)`)

func parseReportScore(raw string) float64 {
	raw = cleanReportValue(raw)
	if raw == "" {
		return 0
	}
	if m := reportFinalScoreRe.FindStringSubmatch(raw); len(m) == 2 {
		raw = m[1]
	}
	score, _ := strconv.ParseFloat(raw, 64)
	return score
}

func parseReportDate(raw string) (time.Time, bool) {
	raw = cleanReportValue(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{time.RFC3339, "2006-01-02", "20060102", "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseReportTags(raw string) []string {
	raw = cleanReportValue(raw)
	raw = strings.Trim(raw, "[]")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.Trim(strings.TrimSpace(part), `"'#`)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func cleanReportValue(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), `"'`)
}

func firstReportValue(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func reportExcerpt(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}
