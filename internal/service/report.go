package service

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ReportService writes markdown reports to the knowledge base.
type ReportService struct {
	basePath        string
	dailyReportPath string
	mu              sync.Mutex // serialize writes to same file
}

// NewReportService creates a new report service.
func NewReportService(basePath string) *ReportService {
	return &ReportService{
		basePath:        basePath,
		dailyReportPath: filepath.Join("vault", "daily"),
	}
}

// SetDailyReportPath overrides the relative path used for the daily channel.
func (s *ReportService) SetDailyReportPath(path string) {
	if strings.TrimSpace(path) != "" {
		s.dailyReportPath = filepath.Clean(path)
	}
}

// WriteRequest represents a request to write a report.
type WriteRequest struct {
	Channel   string            `json:"channel" binding:"required"`
	Content   string            `json:"content" binding:"required"`
	Date      string            `json:"date"`
	Source    string            `json:"source"`
	MessageID string            `json:"message_id"`
	Metadata  map[string]string `json:"metadata"`
}

// WriteResult is the result of a write operation.
type WriteResult struct {
	FilePath    string `json:"file_path"`
	Channel     string `json:"channel"`
	Merged      bool   `json:"merged"`
	NewSections int    `json:"new_sections"`
	NewLines    int    `json:"new_lines"`
}

// WriteMessage writes a markdown message to the knowledge base.
// Path: daily reports go to {basePath}/{dailyReportPath}/{date}.md.
// Other channels keep the legacy path {basePath}/working/messages/{channel}/{date}.md.
// If the file already exists, performs incremental merge: only adds new content,
// never deletes or overwrites existing sections.
func (s *ReportService) WriteMessage(req *WriteRequest) (*WriteResult, error) {
	if req.Channel == "" || req.Content == "" {
		return nil, fmt.Errorf("channel and content are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	dateStr, err := reportDateString(req.Date, now)
	if err != nil {
		return nil, err
	}
	timestamp := now.Format(time.RFC3339)

	dir := s.channelDir(req.Channel)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	filePath := filepath.Join(dir, dateStr+".md")

	// Build frontmatter for this entry
	frontmatter := s.buildFrontmatter(req, timestamp)

	// Strip frontmatter from existing content for merge comparison
	newContent := req.Content

	// Check if file already exists
	existingBytes, err := os.ReadFile(filePath)
	if err == nil && len(existingBytes) > 0 {
		// File exists — do incremental merge
		existingContent := stripFrontmatter(string(existingBytes))
		merged, newSections, newLines := mergeMarkdown(existingContent, newContent)
		finalContent := frontmatter + "\n\n" + merged + "\n"
		if err := os.WriteFile(filePath, []byte(finalContent), 0644); err != nil {
			return nil, fmt.Errorf("write merged file: %w", err)
		}
		return &WriteResult{
			FilePath:    filePath,
			Channel:     req.Channel,
			Merged:      true,
			NewSections: newSections,
			NewLines:    newLines,
		}, nil
	}

	// File does not exist — create new
	finalContent := frontmatter + "\n\n" + newContent + "\n"
	if err := os.WriteFile(filePath, []byte(finalContent), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &WriteResult{
		FilePath:    filePath,
		Channel:     req.Channel,
		Merged:      false,
		NewSections: 0,
		NewLines:    0,
	}, nil
}

func reportDateString(raw string, fallback time.Time) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback.Format("2006-01-02"), nil
	}
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return "", fmt.Errorf("date must be YYYY-MM-DD: %w", err)
	}
	return raw, nil
}

func (s *ReportService) channelDir(channel string) string {
	if channel == "daily" {
		return filepath.Join(s.basePath, s.dailyReportPath)
	}
	return filepath.Join(s.basePath, "working", "messages", channel)
}

// buildFrontmatter generates YAML frontmatter for a report entry.
func (s *ReportService) buildFrontmatter(req *WriteRequest, timestamp string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("created: %s\n", timestamp))
	sb.WriteString(fmt.Sprintf("channel: %s\n", req.Channel))
	if req.Source != "" {
		sb.WriteString(fmt.Sprintf("source: %s\n", req.Source))
	}
	if req.MessageID != "" {
		sb.WriteString(fmt.Sprintf("message_id: %s\n", req.MessageID))
	}
	if len(req.Metadata) > 0 {
		for k, v := range req.Metadata {
			sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
	}
	sb.WriteString("tags: [bot-message, ")
	sb.WriteString(req.Channel)
	sb.WriteString("]\n")
	sb.WriteString("---")
	return sb.String()
}

// stripFrontmatter removes YAML frontmatter (--- ... ---) from markdown content.
func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return content
	}
	// Skip past the closing ---
	rest := content[3+end+3:]
	rest = strings.TrimLeft(rest, "\n\r")
	return rest
}

// markdownSection represents a section of markdown delimited by #### headings.
type markdownSection struct {
	heading string // e.g. "#### 服务状态"
	body    string // content after the heading until next #### or EOF
}

// parseSections splits markdown into the title line (### heading) and #### sections.
func parseSections(content string) (string, []markdownSection) {
	var title string
	var sections []markdownSection

	scanner := bufio.NewScanner(strings.NewReader(content))
	var current *markdownSection
	var bodyLines []string

	flush := func() {
		if current != nil {
			current.body = strings.TrimRight(strings.Join(bodyLines, "\n"), "\n\r")
			sections = append(sections, *current)
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#### ") {
			flush()
			current = &markdownSection{heading: line}
			bodyLines = nil
		} else if strings.HasPrefix(line, "### ") && title == "" {
			title = line
		} else if current != nil {
			bodyLines = append(bodyLines, line)
		} else {
			// Content before first #### (after title) — ignore for section merge
		}
	}
	flush()
	return title, sections
}

// mergeMarkdown merges newContent into existingContent with incremental-only logic.
// - Existing sections are preserved as-is
// - New sections (#### headings not in existing) are appended
// - For sections present in both: existing body is kept, new lines from the new body are appended
// - Lines already in existing (exact match after trim) are not duplicated
func mergeMarkdown(existingContent, newContent string) (string, int, int) {
	_, existingSections := parseSections(existingContent)
	_, newSections := parseSections(newContent)

	// Index existing sections by heading
	existingMap := make(map[string]*markdownSection)
	for i := range existingSections {
		existingMap[existingSections[i].heading] = &existingSections[i]
	}

	totalNewSections := 0
	totalNewLines := 0

	// Track which new sections were matched
	matchedNewSections := make(map[int]bool)

	for ni, ns := range newSections {
		es, found := existingMap[ns.heading]
		if !found {
			// New section doesn't exist — will be appended
			totalNewSections++
			continue
		}
		matchedNewSections[ni] = true

		// Section exists in both — merge body incrementally
		existingLines := strings.Split(es.body, "\n")
		existingSet := make(map[string]bool)
		for _, l := range existingLines {
			trimmed := strings.TrimSpace(l)
			if trimmed != "" {
				existingSet[trimmed] = true
			}
		}

		newLines := strings.Split(ns.body, "\n")
		var appended []string
		for _, l := range newLines {
			trimmed := strings.TrimSpace(l)
			if trimmed != "" && !existingSet[trimmed] {
				appended = append(appended, l)
				existingSet[trimmed] = true
			}
		}

		if len(appended) > 0 {
			// Append new lines to existing section body
			if es.body != "" && !strings.HasSuffix(es.body, "\n") {
				es.body += "\n"
			}
			es.body += strings.Join(appended, "\n")
			totalNewLines += len(appended)
		}
	}

	// Build result: existing sections (with merged content) in original order,
	// then new sections appended at the end.
	var sb strings.Builder

	// Write existing sections (preserving order)
	for i, s := range existingSections {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(s.heading)
		sb.WriteString("\n")
		if s.body != "" {
			sb.WriteString(s.body)
		}
	}

	// Append new sections
	for ni, ns := range newSections {
		if matchedNewSections[ni] {
			continue
		}
		sb.WriteString("\n\n")
		sb.WriteString(ns.heading)
		sb.WriteString("\n")
		if ns.body != "" {
			sb.WriteString(ns.body)
		}
	}

	return sb.String(), totalNewSections, totalNewLines
}
