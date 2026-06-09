package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

// DailyReportWatchdog verifies that the n8n daily report produced its vault file.
type DailyReportWatchdog struct {
	cfg      config.DailyReportConfig
	basePath string
	notify   *NotificationService

	loc    *time.Location
	hour   int
	minute int

	cancel          context.CancelFunc
	done            chan struct{}
	mu              sync.Mutex
	running         bool
	lastCheckedDate string
}

func NewDailyReportWatchdog(cfg config.DailyReportConfig, basePath string, notify *NotificationService) (*DailyReportWatchdog, error) {
	cfg = normalizeDailyReportConfig(cfg)
	loc := time.Local
	if cfg.Timezone != "" {
		loaded, err := time.LoadLocation(cfg.Timezone)
		if err != nil {
			return nil, fmt.Errorf("load daily report timezone: %w", err)
		}
		loc = loaded
	}
	hour, minute, err := parseDailyReportClock(cfg.WatchdogTime)
	if err != nil {
		return nil, err
	}
	return &DailyReportWatchdog{
		cfg:      cfg,
		basePath: basePath,
		notify:   notify,
		loc:      loc,
		hour:     hour,
		minute:   minute,
		done:     make(chan struct{}),
	}, nil
}

func normalizeDailyReportConfig(cfg config.DailyReportConfig) config.DailyReportConfig {
	if cfg.Path == "" {
		cfg.Path = filepath.Join("vault", "daily")
	}
	if cfg.WatchdogTime == "" {
		cfg.WatchdogTime = "22:30"
	}
	if cfg.ReportChannel == "" {
		cfg.ReportChannel = "daily"
	}
	if cfg.AlertChannel == "" {
		cfg.AlertChannel = "alerts"
	}
	return cfg
}

func parseDailyReportClock(raw string) (int, int, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("daily_report.watchdog_time must be HH:MM")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("daily_report.watchdog_time has invalid hour: %s", raw)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("daily_report.watchdog_time has invalid minute: %s", raw)
	}
	return hour, minute, nil
}

func (w *DailyReportWatchdog) Start(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || !w.cfg.WatchdogEnabled {
		return
	}
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.running = true
	w.mu.Unlock()
	go w.loop(ctx)
	log.Printf("[DailyReportWatchdog] started")
}

func (w *DailyReportWatchdog) Stop() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	select {
	case <-w.done:
		log.Printf("[DailyReportWatchdog] stopped")
	case <-time.After(2 * time.Second):
		log.Printf("[DailyReportWatchdog] stop requested; waiting for current check to finish")
	}
}

func (w *DailyReportWatchdog) loop(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		if _, err := w.CheckOnce(ctx, time.Now()); err != nil {
			log.Printf("[DailyReportWatchdog] check failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *DailyReportWatchdog) CheckOnce(ctx context.Context, now time.Time) (bool, error) {
	if w == nil || !w.cfg.Enabled || !w.cfg.WatchdogEnabled {
		return false, nil
	}
	localNow := now.In(w.loc)
	checkAt := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), w.hour, w.minute, 0, 0, w.loc)
	if localNow.Before(checkAt) {
		return false, nil
	}

	date := localNow.Format("2006-01-02")
	w.mu.Lock()
	if w.lastCheckedDate == date {
		w.mu.Unlock()
		return false, nil
	}
	w.mu.Unlock()

	if w.reportExists(date) {
		w.markChecked(date)
		return false, nil
	}
	if w.notify == nil {
		return false, fmt.Errorf("daily report %s missing and notification service is not available", date)
	}

	id := "daily-report-missing-" + date
	if existing, err := w.notify.GetStatus(ctx, id); err != nil {
		log.Printf("[DailyReportWatchdog] read notification status failed: %v", err)
	} else if existing != nil {
		w.markChecked(date)
		return false, nil
	}

	msg := fmt.Sprintf("### 每日日报缺失告警\n\n- 日期: %s\n- 预期文件: `%s`\n- 检查时间: %s\n\nn8n 日报工作流可能未触发、已禁用或执行失败。请检查 O01/O02/K08 日报工作流和 n8n 执行历史。",
		date,
		filepath.ToSlash(filepath.Join(w.cfg.Path, date+".md")),
		localNow.Format(time.RFC3339),
	)
	resp, err := w.notify.Send(ctx, &NotificationRequest{
		Channel:     w.cfg.AlertChannel,
		Message:     msg,
		MessageType: "markdown",
		ID:          id,
		Metadata: map[string]string{
			"source": "daily_report_watchdog",
			"date":   date,
		},
	})
	if err != nil {
		return true, err
	}
	if resp != nil && !resp.Success {
		return true, fmt.Errorf("send daily report watchdog alert: %s", resp.Message)
	}
	w.markChecked(date)
	return true, nil
}

func (w *DailyReportWatchdog) reportExists(date string) bool {
	path := filepath.Join(w.basePath, w.cfg.Path, date+".md")
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func (w *DailyReportWatchdog) markChecked(date string) {
	w.mu.Lock()
	w.lastCheckedDate = date
	w.mu.Unlock()
}
