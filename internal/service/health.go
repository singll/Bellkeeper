package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
)

type HealthService struct {
	cfg      *config.Config
	tagRepo  *repository.TagRepository
	dsRepo   *repository.DataSourceRepository
	rssRepo  *repository.RSSRepository
	dataRepo *repository.DatasetMappingRepository
}

func NewHealthService(
	cfg *config.Config,
	tagRepo *repository.TagRepository,
	dsRepo *repository.DataSourceRepository,
	rssRepo *repository.RSSRepository,
	dataRepo *repository.DatasetMappingRepository,
) *HealthService {
	return &HealthService{
		cfg:      cfg,
		tagRepo:  tagRepo,
		dsRepo:   dsRepo,
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
	Services map[string]ServiceStatus `json:"services"`
	Metrics  map[string]interface{}   `json:"metrics,omitempty"`
}

func (s *HealthService) Check() map[string]string {
	return map[string]string{
		"status":  "healthy",
		"version": "1.0.0",
	}
}

func (s *HealthService) Detailed() *DetailedHealth {
	services := make(map[string]ServiceStatus)

	// Check RagFlow
	services["ragflow"] = s.checkHTTPService(s.cfg.RagFlow.BaseURL + "/api/v1/version")

	// Check n8n
	services["n8n"] = s.checkHTTPService(s.cfg.N8N.WebhookBaseURL + "/healthz")

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

	// Count tags
	if s.tagRepo != nil {
		if tags, total, _ := s.tagRepo.List(1, 1, ""); total > 0 || len(tags) >= 0 {
			metrics["tags_count"] = total
		}
	}

	// Count active data sources
	if s.dsRepo != nil {
		if _, total, _ := s.dsRepo.List(1, 1, "", ""); total > 0 {
			metrics["datasources_count"] = total
		}
	}

	// Count active RSS feeds
	if s.rssRepo != nil {
		if _, total, _ := s.rssRepo.List(1, 1, "", ""); total > 0 {
			metrics["rss_feeds_count"] = total
		}
	}

	// Count dataset mappings
	if s.dataRepo != nil {
		if _, total, _ := s.dataRepo.List(1, 1); total > 0 {
			metrics["datasets_count"] = total
		}
	}

	return &DetailedHealth{
		Status:   overallStatus,
		Services: services,
		Metrics:  metrics,
	}
}

func (s *HealthService) checkHTTPService(url string) ServiceStatus {
	client := &http.Client{Timeout: 5 * time.Second}

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
