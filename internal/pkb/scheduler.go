package pkb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"
	"gorm.io/gorm"
)

const (
	SettingAutoCurateEnabled = "feature_pkb_auto_curate"
	SettingAutoIntervalMins  = "pkb_auto_interval_minutes"

	defaultAutoInterval = 6 * time.Hour
	minAutoInterval     = 30 * time.Minute
	startupDelay        = time.Minute
	togglePollInterval  = time.Minute
	errorBackoff        = 30 * time.Minute
)

// Scheduler runs pkb-curate periodically when the DB feature switch is enabled.
type Scheduler struct {
	cfg         *config.Config
	configDir   string
	settingRepo *repository.SettingRepository
	articleRepo *repository.ArticleTagRepository
	llmJobs     *service.LLMJobQueueService
	activity    *service.ActivityLogService

	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	running bool
}

func NewScheduler(
	cfg *config.Config,
	configDir string,
	settingRepo *repository.SettingRepository,
	articleRepo *repository.ArticleTagRepository,
	llmJobs *service.LLMJobQueueService,
	activity *service.ActivityLogService,
) *Scheduler {
	if configDir == "" {
		configDir = "config/pkb"
	}
	return &Scheduler{
		cfg:         cfg,
		configDir:   configDir,
		settingRepo: settingRepo,
		articleRepo: articleRepo,
		llmJobs:     llmJobs,
		activity:    activity,
		done:        make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.settingRepo == nil || s.articleRepo == nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	go s.loop(ctx)
	log.Printf("[PKBScheduler] started")
}

func (s *Scheduler) Stop() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
	select {
	case <-s.done:
		log.Printf("[PKBScheduler] stopped")
	case <-time.After(2 * time.Second):
		log.Printf("[PKBScheduler] stop requested; current run will exit after its active operation")
	}
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(togglePollInterval)
	defer ticker.Stop()

	nextRun := time.Now().Add(startupDelay)
	wasEnabled := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		enabled := s.autoEnabled()
		if !enabled {
			if wasEnabled {
				log.Printf("[PKBScheduler] disabled")
			}
			wasEnabled = false
			nextRun = time.Now()
			continue
		}

		if !wasEnabled {
			log.Printf("[PKBScheduler] enabled")
			wasEnabled = true
			nextRun = time.Now()
		}
		if time.Now().Before(nextRun) {
			continue
		}

		start := time.Now()
		sum, err := s.runOnce(ctx)
		durationMs := int(time.Since(start).Milliseconds())
		if err != nil {
			log.Printf("[PKBScheduler] run failed: %v", err)
			s.logActivity("failed", fmt.Sprintf("PKB 自动维护失败: %v", err), durationMs)
			nextRun = time.Now().Add(errorBackoff)
			continue
		}

		s.logActivity("success", formatRunSummary("PKB 自动维护完成", sum), durationMs)
		nextRun = time.Now().Add(s.interval())
		log.Printf("[PKBScheduler] next run at %s", nextRun.Format(time.RFC3339))
	}
}

func (s *Scheduler) runOnce(ctx context.Context) (runSummary, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return runSummary{}, nil
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return runSummary{}, ctx.Err()
	default:
	}

	curator, err := NewCurator(s.cfg, Options{
		ConfigDir: s.configDir,
		LLMJobs:   s.llmJobs,
		Context:   ctx,
	}, s.articleRepo)
	if err != nil {
		return runSummary{}, err
	}
	err = curator.Run()
	return curator.lastSummary, err
}

func (s *Scheduler) autoEnabled() bool {
	setting, err := s.settingRepo.GetByKey(SettingAutoCurateEnabled)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true
		}
		log.Printf("[PKBScheduler] read switch failed: %v", err)
		return false
	}
	return parseBoolSetting(setting.Value, true)
}

func (s *Scheduler) interval() time.Duration {
	setting, err := s.settingRepo.GetByKey(SettingAutoIntervalMins)
	if err != nil {
		return defaultAutoInterval
	}
	mins, err := strconv.Atoi(strings.TrimSpace(setting.Value))
	if err != nil || mins <= 0 {
		return defaultAutoInterval
	}
	d := time.Duration(mins) * time.Minute
	if d < minAutoInterval {
		return minAutoInterval
	}
	return d
}

func (s *Scheduler) logActivity(status, summary string, durationMs int) {
	if s.activity == nil {
		return
	}
	s.activity.LogActivity(service.LogActivityParams{
		Module:     "pkb",
		Action:     "auto_curate",
		Status:     status,
		Summary:    summary,
		DurationMs: durationMs,
	})
}

func formatRunSummary(prefix string, sum runSummary) string {
	return fmt.Sprintf("%s：处理 %d / vault %d / archive %d / discard %d / 失败 %d / 延期 %d",
		prefix, sum.processed, sum.vault, sum.archive, sum.discard, sum.failed, sum.deferred)
}

func parseBoolSetting(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on", "enabled", "enable", "启用", "开启":
		return true
	case "false", "0", "no", "off", "disabled", "disable", "禁用", "关闭":
		return false
	default:
		return fallback
	}
}
