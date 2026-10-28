package llmgateway

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/envutil"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// validateBinding checks if a conversation binding is still valid.
func (s *LLMProxyService) validateBinding(binding *model.LLMConversationBinding) error {
	if time.Now().After(binding.ExpiresAt) {
		return fmt.Errorf("binding expired")
	}
	s.mu.RLock()
	ch, ok := s.channels[binding.ChannelName]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("bound channel %s no longer exists", binding.ChannelName)
	}
	if !ch.Health.IsAvailable() {
		return fmt.Errorf("bound channel %s is unavailable", binding.ChannelName)
	}
	return nil
}

// loadFromDB loads channel and model group configuration from the database.
func (s *LLMProxyService) loadFromDB(startCleanupLoops bool) error {
	dbChannels, err := s.channelRepo.ListEnabled()
	if err != nil {
		return fmt.Errorf("load channels: %w", err)
	}

	channels := make(map[string]*Channel)
	modelMap := make(map[string][]*Channel)

	for _, dbCh := range dbChannels {
		chCfg := s.dbChannelToConfig(dbCh)
		ch := &Channel{
			Config: chCfg,
			Bucket: NewTokenBucket(s.effectiveBucketRPM(chCfg), chCfg.RPD, s.cfg.DefaultBucketRPM,
				time.Duration(chCfg.QuotaWindowSeconds)*time.Second),
			Client: httpclient.NewClientWithTimeout(time.Duration(s.cfg.DefaultTimeout) * time.Second),
			Health: NewChannelHealth(s.cfg.CircuitBreaker),
		}
		channels[chCfg.Name] = ch

		for _, m := range chCfg.Models {
			modelMap[m] = append(modelMap[m], ch)
		}
	}

	// Sort channels per model by priority (lower = higher priority)
	for m := range modelMap {
		chs := modelMap[m]
		sort.Slice(chs, func(i, j int) bool {
			return chs[i].Config.Priority < chs[j].Config.Priority
		})
	}

	// Load model groups
	dbGroups, err := s.groupRepo.List()
	if err != nil {
		return fmt.Errorf("load groups: %w", err)
	}

	modelGroups := make(map[string]*ModelGroup)
	var stopChans []chan struct{}

	for _, dbGroup := range dbGroups {
		gCfg := dbGroupToConfig(dbGroup)
		group, err := NewModelGroup(gCfg, channels)
		if err != nil {
			middleware.GetLogger().Error("error initializing model group",
				zap.String("group", gCfg.Name), zap.Error(err))
			continue
		}
		if len(group.Members) == 0 {
			middleware.GetLogger().Warn("model group has no valid members, skipping",
				zap.String("group", gCfg.Name))
			continue
		}
		modelGroups[gCfg.Name] = group

		if startCleanupLoops {
			stop := group.StartCleanup(1 * time.Minute)
			stopChans = append(stopChans, stop)
		}

		middleware.GetLogger().Info("initialized model group",
			zap.String("group", gCfg.Name), zap.Int("members", len(group.Members)),
			zap.String("strategy", gCfg.Strategy), zap.Int("sticky_ttl_seconds", gCfg.StickyTTLSeconds))
	}

	s.channels = channels
	s.modelMap = modelMap
	s.modelGroups = modelGroups
	s.groupStopChans = stopChans

	middleware.GetLogger().Info("loaded llm-proxy config from DB",
		zap.Int("channels", len(channels)), zap.Int("models", len(modelMap)),
		zap.Int("model_groups", len(modelGroups)))
	return nil
}

// Reload reloads all channels and model groups from the database,
// preserving health and rate-limiter state for unchanged channels.
func (s *LLMProxyService) Reload() error {
	s.startMu.Lock()
	started := s.started
	s.startMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop only the per-load group-cleanup goroutines. Long-lived background loops
	// (conv cleanup, log archival, quota alerts) live in bgStopChans and must NOT be
	// touched here, or they'd silently die on the first reload (audit #9).
	for _, stop := range s.groupStopChans {
		close(stop)
	}
	s.groupStopChans = nil

	// Preserve existing channel state
	oldChannels := s.channels

	if err := s.loadFromDB(started); err != nil {
		return err
	}

	// Restore health + bucket state for channels that still exist. Carry the old
	// bucket (preserves token level + daily counter) but RESIZE it to the freshly
	// learned-safe RPM so adaptive rate-limit learning takes effect on reload.
	for name, newCh := range s.channels {
		if oldCh, ok := oldChannels[name]; ok {
			newCh.Health = oldCh.Health
			// Quota-window mode changes migrate the counter in place instead of
			// dropping it (calendar↔rolling, 2026-09-24 29 号方案).
			oldCh.Bucket.SetQuotaWindow(time.Duration(newCh.Config.QuotaWindowSeconds) * time.Second)
			oldCh.Bucket.Resize(s.effectiveBucketRPM(newCh.Config), s.cfg.DefaultBucketRPM)
			newCh.Bucket = oldCh.Bucket
		}
	}

	// Balance providers are constructed from channel configs, so a reload must
	// rebuild them; otherwise newly added/removed balance channels only take
	// effect after a process restart.
	if s.balanceMgr != nil {
		s.balanceMgr.Reset()
		s.registerBalanceProviders()
	}
	if s.taskRouter != nil {
		s.taskRouter.SetCodingStrategy(s.loadedCodingStrategy(s.cfg.CodingRoutingStrategy))
	}

	return nil
}

// resolveChannelKey returns the effective API key for a channel and its source:
// "credential" (DB-stored, purpose=api) | "env" (APIKeyEnv resolved from the
// environment) | "literal" (APIKeyEnv holds a raw key) | "" (none available). A DB
// credential always takes precedence over the environment variable. Both
// dbChannelToConfig (the key actually sent upstream) and channelToView (the status
// shown in the UI) call this, so the displayed status can never disagree with the
// key in use.
func resolveChannelKey(credentialRepo *repository.LLMChannelCredentialRepository, ch *model.LLMChannel) (key, source string) {
	if credentialRepo != nil {
		if resolved, err := resolveCredentialForChannel(credentialRepo, ch.ID, "api"); err != nil {
			middleware.GetLogger().Warn("resolve credential failed, falling back to APIKeyEnv",
				zap.String("channel", ch.Name), zap.Error(err))
		} else if resolved != "" {
			return resolved, "credential"
		}
	}
	if ch.APIKeyEnv == "" {
		return "", ""
	}
	if v := os.Getenv(ch.APIKeyEnv); v != "" {
		return v, "env"
	}
	if envutil.LooksLikeEnvVar(ch.APIKeyEnv) {
		return "", "" // looks like an env-var name but unset → no key available
	}
	return ch.APIKeyEnv, "literal" // not an env-var name → legacy literal key
}

func (s *LLMProxyService) dbChannelToConfig(ch model.LLMChannel) config.ChannelConfig {
	apiKey, source := resolveChannelKey(s.credentialRepo, &ch)
	if apiKey == "" && envutil.LooksLikeEnvVar(ch.APIKeyEnv) {
		middleware.GetLogger().Warn("channel API key env var not set",
			zap.String("channel", ch.Name), zap.String("env_var", ch.APIKeyEnv))
	} else if source == "literal" {
		middleware.GetLogger().Warn("channel api_key_env is not a valid env var name, treating as direct key",
			zap.String("channel", ch.Name), zap.String("api_key_env", maskAPIKey(ch.APIKeyEnv)))
	}
	providerType := ch.ProviderType
	if providerType == "" {
		providerType = "openai"
	}
	vendor := ch.Vendor
	if vendor == "" {
		vendor = InferVendor(ch.BaseURL)
	}

	return config.ChannelConfig{
		ID:                  ch.ID,
		Name:                ch.Name,
		BaseURL:             ch.BaseURL,
		APIKey:              apiKey,
		ProviderType:        providerType,
		Vendor:              vendor,
		RPM:                 ch.RPM,
		RPD:                 ch.RPD,
		QuotaWindowSeconds:  ch.QuotaWindowSeconds,
		Priority:            ch.Priority,
		Models:              ch.GetModels(),
		IsEnabled:           ch.IsEnabled,
		IsFree:              ch.IsFree,
		BalanceProviderType: ch.BalanceProviderType,
		BalanceConfigJSON:   ch.BalanceConfigJSON,
		ModelRPMOverrides:   ch.ModelRPMOverrides,
		TaskTypes:           ch.GetTaskTypes(),
		Tier:                ch.Tier,
	}
}

// effectiveBucketRPM seeds per-channel-model rate-limit rows with the configured
// baseline and returns the learned-safe RPM used to size the channel's token bucket
// (min across the channel's models — conservative). Falls back to the configured RPM
// when no learner is present. This is what makes adaptive learning actually feed back
// into client-side throttling (audit #5).
func (s *LLMProxyService) effectiveBucketRPM(cfg config.ChannelConfig) int {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil || cfg.ID == 0 || len(cfg.Models) == 0 {
		return cfg.RPM
	}
	effective := 0
	for _, m := range cfg.Models {
		rl, err := s.rateLimitLearner.repo.GetOrCreate(cfg.ID, m, cfg.RPM)
		if err != nil || rl == nil {
			continue
		}
		s.rateLimitLearner.CachePut(rl) // so GetSafeRPM/Record429 see the seeded row
		safe := rl.LearnedRPMSafe
		if safe <= 0 {
			safe = int(float64(cfg.RPM) * 0.5) // cold-start fallback
		}
		if effective == 0 || safe < effective {
			effective = safe
		}
	}
	if effective == 0 {
		return cfg.RPM
	}
	return effective
}

func dbGroupToConfig(g model.LLMModelGroup) config.ModelGroupConfig {
	members := make([]config.ModelGroupMember, len(g.Members))
	for i, m := range g.Members {
		members[i] = config.ModelGroupMember{
			Channel:          m.ChannelName,
			Model:            m.Model,
			Weight:           m.Weight,
			MaxContextTokens: m.MaxContextTokens,
			MaxOutputTokens:  m.MaxOutputTokens,
		}
	}
	return config.ModelGroupConfig{
		Name:             g.Name,
		Description:      g.Description,
		Strategy:         g.Strategy,
		StickyTTLSeconds: g.StickyTTLSeconds,
		Members:          members,
	}
}

// ChannelConfigView extends model.LLMChannel with API key status information
// for the frontend, without exposing the actual key value.
type ChannelConfigView struct {
	model.LLMChannel
	APIKeyStatus  string `json:"api_key_status"`  // "configured" | "missing" | "direct"
	APIKeyPreview string `json:"api_key_preview"` // masked preview, e.g. "sk-o...Lvr"
}

// maskAPIKey renders a preview of the API key: first 4 + "..." + last 4 chars.
func maskAPIKey(s string) string {
	n := len(s)
	if n == 0 {
		return ""
	}
	if n <= 8 {
		return "****"
	}
	return s[:4] + "..." + s[n-4:]
}

func channelToView(credentialRepo *repository.LLMChannelCredentialRepository, ch *model.LLMChannel) ChannelConfigView {
	v := ChannelConfigView{LLMChannel: *ch}
	if v.ProviderType == "" {
		v.ProviderType = "openai"
	}
	if v.Vendor == "" {
		v.Vendor = InferVendor(v.BaseURL)
	}
	key, source := resolveChannelKey(credentialRepo, ch)
	switch source {
	case "credential", "env":
		v.APIKeyStatus = "configured"
		v.APIKeyPreview = maskAPIKey(key)
	case "literal":
		v.APIKeyStatus = "direct"
		v.APIKeyPreview = maskAPIKey(key)
	default:
		v.APIKeyStatus = "missing"
		v.APIKeyPreview = ""
	}
	return v
}

// loadedCodingStrategy returns the persisted UI choice when present, otherwise
// the YAML/default strategy. Invalid stored values are ignored.
func (s *LLMProxyService) loadedCodingStrategy(fallback string) string {
	if s.settingRepo == nil {
		return fallback
	}
	setting, err := s.settingRepo.GetByKey(SettingCodingRoutingStrategy)
	if err != nil || setting == nil {
		return fallback
	}
	switch setting.Value {
	case "free_first", "quality_first", "complexity_aware":
		return setting.Value
	default:
		return fallback
	}
}
