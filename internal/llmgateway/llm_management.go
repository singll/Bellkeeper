package llmgateway

import (
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

// ListAlertEvents returns recent alert events for the alerts UI.
func (s *LLMProxyService) ListAlertEvents(severity, alertType string, hours, limit int) ([]model.LLMAlertEvent, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("proxy repo not initialized")
	}
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	return s.repo.ListAlertEvents(since, severity, alertType, limit)
}

// --- Management APIs ---

// GetChannelsStatus returns current status of all channels including bucket state and health.
func (s *LLMProxyService) GetChannelsStatus() []map[string]interface{} {
	s.mu.RLock()
	channels := s.channels
	s.mu.RUnlock()

	var result []map[string]interface{}
	for name, ch := range channels {
		status := ch.Bucket.Status()
		status["name"] = name
		status["base_url"] = ch.Config.BaseURL
		status["models"] = ch.Config.Models
		status["priority"] = ch.Config.Priority
		status["rpm_limit"] = ch.Config.RPM
		status["rpd_limit"] = ch.Config.RPD
		status["is_free"] = ch.Config.IsFree
		status["health"] = ch.Health.Status()
		// 真实额度观测（29 号方案）：balance provider 已配置时暴露窗口余量
		// （OpenCode Go：rolling/weekly/monthly 最紧窗口剩余比例；货币单位
		// window_ratio 表示非美元）。dsh 供给哨兵优先用该比值做降速判断，
		// 无数据时回退本地令牌桶口径。
		if info := s.GetChannelBalance(name); info != nil && info.Error == "" {
			status["quota_ratio_remaining"] = info.Balance
			status["quota_kind"] = string(info.Kind)
			status["quota_currency"] = info.Currency
			if info.ExpiresAt != nil {
				status["quota_window_resets_at"] = info.ExpiresAt.Format(time.RFC3339)
			}
			status["quota_fetched_at"] = info.FetchedAt.Format(time.RFC3339)
		}
		result = append(result, status)
	}
	return result
}

// GetHealthStatus returns health status of all channels.
func (s *LLMProxyService) GetHealthStatus() []map[string]interface{} {
	s.mu.RLock()
	channels := s.channels
	s.mu.RUnlock()

	var result []map[string]interface{}
	for name, ch := range channels {
		entry := map[string]interface{}{
			"name":     name,
			"models":   ch.Config.Models,
			"priority": ch.Config.Priority,
			"is_free":  ch.Config.IsFree,
			"health":   ch.Health.Status(),
		}
		result = append(result, entry)
	}
	return result
}

// GetGroupsStatus returns status of all virtual model groups.
func (s *LLMProxyService) GetGroupsStatus() []map[string]interface{} {
	s.mu.RLock()
	modelGroups := s.modelGroups
	s.mu.RUnlock()

	var result []map[string]interface{}
	for _, group := range modelGroups {
		result = append(result, group.Status())
	}
	return result
}

// ClearGroupSticky clears all sticky bindings for the named model group.
// Returns the number of bindings cleared, or -1 if the group doesn't exist.
func (s *LLMProxyService) ClearGroupSticky(groupName string) int {
	s.mu.RLock()
	group, ok := s.modelGroups[groupName]
	s.mu.RUnlock()

	if !ok {
		return -1
	}
	if group.Sticky == nil {
		return 0
	}
	return group.Sticky.Clear()
}

// ResetChannelCircuit resets the circuit breaker for the named channel.
// Returns false if the channel doesn't exist.
func (s *LLMProxyService) ResetChannelCircuit(channelName string) bool {
	s.mu.RLock()
	ch, ok := s.channels[channelName]
	s.mu.RUnlock()

	if !ok {
		return false
	}
	ch.Health.Reset()
	return true
}

// GetStats returns aggregated statistics for the given time window.
func (s *LLMProxyService) GetStats(hours int) (map[string]interface{}, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats, err := s.repo.GetStats(since)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"channels": stats, "since": since}, nil
}

// GetRecentLogs returns recent proxy request logs.
func (s *LLMProxyService) GetRecentLogs(channelName string, limit int) ([]model.LLMProxyLog, error) {
	return s.repo.GetRecentLogs(channelName, limit)
}

// GetRateLimitEvents returns recent rate-limit events.
func (s *LLMProxyService) GetRateLimitEvents(hours int, channelName string) ([]model.LLMProxyLog, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	return s.repo.GetRateLimitEvents(since, channelName)
}

// --- Conversations ---

// GetConversations returns all active conversation bindings.
func (s *LLMProxyService) GetConversations() []*model.LLMConversationBinding {
	if s.convMgr == nil {
		return nil
	}
	return s.convMgr.List()
}

// DeleteConversation removes a conversation binding.
func (s *LLMProxyService) DeleteConversation(convID string) error {
	if s.convMgr == nil {
		return fmt.Errorf("conversation manager not initialized")
	}
	s.convMgr.Remove(convID)
	if s.convMgr.repo != nil {
		return s.convMgr.repo.Delete(convID)
	}
	return nil
}

// --- Rate Limits ---

// GetRateLimits returns the adaptive rate-limit learning state for all channel×model pairs.
func (s *LLMProxyService) GetRateLimits() ([]*model.LLMModelRateLimit, error) {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil {
		return nil, nil
	}
	return s.rateLimitLearner.repo.ListAll()
}

// ResetRateLimit resets learned rate limit for a channel×model to cold-start defaults.
func (s *LLMProxyService) ResetRateLimit(channelID uint, model string) error {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil {
		return fmt.Errorf("rate limit learner not initialized")
	}
	rl, err := s.rateLimitLearner.repo.GetOrCreate(channelID, model, 0)
	if err != nil {
		return err
	}
	// Reset to configured * 0.5
	rl.LearnedRPMSafe = int(float64(rl.ConfiguredRPM) * 0.5)
	rl.LearnedRPDSafe = 0
	rl.ConfidenceScore = 0.1
	rl.LastAdjustAt = nil
	rl.Last429At = nil
	rl.Last429ObservedRPM = 0
	rl.AdjustmentLog = nil
	return s.rateLimitLearner.repo.Update(rl)
}

// LockRateLimit toggles the locked state for a rate limit record.
func (s *LLMProxyService) LockRateLimit(id uint, locked bool) error {
	if s.rateLimitLearner == nil || s.rateLimitLearner.repo == nil {
		return fmt.Errorf("rate limit learner not initialized")
	}
	return s.rateLimitLearner.repo.SetLocked(id, locked)
}

// --- Task Router ---

// GetCodingStrategy returns the current coding routing strategy.
func (s *LLMProxyService) GetCodingStrategy() string {
	if s.taskRouter == nil {
		return "complexity_aware"
	}
	return s.taskRouter.GetCodingStrategy()
}

// SetCodingStrategy updates and persists the coding routing strategy.
func (s *LLMProxyService) SetCodingStrategy(strategy string) error {
	switch strategy {
	case "free_first", "quality_first", "complexity_aware":
	default:
		return invalidf("strategy must be one of: free_first, quality_first, complexity_aware")
	}
	if s.settingRepo != nil {
		if err := s.settingRepo.Set(SettingCodingRoutingStrategy, strategy, "string", "llm", "LLM coding routing strategy", false); err != nil {
			return fmt.Errorf("persist coding strategy: %w", err)
		}
	}
	if s.taskRouter != nil {
		s.taskRouter.SetCodingStrategy(strategy)
	}
	return nil
}

// --- Usage / Billing ---

// GetUsageAggregates returns token usage aggregated by the requested dimension.
func (s *LLMProxyService) GetUsageAggregates(groupBy string, from, to time.Time) ([]map[string]interface{}, error) {
	// model dimension has no column in llm_token_usage_daily; aggregate from proxy logs.
	if groupBy == "model" {
		if s.repo == nil {
			return nil, fmt.Errorf("proxy repo not initialized")
		}
		return s.repo.AggregateByModel(from, to)
	}
	if s.tokenUsageRepo == nil {
		return nil, fmt.Errorf("token usage repo not initialized")
	}
	return s.tokenUsageRepo.Aggregate(groupBy, from, to)
}
