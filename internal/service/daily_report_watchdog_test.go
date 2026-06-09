package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

func TestDailyReportWatchdogSkipsBeforeWatchdogTime(t *testing.T) {
	w, err := NewDailyReportWatchdog(config.DailyReportConfig{
		Enabled:         true,
		WatchdogEnabled: true,
		WatchdogTime:    "22:30",
		Timezone:        "UTC",
		Path:            "vault/daily",
	}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewDailyReportWatchdog returned error: %v", err)
	}
	alerted, err := w.CheckOnce(context.Background(), time.Date(2026, 6, 9, 22, 29, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CheckOnce returned error: %v", err)
	}
	if alerted {
		t.Fatal("CheckOnce alerted before watchdog time")
	}
}

func TestDailyReportWatchdogAcceptsExistingReport(t *testing.T) {
	base := t.TempDir()
	reportPath := filepath.Join(base, "vault", "daily", "2026-06-09.md")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	w, err := NewDailyReportWatchdog(config.DailyReportConfig{
		Enabled:         true,
		WatchdogEnabled: true,
		WatchdogTime:    "22:30",
		Timezone:        "UTC",
		Path:            "vault/daily",
	}, base, nil)
	if err != nil {
		t.Fatalf("NewDailyReportWatchdog returned error: %v", err)
	}
	alerted, err := w.CheckOnce(context.Background(), time.Date(2026, 6, 9, 22, 31, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CheckOnce returned error: %v", err)
	}
	if alerted {
		t.Fatal("CheckOnce alerted even though report exists")
	}
}

func TestDailyReportWatchdogReportsMissingWithoutNotifier(t *testing.T) {
	w, err := NewDailyReportWatchdog(config.DailyReportConfig{
		Enabled:         true,
		WatchdogEnabled: true,
		WatchdogTime:    "22:30",
		Timezone:        "UTC",
		Path:            "vault/daily",
	}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewDailyReportWatchdog returned error: %v", err)
	}
	_, err = w.CheckOnce(context.Background(), time.Date(2026, 6, 9, 22, 31, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "notification service is not available") {
		t.Fatalf("CheckOnce err = %v, want missing notifier error", err)
	}
}
