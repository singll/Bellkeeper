package llmgateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmgateway/balance"
	llmerrors "github.com/singll/bellkeeper/internal/llmgateway/errors"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// --- LLM Proxy Service ---

// Repos bundles the repositories used by the runtime proxy. SettingRepository is
// passed separately because it is only consumed for the coding-strategy setting.
type Repos struct {
	Proxy               *repository.LLMProxyRepository
	Channel             *repository.LLMChannelRepository
	Group               *repository.LLMModelGroupRepository
	TokenUsage          *repository.LLMTokenUsageRepository
	RateLimit           *repository.LLMRateLimitRepository
	ConversationBinding *repository.ConversationBindingRepository
	Token               *repository.LLMTokenRepository
	Credential          *repository.LLMChannelCredentialRepository
	BalanceSnapshot     *repository.LLMChannelBalanceSnapshotRepository
}

// Options is the full dependency set for the LLM proxy runtime. Grouping them
// here keeps the constructor stable as more read-only dependencies are added.
type Options struct {
	Config      config.LLMProxyConfig
	Repos       Repos
	Pricer      *Pricer
	SettingRepo *repository.SettingRepository
}

// LLMProxyService provides a rate-limited, multi-channel OpenAI-compatible proxy
// with virtual model groups, sticky routing, and circuit breaker support.
type LLMProxyService struct {
	mu                  sync.RWMutex
	startMu             sync.Mutex
	started             bool
	cfg                 config.LLMProxyConfig
	channels            map[string]*Channel    // name -> channel
	modelMap            map[string][]*Channel  // model -> sorted channels (by priority)
	modelGroups         map[string]*ModelGroup // group name -> model group
	repo                *repository.LLMProxyRepository
	channelRepo         *repository.LLMChannelRepository
	groupRepo           *repository.LLMModelGroupRepository
	pricer              *Pricer
	tokenUsageRepo      *repository.LLMTokenUsageRepository
	balanceMgr          *balance.Manager
	rateLimitLearner    *RateLimitLearner
	taskRouter          *TaskRouter
	convMgr             *ConversationBindingManager
	tokenRepo           *repository.LLMTokenRepository
	credentialRepo      *repository.LLMChannelCredentialRepository
	balanceSnapshotRepo *repository.LLMChannelBalanceSnapshotRepository
	settingRepo         *repository.SettingRepository
	alertAggregator     *AlertAggregator
	groupStopChans      []chan struct{} // per-load group-cleanup goroutines (recreated on reload)
	bgStopChans         []chan struct{} // long-lived background loops (conv cleanup, archival, quota)
	logQueue            chan proxyLogEntry
	logStopCh           chan struct{}
	logWG               sync.WaitGroup
}

const (
	llmProxyLogQueueSize = 4096
	llmProxyLogWorkers   = 2

	// SettingCodingRoutingStrategy persists the UI-selected coding strategy so it
	// survives restarts instead of silently reverting to the YAML default.
	SettingCodingRoutingStrategy = "llm_coding_routing_strategy"
)

func NewLLMProxyService(opts Options) *LLMProxyService {
	cfg := opts.Config
	svc := &LLMProxyService{
		cfg:                 cfg,
		channels:            make(map[string]*Channel),
		modelMap:            make(map[string][]*Channel),
		modelGroups:         make(map[string]*ModelGroup),
		repo:                opts.Repos.Proxy,
		channelRepo:         opts.Repos.Channel,
		groupRepo:           opts.Repos.Group,
		pricer:              opts.Pricer,
		tokenUsageRepo:      opts.Repos.TokenUsage,
		tokenRepo:           opts.Repos.Token,
		credentialRepo:      opts.Repos.Credential,
		balanceSnapshotRepo: opts.Repos.BalanceSnapshot,
		settingRepo:         opts.SettingRepo,
		logQueue:            make(chan proxyLogEntry, llmProxyLogQueueSize),
		logStopCh:           make(chan struct{}),
	}

	// Initialize the rate-limit learner BEFORE loading channels so learned-safe RPM
	// is available to size token buckets at first load (not just after a reload).
	svc.rateLimitLearner = NewRateLimitLearner(opts.Repos.RateLimit)
	if err := svc.rateLimitLearner.LoadCache(); err != nil {
		middleware.GetLogger().Warn("llm proxy: load rate limit cache failed", zap.Error(err))
	}

	if err := svc.loadFromDB(false); err != nil {
		middleware.GetLogger().Error("failed to load llm-proxy config from DB", zap.Error(err))
	}

	// Initialize balance manager
	if cfg.BalanceSyncInterval > 0 {
		svc.balanceMgr = balance.NewManager(time.Duration(cfg.BalanceSyncInterval) * time.Second)
		svc.registerBalanceProviders()
	}

	// Initialize task router (persisted UI choice overrides the YAML default).
	svc.taskRouter = NewTaskRouter(svc.loadedCodingStrategy(cfg.CodingRoutingStrategy), cfg.Complexity)

	// Alert aggregator (nil notifier until Matrix infra wires one in via
	// SetAlertNotifier — until then it still buffers + persists events to DB).
	svc.alertAggregator = NewAlertAggregator(opts.Repos.Proxy, nil)

	// Initialize conversation binding manager
	svc.convMgr = NewConversationBindingManager(opts.Repos.ConversationBinding, 24*time.Hour)
	_ = svc.convMgr.LoadFromDB()

	return svc
}

func (s *LLMProxyService) Start(ctx context.Context) error {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if s.started {
		return nil
	}
	s.started = true

	if s.rateLimitLearner != nil {
		s.rateLimitLearner.Start()
	}
	if s.balanceMgr != nil {
		s.balanceMgr.Start()
	}
	if s.alertAggregator != nil {
		s.alertAggregator.Start()
	}
	s.startGroupCleanupLoops()
	s.startBackgroundLoops()
	s.startLogWorkers()
	return nil
}

func (s *LLMProxyService) registerBalanceProviders() {
	if s.balanceMgr == nil {
		return
	}
	for name, ch := range s.channels {
		if ch.Config.BalanceProviderType == "" {
			continue
		}
		apiKey := ""
		if s.credentialRepo != nil {
			if resolved, err := resolveCredentialForChannel(s.credentialRepo, ch.Config.ID, "balance"); err == nil && resolved != "" {
				apiKey = resolved
			}
			if apiKey == "" {
				if resolved, err := resolveCredentialForChannel(s.credentialRepo, ch.Config.ID, "api"); err == nil && resolved != "" {
					apiKey = resolved
				}
			}
		}
		if apiKey == "" {
			apiKey = ch.Config.APIKey
		}
		if apiKey == "" {
			continue
		}
		if err := s.balanceMgr.Register(name, ch.Config.BalanceProviderType, ch.Config.BaseURL, apiKey, ch.Config.BalanceConfigJSON); err != nil {
			middleware.GetLogger().Warn("failed to register balance provider",
				zap.String("channel", name), zap.Error(err))
		}
	}
}

// GetChannelBalance returns the last fetched balance for a channel.
func (s *LLMProxyService) GetChannelBalance(channelName string) *balance.Info {
	if s.balanceMgr == nil {
		return nil
	}
	return s.balanceMgr.Get(channelName)
}

// GetAllBalances returns all channel balances.
func (s *LLMProxyService) GetAllBalances() map[string]*balance.Info {
	if s.balanceMgr == nil {
		return nil
	}
	return s.balanceMgr.GetAll()
}

// RefreshBalances triggers an immediate balance refresh.
func (s *LLMProxyService) RefreshBalances() {
	if s.balanceMgr == nil {
		return
	}
	s.balanceMgr.RefreshAll()
}

// Stop halts background services (balance manager, learner, aggregator, loops).
func (s *LLMProxyService) Stop() {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if !s.started {
		return
	}
	s.started = false

	s.mu.Lock()
	for _, stop := range s.groupStopChans {
		close(stop)
	}
	s.groupStopChans = nil
	s.mu.Unlock()
	for _, stop := range s.bgStopChans {
		close(stop)
	}
	s.bgStopChans = nil
	if s.alertAggregator != nil {
		s.alertAggregator.Stop()
	}
	if s.balanceMgr != nil {
		s.balanceMgr.Stop()
	}
	if s.rateLimitLearner != nil {
		s.rateLimitLearner.Stop()
	}
	close(s.logStopCh)
	s.logWG.Wait()
	s.logStopCh = make(chan struct{})
}

// SetAlertNotifier wires the delivery notifier into the alert aggregator. Called
// from app setup once the Matrix notification service is available.
func (s *LLMProxyService) SetAlertNotifier(n AlertNotifier) {
	if s.alertAggregator != nil {
		s.alertAggregator.SetNotifier(n)
	}
}

// recordAlert buffers an alert event (no-op if the aggregator isn't initialized).
func (s *LLMProxyService) recordAlert(alertType, severity, channel, msg string) {
	if s.alertAggregator != nil {
		s.alertAggregator.Record(alertType, severity, channel, msg)
	}
}

// alertForClass maps a semantic upstream error class to an aggregated alert so
// operators learn about quota/balance/session/auth failures (§2.6.6).
func (s *LLMProxyService) alertForClass(channel string, class llmerrors.Class) {
	switch class {
	case llmerrors.QuotaExhausted:
		s.recordAlert("quota_exhausted", "error", channel, fmt.Sprintf("channel %s quota exhausted", channel))
	case llmerrors.BalanceZero:
		s.recordAlert("balance_zero", "error", channel, fmt.Sprintf("channel %s balance depleted — please top up", channel))
	case llmerrors.SessionExpired:
		s.recordAlert("session_expired", "error", channel, fmt.Sprintf("channel %s session expired — re-import credentials", channel))
	case llmerrors.AuthFailed:
		s.recordAlert("auth_failed", "critical", channel, fmt.Sprintf("channel %s auth failed — API key invalid", channel))
	case llmerrors.SubscriptionInvalid:
		s.recordAlert("subscription_invalid", "error", channel, fmt.Sprintf("channel %s subscription validation failed", channel))
	}
}
