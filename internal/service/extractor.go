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
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
)

// ExtractorService handles content extraction from URLs using Trafilatura and Firecrawl
type ExtractorService struct {
	cfg         config.FileIngestionConfig
	httpClient  *httpclient.Client
	activityLog *ActivityLogService
}

// RequestOverrides holds per-domain extraction tuning (User-Agent, headers,
// timeout, preferred strategy, Firecrawl wait/actions). Populated by the rule
// optimizer and persisted on crawl_domain_profiles; applied here at extract time.
type RequestOverrides struct {
	UserAgent        string            `json:"user_agent,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Strategy         string            `json:"strategy,omitempty"`
	FirecrawlWaitFor int               `json:"firecrawl_wait_for,omitempty"`
	FirecrawlActions []FirecrawlAction `json:"firecrawl_actions,omitempty"`
}

// FirecrawlAction is a scripted Firecrawl interaction (e.g. click a consent button).
type FirecrawlAction struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value,omitempty"`
}

type ExtractionRequest struct {
	URL     string
	Timeout int
	Overrides *RequestOverrides
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
	URL     string            `json:"url"`
	Formats []string          `json:"formats"`
	Timeout int               `json:"timeout,omitempty"`
	WaitFor int               `json:"waitFor,omitempty"`
	Actions []FirecrawlAction `json:"actions,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
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
func NewExtractorService(cfg config.FileIngestionConfig, activityLog *ActivityLogService) *ExtractorService {
	return &ExtractorService{
		cfg:         cfg,
		httpClient:  httpclient.NewClientWithTimeout(time.Duration(cfg.Firecrawl.Timeout) * time.Second),
		activityLog: activityLog,
	}
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
	if req.Overrides != nil && req.Overrides.TimeoutSeconds > 0 {
		timeout = req.Overrides.TimeoutSeconds
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	args := []string{"/app/scripts/trafilatura_extract.py", req.URL, "--timeout", fmt.Sprintf("%d", timeout)}
	if req.Overrides != nil {
		if req.Overrides.UserAgent != "" {
			args = append(args, "--user-agent", req.Overrides.UserAgent)
		}
		if len(req.Overrides.Headers) > 0 {
			if hdrJSON, err := json.Marshal(req.Overrides.Headers); err == nil {
				args = append(args, "--headers", string(hdrJSON))
			}
		}
	}

	cmd := exec.CommandContext(ctx, "python3", args...)
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
	if req.Overrides != nil && req.Overrides.TimeoutSeconds > 0 {
		timeout = req.Overrides.TimeoutSeconds
	}

	fcReq := FirecrawlRequest{
		URL:     req.URL,
		Formats: []string{"markdown"},
		Timeout: timeout * 1000,
	}
	if req.Overrides != nil {
		if req.Overrides.FirecrawlWaitFor > 0 {
			fcReq.WaitFor = req.Overrides.FirecrawlWaitFor
		}
		if len(req.Overrides.FirecrawlActions) > 0 {
			fcReq.Actions = req.Overrides.FirecrawlActions
		}
		if len(req.Overrides.Headers) > 0 {
			fcReq.Headers = req.Overrides.Headers
		}
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
	defer resp.Body.Close() //nolint:errcheck

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			return nil, fmt.Errorf("HTTP %d retry_after=%q: %s", resp.StatusCode, retryAfter, string(respBody))
		}
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
