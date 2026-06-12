package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecideRecovery(t *testing.T) {
	cases := []struct {
		name   string
		total  int
		passed int
		want   recoveryDecision
	}{
		{"no paused feeds", 0, 0, recoverNone},
		{"all probes failed", 10, 0, recoverNone},
		{"majority passed resumes all", 10, 5, recoverAll},
		{"all passed resumes all", 4, 4, recoverAll},
		{"minority passed resumes partial", 10, 3, recoverPartial},
		{"single feed passed", 1, 1, recoverAll},
		{"single feed failed", 1, 0, recoverNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decideRecovery(c.total, c.passed); got != c.want {
				t.Errorf("decideRecovery(%d, %d) = %v, want %v", c.total, c.passed, got, c.want)
			}
		})
	}
}

func TestProbeURL(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failSrv.Close()

	svc := NewRSSFetcherService(RSSFetcherConfig{Timeout: 5}, nil)
	ctx := context.Background()

	if !svc.probeURL(ctx, okSrv.URL) {
		t.Errorf("probeURL(%s) = false, want true for 200 response", okSrv.URL)
	}
	if svc.probeURL(ctx, failSrv.URL) {
		t.Errorf("probeURL(%s) = true, want false for 503 response", failSrv.URL)
	}
	if svc.probeURL(ctx, "http://127.0.0.1:1/unreachable") {
		t.Error("probeURL(unreachable) = true, want false for connection error")
	}
}
