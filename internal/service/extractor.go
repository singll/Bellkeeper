package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

// ExtractorService handles content extraction from URLs using Trafilatura and Firecrawl
type ExtractorService struct {
	cfg         config.FileIngestionConfig
	httpClient  *http.Client
	activityLog *ActivityLogService
}

// ExtractionRequest represents a request to extract content from a URL
type ExtractionRequest struct {
	URL     string
	Timeout int // timeout in seconds, 0 = use default
}

// ExtractionResult represents the result of content extraction
type ExtractionResult struct {
	Content   string `json:"content"`
	Title     string `json:"title"`
	Extractor string `json:"extractor"` // "trafilatura" or "firecrawl"
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// TrafilaturaOutput represents the JSON output from trafilatura
type TrafilaturaOutput struct {
	Title   string `json:"title"`
	Author  string `json:"author"`
	URL     string `json:"url"`
	Date    string `json:"date"`
	Content string `json:"raw_text"`
}

// FirecrawlRequest represents the request to Firecrawl API
type FirecrawlRequest struct {
	URL     string `json:"url"`
	Formats []string `json:"formats"`
	Timeout int    `json:"timeout,omitempty"`
}

// FirecrawlResponse represents the response from Firecrawl API
type FirecrawlResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Markdown string `json:"markdown"`
		HTML     string `json:"html"`
		Metadata struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"metadata"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// NewExtractorService creates a new ExtractorService
func NewExtractorService(cfg config.FileIngestionConfig) *ExtractorService {
	return &ExtractorService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Firecrawl.Timeout) * time.Second,
		},
	}
}

// SetActivityLog sets the activity log service
func (s *ExtractorService) SetActivityLog(log *ActivityLogService) {
	s.activityLog = log
}

// Extract extracts content from a URL using the configured extractors
func (s *ExtractorService) Extract(req *ExtractionRequest) (*ExtractionResult, error) {
	// Try Trafilatura first if enabled
	if s.cfg.Trafilatura.Enabled {
		result, err := s.extractWithTrafilatura(req)
		if err == nil && result.Success && len(result.Content) >= s.cfg.Trafilatura.MinContentLength {
			s.logExtraction("trafilatura", req.URL, true, "")
			return result, nil
		}
		// Log failure but continue to fallback
		if err != nil {
			s.logExtraction("trafilatura", req.URL, false, err.Error())
		}
	}

	// Fallback to Firecrawl if enabled
	if s.cfg.Firecrawl.Enabled {
		result, err := s.extractWithFirecrawl(req)
		if err == nil && result.Success {
			s.logExtraction("firecrawl", req.URL, true, "")
			return result, nil
		}
		s.logExtraction("firecrawl", req.URL, false, err.Error())
		return nil, fmt.Errorf("firecrawl extraction failed: %w", err)
	}

	return nil, fmt.Errorf("all extractors failed or disabled")
}

// extractWithTrafilatura extracts content using Trafilatura
func (s *ExtractorService) extractWithTrafilatura(req *ExtractionRequest) (*ExtractionResult, error) {
	timeout := s.cfg.Trafilatura.Timeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Run trafilatura command
	cmd := exec.CommandContext(ctx, "python3", "-m", "trafilatura", "-u", req.URL, "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &ExtractionResult{
			Success:   false,
			Extractor: "trafilatura",
			Error:     fmt.Sprintf("command failed: %v, stderr: %s", err, stderr.String()),
		}, err
	}

	// Parse JSON output
	var output TrafilaturaOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return &ExtractionResult{
			Success:   false,
			Extractor: "trafilatura",
			Error:     fmt.Sprintf("failed to parse JSON: %v", err),
		}, err
	}

	// Validate content length
	if len(output.Content) < s.cfg.Trafilatura.MinContentLength {
		return &ExtractionResult{
			Success:   false,
			Extractor: "trafilatura",
			Error:     fmt.Sprintf("content too short: %d bytes", len(output.Content)),
		}, fmt.Errorf("content too short")
	}

	return &ExtractionResult{
		Content:   output.Content,
		Title:     output.Title,
		Extractor: "trafilatura",
		Success:   true,
	}, nil
}

// extractWithFirecrawl extracts content using Firecrawl API
func (s *ExtractorService) extractWithFirecrawl(req *ExtractionRequest) (*ExtractionResult, error) {
	if s.cfg.Firecrawl.APIURL == "" {
		return nil, fmt.Errorf("firecrawl API URL not configured")
	}

	timeout := s.cfg.Firecrawl.Timeout
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	// Prepare request
	fcReq := FirecrawlRequest{
		URL:     req.URL,
		Formats: []string{"markdown"},
		Timeout: timeout * 1000, // Firecrawl expects milliseconds
	}

	body, err := json.Marshal(fcReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make HTTP request
	httpReq, err := http.NewRequest("POST", s.cfg.Firecrawl.APIURL+"/v1/scrape", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var fcResp FirecrawlResponse
	if err := json.Unmarshal(respBody, &fcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !fcResp.Success {
		return &ExtractionResult{
			Success:   false,
			Extractor: "firecrawl",
			Error:     fcResp.Error,
		}, fmt.Errorf("firecrawl error: %s", fcResp.Error)
	}

	// Use markdown content
	content := strings.TrimSpace(fcResp.Data.Markdown)
	if content == "" {
		return &ExtractionResult{
			Success:   false,
			Extractor: "firecrawl",
			Error:     "empty content",
		}, fmt.Errorf("empty content")
	}

	title := fcResp.Data.Metadata.Title
	if title == "" {
		title = "Untitled"
	}

	return &ExtractionResult{
		Content:   content,
		Title:     title,
		Extractor: "firecrawl",
		Success:   true,
	}, nil
}

// logExtraction logs extraction activity
func (s *ExtractorService) logExtraction(extractor, url string, success bool, errMsg string) {
	if s.activityLog == nil {
		return
	}

	status := "success"
	if !success {
		status = "failed"
	}

	summary := fmt.Sprintf("提取器 %s: %s", extractor, url)
	if errMsg != "" {
		summary += fmt.Sprintf(" (错误: %s)", errMsg)
	}

	s.activityLog.LogActivity(LogActivityParams{
		Module:  "extractor",
		Action:  "extract",
		Status:  status,
		Summary: summary,
		Detail: map[string]interface{}{
			"extractor": extractor,
			"url":       url,
			"error":     errMsg,
		},
	})
}
