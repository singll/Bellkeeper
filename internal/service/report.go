package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ReportService writes markdown reports to the knowledge base.
type ReportService struct {
	basePath string
	mu       sync.Mutex // serialize writes to same file
}

// NewReportService creates a new report service.
func NewReportService(basePath string) *ReportService {
	return &ReportService{
		basePath: basePath,
	}
}

// WriteRequest represents a request to write a report.
type WriteRequest struct {
	Channel   string            `json:"channel" binding:"required"`
	Content   string            `json:"content" binding:"required"`
	Source    string            `json:"source"`
	MessageID string            `json:"message_id"`
	Metadata  map[string]string `json:"metadata"`
}

// WriteResult is the result of a write operation.
type WriteResult struct {
	FilePath   string `json:"file_path"`
	Channel    string `json:"channel"`
	Appended   bool   `json:"appended"`
}

// WriteMessage writes a markdown message to the knowledge base.
// Path: {basePath}/working/messages/{channel}/{date}.md
// If the file already exists today, appends with a separator and increments suffix.
func (s *ReportService) WriteMessage(req *WriteRequest) (*WriteResult, error) {
	if req.Channel == "" || req.Content == "" {
		return nil, fmt.Errorf("channel and content are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	dateStr := now.Format("2006-01-02")
	timestamp := now.Format(time.RFC3339)

	// Build directory: {basePath}/working/messages/{channel}/
	dir := filepath.Join(s.basePath, "working", "messages", req.Channel)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	// Determine file path (handle same-day multiple messages)
	filePath, appended := s.resolveFilePath(dir, dateStr)

	// Build content with frontmatter
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
	sb.WriteString("---\n\n")

	if appended {
		sb.WriteString("---\n\n") // separator between entries
	}
	sb.WriteString(req.Content)
	sb.WriteString("\n")

	// Write: append or create
	var err error
	if appended {
		f, openErr := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
		if openErr != nil {
			return nil, fmt.Errorf("open file: %w", openErr)
		}
		_, err = f.WriteString(sb.String())
		f.Close()
	} else {
		err = os.WriteFile(filePath, []byte(sb.String()), 0644)
	}
	if err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &WriteResult{
		FilePath: filePath,
		Channel:  req.Channel,
		Appended: appended,
	}, nil
}

// resolveFilePath finds the correct file path for today's date, handling multiple messages per day.
func (s *ReportService) resolveFilePath(dir, dateStr string) (string, bool) {
	// Try base file first
	base := filepath.Join(dir, dateStr+".md")
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base, false
	}

	// File exists, find next available suffix
	for i := 1; i < 100; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%s_%d.md", dateStr, i))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, false
		}
	}

	// Fallback: append to the last file
	for i := 99; i >= 0; i-- {
		var path string
		if i == 0 {
			path = filepath.Join(dir, dateStr+".md")
		} else {
			path = filepath.Join(dir, fmt.Sprintf("%s_%d.md", dateStr, i))
		}
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	return base, false
}
