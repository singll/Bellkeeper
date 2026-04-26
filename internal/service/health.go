package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/singll/bellkeeper/internal/repository"
)

type HealthService struct {
	cfg      *config.Config
	version  string
	tagRepo  *repository.TagRepository
	rssRepo  *repository.RSSRepository
	dataRepo *repository.DatasetMappingRepository
}

func NewHealthService(
	cfg *config.Config,
	version string,
	tagRepo *repository.TagRepository,
	rssRepo *repository.RSSRepository,
	dataRepo *repository.DatasetMappingRepository,
) *HealthService {
	return &HealthService{
		cfg:      cfg,
		version:  version,
		tagRepo:  tagRepo,
		rssRepo:  rssRepo,
		dataRepo: dataRepo,
	}
}

type ServiceStatus struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type DetailedHealth struct {
	Status   string                   `json:"status"`
	Version  string                   `json:"version,omitempty"`
	Services map[string]ServiceStatus `json:"services"`
	Metrics  map[string]interface{}   `json:"metrics,omitempty"`
}

func (s *HealthService) Check() map[string]string {
	return map[string]string{
		"status":  "healthy",
		"version": s.version,
	}
}

func (s *HealthService) Detailed() *DetailedHealth {
	services := make(map[string]ServiceStatus)

	// Check RagFlow (requires API key auth)
	services["ragflow"] = s.checkRagFlow()

	// Check n8n (使用 API 端点 + API Key 认证，避免根路径 404 误判)
	if s.cfg.N8N.APIBaseURL != "" && s.cfg.N8N.APIKey != "" {
		services["n8n"] = s.checkN8N()
	}

	// Check Meilisearch (only if enabled in knowledge config)
	if s.cfg.Meilisearch.URL != "" {
		services["meilisearch"] = s.checkHTTPService(s.cfg.Meilisearch.URL + "/health")
	}

	// Check RSS Fetcher status
	services["rss_fetcher"] = ServiceStatus{
		Status: s.checkRSSFetcher(),
	}

	// Determine overall status
	overallStatus := "healthy"
	for _, svc := range services {
		if svc.Status != "up" {
			overallStatus = "degraded"
			break
		}
	}

	// Get statistics from database
	metrics := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if s.tagRepo != nil {
		if tags, total, _ := s.tagRepo.List(1, 1, ""); total > 0 || len(tags) >= 0 {
			metrics["tags_count"] = total
		}
	}

	if s.rssRepo != nil {
		if _, total, _ := s.rssRepo.List(1, 1, "", ""); total > 0 {
			metrics["rss_feeds_count"] = total
		}
	}

	if s.dataRepo != nil {
		if _, total, _ := s.dataRepo.List(1, 1); total > 0 {
			metrics["datasets_count"] = total
		}
	}

	return &DetailedHealth{
		Status:   overallStatus,
		Version:  s.version,
		Services: services,
		Metrics:  metrics,
	}
}

// checkN8N checks n8n health via API endpoint with API key authentication.
// n8n 根路径返回 404 会导致误判，改用 /api/v1/workflows?limit=1 携带 API Key 探测。
func (s *HealthService) checkN8N() ServiceStatus {
	url := s.cfg.N8N.APIBaseURL + "/api/v1/workflows?limit=1"
	client := httpclient.HealthCheck(time.Duration(defaults.HealthCheckTimeout) * time.Second)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ServiceStatus{Status: "down", Error: err.Error()}
	}
	req.Header.Set("X-N8N-API-KEY", s.cfg.N8N.APIKey)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ServiceStatus{Status: "down", LatencyMs: latency, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return ServiceStatus{Status: "up", LatencyMs: latency}
	}

	return ServiceStatus{
		Status:    "unhealthy",
		LatencyMs: latency,
		Error:     fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

// checkRagFlow checks RagFlow health with API key authentication.
func (s *HealthService) checkRagFlow() ServiceStatus {
	url := s.cfg.RagFlow.BaseURL + "/api/v1/datasets?page=1&limit=1"
	client := httpclient.HealthCheck(time.Duration(defaults.HealthCheckTimeout) * time.Second)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ServiceStatus{Status: "down", Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.RagFlow.APIKey)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ServiceStatus{Status: "down", LatencyMs: latency, Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return ServiceStatus{Status: "up", LatencyMs: latency}
	}

	return ServiceStatus{
		Status:    "unhealthy",
		LatencyMs: latency,
		Error:     fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

func (s *HealthService) checkHTTPService(url string) ServiceStatus {
	client := httpclient.HealthCheck(time.Duration(defaults.HealthCheckTimeout) * time.Second)

	start := time.Now()
	resp, err := client.Get(url)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ServiceStatus{
			Status:    "down",
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return ServiceStatus{
			Status:    "up",
			LatencyMs: latency,
		}
	}

	return ServiceStatus{
		Status:    "unhealthy",
		LatencyMs: latency,
		Error:     fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

// checkRSSFetcher returns the status of the RSS fetcher service
func (s *HealthService) checkRSSFetcher() string {
	// If RSS fetcher config is not enabled, report as disabled
	if !s.cfg.RSSFetcher.Enabled {
		return "disabled"
	}
	// If enabled, report as up (the actual running status is tracked by the service itself)
	return "up"
}
