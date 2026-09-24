package balance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenCodeGoProvider_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"usage":{"rolling":{"status":"ok","percent":0,"resetsAt":"2026-09-24T15:18:54.930Z"},"weekly":{"status":"ok","percent":34,"resetsAt":"2026-09-28T00:00:00.000Z"},"monthly":{"status":"ok","percent":40,"resetsAt":"2026-10-03T09:30:18.000Z"}}}`))
	}))
	defer srv.Close()

	p, err := newOpenCodeGoProvider("opencode-go-secagent", srv.URL, "test-key", "")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	info, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// 最紧窗口 monthly 40% 已用 → 剩余 0.6
	if info.Balance < 0.59 || info.Balance > 0.61 {
		t.Errorf("expected remaining ratio ~0.6, got %v", info.Balance)
	}
	if info.TotalUsed < 0.39 || info.TotalUsed > 0.41 {
		t.Errorf("expected used ratio ~0.4, got %v", info.TotalUsed)
	}
	if info.ExpiresAt == nil {
		t.Error("expected earliest reset timestamp")
	}
	if info.Currency != "window_ratio" {
		t.Errorf("expected window_ratio currency marker, got %q", info.Currency)
	}
}

func TestOpenCodeGoProvider_ExhaustedWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"usage":{"rolling":{"status":"exhausted","percent":100,"resetsAt":"2026-09-24T20:00:00.000Z"},"weekly":{"status":"ok","percent":20,"resetsAt":"2026-09-28T00:00:00.000Z"},"monthly":{"status":"ok","percent":10,"resetsAt":"2026-10-03T00:00:00.000Z"}}}`))
	}))
	defer srv.Close()

	p, _ := newOpenCodeGoProvider("ch", srv.URL, "k", "")
	info, err := p.Fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if info.Balance != 0 {
		t.Errorf("rolling window exhausted → remaining must be 0, got %v", info.Balance)
	}
}
