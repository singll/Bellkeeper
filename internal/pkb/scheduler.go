package pkb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"
	"gorm.io/gorm"
)

const (
	SettingAutoCurateEnabled = "feature_pkb_auto_curate"
	SettingAutoIntervalMins  = "pkb_auto_interval_minutes"

	// 骨架链维护任务开关 + 间隔（独立于主 curate；默认关，由前端/运维显式开启）。
	SettingAutoFillEnabled     = "feature_pkb_auto_fill"
	SettingAutoFeedEnabled     = "feature_pkb_auto_feed"
	SettingAutoProposeEnabled  = "feature_pkb_auto_propose"
	SettingFillIntervalMins    = "pkb_fill_interval_minutes"
	SettingFeedIntervalMins    = "pkb_feed_interval_minutes"
	SettingProposeIntervalMins = "pkb_propose_interval_minutes"

	defaultAutoInterval  = 6 * time.Hour
	defaultMaintInterval = 24 * time.Hour
	minAutoInterval      = 30 * time.Minute
	startupDelay         = time.Minute
	togglePollInterval   = time.Minute
	errorBackoff         = 30 * time.Minute
)

var errSchedulerBusy = errors.New("scheduler busy")

// Scheduler runs pkb-curate periodically when the DB feature switch is enabled.
type Scheduler struct {
	cfg         *config.Config
	configDir   string
	settingRepo *repository.SettingRepository
	articleRepo *repository.ArticleTagRepository
	llmJobs     *llmgateway.LLMJobQueueService
	activity    *service.ActivityLogService
	domainRepo  *repository.CrawlDomainProfileRepository

	cancel          context.CancelFunc
	done            chan struct{}
	mu              sync.Mutex
	running         bool
	manualQueue     []string // 手动「生成骨架」请求(域名)，loop 插队优先于自动任务，去重
	runningSkeleton string   // 当前正在生成骨架的域名(供前端显示「生成中」)，空=无
}

func NewScheduler(
	cfg *config.Config,
	configDir string,
	settingRepo *repository.SettingRepository,
	articleRepo *repository.ArticleTagRepository,
	llmJobs *llmgateway.LLMJobQueueService,
	activity *service.ActivityLogService,
	domainRepo *repository.CrawlDomainProfileRepository,
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
		domainRepo:  domainRepo,
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

	tasks := s.buildTasks()
	startAt := time.Now().Add(startupDelay)
	for _, t := range tasks {
		t.nextRun = startAt
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if s.isRunning() {
			continue // 有任务在跑，本 tick 让出（手动/自动都等当前任务完成）
		}
		now := time.Now()
		// 手动「生成骨架」插队：优先于所有自动任务，避免被慢任务(fill)饿死。
		if dom := s.popManual(); dom != "" {
			start := time.Now()
			summary, err := s.runGuarded(ctx, func(ctx context.Context) (string, error) {
				return s.runSkeletonDomain(ctx, dom)
			})
			durationMs := int(time.Since(start).Milliseconds())
			if err != nil {
				log.Printf("[PKBScheduler] 手动 skeleton(%s) 失败: %v", dom, err)
				s.logActivity("failed", fmt.Sprintf("PKB 骨架生成失败(%s): %v", dom, err), durationMs)
			} else {
				s.logActivity("success", summary, durationMs)
				log.Printf("[PKBScheduler] 手动 skeleton(%s) 完成", dom)
			}
			s.mu.Lock()
			s.runningSkeleton = ""
			s.mu.Unlock()
			continue
		}
		for _, t := range tasks {
			if !t.enabled() || now.Before(t.nextRun) {
				continue
			}
			start := time.Now()
			summary, err := s.runGuarded(ctx, t.run)
			if errors.Is(err, errSchedulerBusy) {
				break // 已有任务在跑，本 tick 让出，下个 tick 重试
			}
			durationMs := int(time.Since(start).Milliseconds())
			if err != nil {
				log.Printf("[PKBScheduler] %s 失败: %v", t.name, err)
				s.logActivity("failed", fmt.Sprintf("PKB %s 失败: %v", t.name, err), durationMs)
				t.nextRun = now.Add(errorBackoff)
			} else {
				if summary != "" {
					s.logActivity("success", summary, durationMs)
				}
				t.nextRun = now.Add(t.interval())
				log.Printf("[PKBScheduler] %s 完成，下次 %s", t.name, t.nextRun.Format(time.RFC3339))
			}
			break // 一个 tick 只跑一个到期任务（串行 + 公平轮转）
		}
	}
}

// schedTask 是调度器的一个周期维护任务。
type schedTask struct {
	name     string
	enabled  func() bool
	interval func() time.Duration
	run      func(context.Context) (string, error)
	nextRun  time.Time
}

// buildTasks 构建维护任务列表：主 curate+digest（新卡归位+综述）+ fill/feed/propose（独立开关周期）。
func (s *Scheduler) buildTasks() []*schedTask {
	return []*schedTask{
		{name: "curate+digest", enabled: s.autoEnabled, interval: s.interval, run: s.runCurateDigest},
		{name: "fill", enabled: func() bool { return s.boolSetting(SettingAutoFillEnabled, false) }, interval: func() time.Duration { return s.intervalSetting(SettingFillIntervalMins, defaultMaintInterval) }, run: s.runFill},
		{name: "feed", enabled: func() bool { return s.boolSetting(SettingAutoFeedEnabled, false) }, interval: func() time.Duration { return s.intervalSetting(SettingFeedIntervalMins, defaultMaintInterval) }, run: s.runFeed},
		{name: "propose", enabled: func() bool { return s.boolSetting(SettingAutoProposeEnabled, false) }, interval: func() time.Duration { return s.intervalSetting(SettingProposeIntervalMins, defaultMaintInterval) }, run: s.runPropose},
	}
}

// runGuarded 串行执行一个任务：若已有任务在跑则返回 errSchedulerBusy（本 tick 让出，避免并发写 vault/抢 LLM 队列）。
func (s *Scheduler) runGuarded(ctx context.Context, fn func(context.Context) (string, error)) (string, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return "", errSchedulerBusy
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
		return "", ctx.Err()
	default:
	}
	return fn(ctx)
}

func (s *Scheduler) newCurator(ctx context.Context) (*Curator, error) {
	return NewCurator(s.cfg, Options{
		ConfigDir:  s.configDir,
		LLMJobs:    s.llmJobs,
		Context:    ctx,
		DomainRepo: s.domainRepo, // fill 缺口填充 G3 冷却让路用
	}, s.articleRepo)
}

// runCurateDigest 主维护任务：curate（打分/重构落卡）→ digest 全域（渲染骨架 + 新卡确定性归位
// + 综述）。digest 内含 placeCardsOntoSkeleton，故跑完即「新卡挂骨架」。digest 失败不致命
// （curate 已落卡），仅记录。
func (s *Scheduler) runCurateDigest(ctx context.Context) (string, error) {
	c, err := s.newCurator(ctx)
	if err != nil {
		return "", err
	}
	if err := c.Run(); err != nil {
		return "", err
	}
	summary := formatRunSummary("PKB 自动维护完成", c.lastSummary)
	if err := c.RunDigest(DigestOptions{}); err != nil {
		log.Printf("[PKBScheduler] curate 后 digest 归位失败: %v", err)
		summary += "（digest 归位失败，详见日志）"
	}
	return summary, nil
}

// runFill 缺口填充：遍历开启了 gap_fill 的知识域（跳过 feed/兜底/无 scope），逐域补缺口。
func (s *Scheduler) runFill(ctx context.Context) (string, error) {
	doms, err := LoadDomains(filepath.Join(s.configDir, "domains.yaml"))
	if err != nil {
		return "", err
	}
	c, err := s.newCurator(ctx)
	if err != nil {
		return "", err
	}
	n := 0
	for _, d := range doms.Domains {
		if d.Feed || d.IsDefault || strings.TrimSpace(d.Scope) == "" {
			continue
		}
		if !doms.Defaults.GapFillEnabledFor(d.Name) {
			continue
		}
		// 小批量(每轮每域 3 缺口)：fill 单缺口慢(~10min)，限小让单轮快速完成、及时让出串行锁，
		// 避免长期阻塞手动「生成骨架」插队；缺口靠每日多轮渐进填满。
		if err := c.RunGapFill(GapFillOptions{Domain: d.Name, PerRun: 3}); err != nil {
			log.Printf("[PKBScheduler] fill %s: %v", d.Name, err)
			continue
		}
		n++
	}
	return fmt.Sprintf("PKB 自动缺口填充完成（%d 个领域）", n), nil
}

// runFeed 资讯库：全域生成当日资讯存档 + 晋升闸（耐久知识点→知识库卡）。
func (s *Scheduler) runFeed(ctx context.Context) (string, error) {
	c, err := s.newCurator(ctx)
	if err != nil {
		return "", err
	}
	if err := c.RunFeed(FeedOptions{}); err != nil {
		return "", err
	}
	return "PKB 自动资讯库生成完成", nil
}

// runPropose 骨架结构增长：遍历知识域，从待归位簇生成骨架变更提议（影响半径闸——小动作自动应用、
// 大动作落待批提议供前端审批）。
func (s *Scheduler) runPropose(ctx context.Context) (string, error) {
	doms, err := LoadDomains(filepath.Join(s.configDir, "domains.yaml"))
	if err != nil {
		return "", err
	}
	c, err := s.newCurator(ctx)
	if err != nil {
		return "", err
	}
	n := 0
	for _, d := range doms.Domains {
		if d.Feed || d.IsDefault || strings.TrimSpace(d.Scope) == "" {
			continue
		}
		if err := c.RunPropose(ProposeOptions{Domain: d.Name}); err != nil {
			log.Printf("[PKBScheduler] propose %s: %v", d.Name, err)
			continue
		}
		n++
	}
	return fmt.Sprintf("PKB 自动骨架提议完成（%d 个领域）", n), nil
}

// TriggerSkeleton 把手动「生成骨架」请求入队（去重）；loop 会在当前任务完成后**优先**执行
// （插队，先于所有自动任务），故不会被慢任务(fill)饿死。立即返回，结果经 domains/stats
// 的 has_skeleton 反映。
func (s *Scheduler) TriggerSkeleton(domain string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runningSkeleton == domain {
		return nil // 正在生成该域，去重
	}
	for _, d := range s.manualQueue {
		if d == domain {
			return nil // 已在队列，去重（重复点击不堆积）
		}
	}
	s.manualQueue = append(s.manualQueue, domain)
	return nil
}

// isRunning 当前是否有 PKB 任务在执行（持锁）。
func (s *Scheduler) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// popManual 取出一个待处理的手动 skeleton 请求（FIFO），无则返回空串；取出即标记为「生成中」。
func (s *Scheduler) popManual() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.manualQueue) == 0 {
		return ""
	}
	d := s.manualQueue[0]
	s.manualQueue = s.manualQueue[1:]
	s.runningSkeleton = d
	return d
}

// SkeletonStatus 返回正在排队的域名列表 + 当前正在生成的域名（供前端显示「排队中/生成中」）。
func (s *Scheduler) SkeletonStatus() (queued []string, running string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queued = append([]string{}, s.manualQueue...)
	return queued, s.runningSkeleton
}

// runSkeletonDomain 为单个领域生成骨架（手动插队任务体）。
func (s *Scheduler) runSkeletonDomain(ctx context.Context, domain string) (string, error) {
	c, err := s.newCurator(ctx)
	if err != nil {
		return "", err
	}
	if err := c.RunSkeleton(SkeletonOptions{Domain: domain}); err != nil {
		return "", err
	}
	return fmt.Sprintf("PKB 骨架生成完成（%s）", domain), nil
}

// boolSetting 读布尔型 setting（record not found → def）。
func (s *Scheduler) boolSetting(key string, def bool) bool {
	setting, err := s.settingRepo.GetByKey(key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return def
		}
		log.Printf("[PKBScheduler] read %s failed: %v", key, err)
		return false
	}
	return parseBoolSetting(setting.Value, def)
}

// intervalSetting 读分钟数型 setting → Duration（缺省/非法 → def，下限 minAutoInterval）。
func (s *Scheduler) intervalSetting(key string, def time.Duration) time.Duration {
	setting, err := s.settingRepo.GetByKey(key)
	if err != nil {
		return def
	}
	mins, err := strconv.Atoi(strings.TrimSpace(setting.Value))
	if err != nil || mins <= 0 {
		return def
	}
	if d := time.Duration(mins) * time.Minute; d >= minAutoInterval {
		return d
	}
	return minAutoInterval
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
