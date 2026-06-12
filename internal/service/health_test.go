package service

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/singll/bellkeeper/internal/config"
)

func TestHealthService_Detailed_ConfigProbes(t *testing.T) {
	svc := NewHealthService(&config.Config{}, "test", nil, nil, nil)

	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer healthySrv.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close() //nolint:errcheck
		}
	}()
	tcpAddr := ln.Addr().String()

	svc.cfg = &config.Config{
		Health: config.HealthConfig{
			Services: []config.ServiceProbe{
				{Name: "test_http", URL: healthySrv.URL, Type: "http"},
				{Name: "test_tcp", URL: tcpAddr, Type: "tcp"},
				{Name: "rss_fetcher", Type: "internal"},
			},
		},
		RSSFetcher: config.RSSFetcherConfig{Enabled: true},
	}

	result := svc.Detailed()
	if result.Status != "healthy" {
		t.Errorf("overall status = %q, want healthy", result.Status)
	}
	if result.Services["test_http"].Status != "up" {
		t.Errorf("test_http status = %q, want up", result.Services["test_http"].Status)
	}
	if result.Services["test_tcp"].Status != "up" {
		t.Errorf("test_tcp status = %q, want up", result.Services["test_tcp"].Status)
	}
	if result.Services["rss_fetcher"].Status != "up" {
		t.Errorf("rss_fetcher status = %q, want up", result.Services["rss_fetcher"].Status)
	}
}

func TestHealthService_Detailed_DownService(t *testing.T) {
	svc := NewHealthService(&config.Config{}, "test", nil, nil, nil)

	svc.cfg = &config.Config{
		Health: config.HealthConfig{
			Services: []config.ServiceProbe{
				{Name: "broken", URL: "http://127.0.0.1:1", Type: "http", Timeout: 1},
			},
		},
	}

	result := svc.Detailed()
	if result.Status != "degraded" {
		t.Errorf("overall status = %q, want degraded", result.Status)
	}
	if result.Services["broken"].Status != "down" {
		t.Errorf("broken status = %q, want down", result.Services["broken"].Status)
	}
}

func TestHealthService_Detailed_FallbackWhenNoProbes(t *testing.T) {
	svc := NewHealthService(&config.Config{}, "test", nil, nil, nil)
	svc.cfg = &config.Config{
		RSSFetcher: config.RSSFetcherConfig{Enabled: true},
	}

	result := svc.Detailed()
	if _, ok := result.Services["rss_fetcher"]; !ok {
		t.Error("expected rss_fetcher in fallback mode")
	}
}

func TestHealthService_checkTCPService(t *testing.T) {
	svc := NewHealthService(&config.Config{}, "test", nil, nil, nil)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close() //nolint:errcheck
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close() //nolint:errcheck
		}
	}()

	status := svc.checkTCPService(ln.Addr().String(), 2)
	if status.Status != "up" {
		t.Errorf("status = %q, want up", status.Status)
	}

	status = svc.checkTCPService("127.0.0.1:1", 1)
	if status.Status != "down" {
		t.Errorf("status = %q, want down for unreachable", status.Status)
	}
}
