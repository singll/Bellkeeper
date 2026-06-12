package service

import (
	"fmt"
	"net"
	"net/http"
	"sync"
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

	probes := s.cfg.Health.Services
	if len(probes) == 0 {
		services["n8n"] = s.checkN8N()
		if s.cfg.Meilisearch.URL != "" {
			services["meilisearch"] = s.checkHTTPService(s.cfg.Meilisearch.URL + "/health")
		}
		services["rss_fetcher"] = ServiceStatus{Status: s.checkRSSFetcher()}
	} else {
		sem := make(chan struct{}, 4)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, probe := range probes {
			wg.Add(1)
			go func(p config.ServiceProbe) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				var status ServiceStatus
				switch p.Type {
				case "n8n_api":
					if s.cfg.N8N.APIBaseURL != "" && s.cfg.N8N.APIKey != "" {
						status = s.checkN8N()
					} else {
						status = ServiceStatus{Status: "disabled", Error: "n8n not configured"}
					}
				case "http":
					if p.URL != "" {
						status = s.checkHTTPServiceWithTimeout(p.URL, p.Timeout)
					} else {
						status = ServiceStatus{Status: "disabled", Error: "no URL configured"}
					}
				case "tcp":
					if p.URL != "" {
						status = s.checkTCPService(p.URL, p.Timeout)
					} else {
						status = ServiceStatus{Status: "disabled", Error: "no address configured"}
					}
				case "internal":
					if p.Name == "rss_fetcher" {
						status = ServiceStatus{Status: s.checkRSSFetcher()}
					} else {
						status = ServiceStatus{Status: "up"}
					}
				default:
					if p.URL != "" {
						status = s.checkHTTPServiceWithTimeout(p.URL, p.Timeout)
					} else {
						status = ServiceStatus{Status: "disabled"}
					}
				}

				mu.Lock()
				services[p.Name] = status
				mu.Unlock()
			}(probe)
		}
		wg.Wait()
	}

	overallStatus := "healthy"
	for _, svc := range services {
		if svc.Status != "up" && svc.Status != "disabled" {
			overallStatus = "degraded"
			break
		}
	}

	metrics := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if s.tagRepo != nil {
		if tags, total, _ := s.tagRepo.List(1, 1, ""); total > 0 || len(tags) >= 0 {
			metrics["tags_count"] = total
		}
	}

	if s.rssRepo != nil {
		if _, total, _ := s.rssRepo.List(1, 1, "", "", nil); total > 0 {
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

func (s *HealthService) checkN8N() ServiceStatus {
	url := s.cfg.N8N.APIBaseURL + "/workflows?limit=1"
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
	defer resp.Body.Close() //nolint:errcheck

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
	return s.checkHTTPServiceWithTimeout(url, 0)
}

func (s *HealthService) checkHTTPServiceWithTimeout(url string, timeoutSec int) ServiceStatus {
	timeout := defaults.HealthCheckTimeout
	if timeoutSec > 0 {
		timeout = timeoutSec
	}
	client := httpclient.HealthCheck(time.Duration(timeout) * time.Second)

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
	defer resp.Body.Close() //nolint:errcheck

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

func (s *HealthService) checkTCPService(addr string, timeoutSec int) ServiceStatus {
	timeout := defaults.HealthCheckTimeout
	if timeoutSec > 0 {
		timeout = timeoutSec
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeout)*time.Second)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return ServiceStatus{
			Status:    "down",
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}
	_ = conn.Close()
	return ServiceStatus{
		Status:    "up",
		LatencyMs: latency,
	}
}

func (s *HealthService) checkRSSFetcher() string {
	if !s.cfg.RSSFetcher.Enabled {
		return "disabled"
	}
	return "up"
}
