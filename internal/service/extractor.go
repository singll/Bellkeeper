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
	// 显式声明只要正文，减少无谓下载与噪声。onlyMainContent 服务端默认已 true，
	// 这里显式下发使意图可见、不随上游默认变化；blockAds 屏蔽广告/追踪；
	// removeBase64Images 剔除内联大图 base64，避免灌进 markdown 撑大产物。
	// 真正砍机场流量的是 playwright 端按 resourceType 拦图片/媒体/字体（见 knowledge 主机），
	// 本请求参数是同向补强、对正文抽取质量无损。
	OnlyMainContent    bool `json:"onlyMainContent"`
	BlockAds           bool `json:"blockAds"`
	RemoveBase64Images bool `json:"removeBase64Images"`
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
		cfg: cfg,
		// 本地 HTTP 全程超时（http.Client.Timeout 覆盖含读 body 的整个请求）必须 >
		// 发给 firecrawl 的抓取超时（fcReq.Timeout，见 extractWithFirecrawl），
		// 否则慢响应逼近抓取超时时，本地 deadline 与服务端几乎同时到点掐断连接，
		// 正在执行的 io.ReadAll 得到 "http: read on closed response body"（线上实测散发 unknown）。
		// 预留 30s 传输 headroom：客户端必须比它要求服务端的时间等得更久。
		httpClient:  httpclient.NewClientWithTimeout(time.Duration(cfg.Firecrawl.Timeout+30) * time.Second),
		activityLog: activityLog,
	}
}

// FirecrawlSupportsActions 报告当前 Firecrawl 实例是否支持 scrape actions。
// 供规则优化器在持久化 LLM 产出的 overrides 前净化不受支持的 actions。
func (s *ExtractorService) FirecrawlSupportsActions() bool {
	return s.cfg.Firecrawl.SupportsActions
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
		URL:                req.URL,
		Formats:            []string{"markdown"},
		Timeout:            timeout * 1000,
		OnlyMainContent:    true,
		BlockAds:           true,
		RemoveBase64Images: true,
	}
	if req.Overrides != nil {
		if req.Overrides.FirecrawlWaitFor > 0 {
			fcReq.WaitFor = req.Overrides.FirecrawlWaitFor
		}
		// actions 仅在 Firecrawl 实例确实支持时才下发。自托管开源版无 Fire Engine，
		// 下发 actions 必被拒为 HTTP 400 SCRAPE_ACTIONS_NOT_SUPPORTED（线上实测 client_error
		// 失败大头）。此处显式降级：🔶 丢弃 actions 而非透传注定 400 的请求。
		if len(req.Overrides.FirecrawlActions) > 0 {
			if s.cfg.Firecrawl.SupportsActions {
				fcReq.Actions = req.Overrides.FirecrawlActions
			} else {
				s.logExtraction("firecrawl", req.URL, false,
					fmt.Sprintf("🔶 dropped %d firecrawl actions: instance has no Fire Engine (supports_actions=false)",
						len(req.Overrides.FirecrawlActions)))
			}
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
