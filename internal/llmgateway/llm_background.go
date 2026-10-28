package llmgateway

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	llmerrors "github.com/singll/bellkeeper/internal/llmgateway/errors"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"go.uber.org/zap"
)

// balanceSnapshotLoop persists a balance-history row per channel each time the
// balance manager publishes a fresh fetch. It ticks at the balance sync interval
// and dedupes by FetchedAt so a channel that hasn't refreshed isn't re-recorded.
func (s *LLMProxyService) balanceSnapshotLoop(stopCh <-chan struct{}) {
	interval := time.Duration(s.cfg.BalanceSyncInterval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastSnapped := make(map[string]time.Time) // channel -> last persisted FetchedAt
	for {
		select {
		case <-ticker.C:
			s.snapshotBalances(lastSnapped)
		case <-stopCh:
			return
		}
	}
}

// snapshotBalances reads the latest balances and persists one history row per
// channel whose fetch timestamp advanced since the previous tick. Failed fetches
// (info.Error set) are skipped so the history stays a clean record of real balances.
func (s *LLMProxyService) snapshotBalances(lastSnapped map[string]time.Time) {
	if s.balanceMgr == nil || s.balanceSnapshotRepo == nil {
		return
	}
	all := s.balanceMgr.GetAll()
	if len(all) == 0 {
		return
	}
	// Resolve channel name -> DB id from the in-memory channel table (one RLock,
	// no per-channel DB round-trip).
	ids := make(map[string]uint, len(all))
	s.mu.RLock()
	for name, ch := range s.channels {
		ids[name] = ch.Config.ID
	}
	s.mu.RUnlock()

	for name, info := range all {
		if info == nil || info.Error != "" || info.FetchedAt.IsZero() {
			continue
		}
		if prev, ok := lastSnapped[name]; ok && !info.FetchedAt.After(prev) {
			continue // already captured this fetch
		}
		raw, err := json.Marshal(info)
		if err != nil {
			middleware.GetLogger().Warn("balance snapshot: marshal failed",
				zap.String("channel", name), zap.Error(err))
			continue
		}
		snap := &model.LLMChannelBalanceSnapshot{
			ChannelID:    ids[name],
			ChannelName:  name,
			BalanceUSD:   info.Balance,
			Currency:     info.Currency,
			TotalGranted: info.TotalGranted,
			TotalUsed:    info.TotalUsed,
			BalanceRaw:   string(raw),
			FetchedAt:    info.FetchedAt,
		}
		if err := s.balanceSnapshotRepo.Create(snap); err != nil {
			middleware.GetLogger().Warn("balance snapshot: persist failed",
				zap.String("channel", name), zap.Error(err))
			continue
		}
		lastSnapped[name] = info.FetchedAt
	}
}

// kimiCodeProbeLoop periodically re-probes channels stuck in a long quota_exhausted
// breakdown (notably Kimi Code, whose quota resets on a ~5h/7d cycle). When a
// channel's breakdown window has elapsed but no live traffic has reopened it, a
// minimal 1-token probe checks whether the upstream quota actually recovered; on
// success the circuit is reset, otherwise the breakdown is re-armed so probes stay
// ~5h apart rather than firing every tick. Long-lived → registered in bgStopChans.
//
// 2026-09-24: also probes member-level quota breakdowns (per-model credit pools,
// e.g. SenseNova flash-lite vs deepseek-flash on one channel). Previously only
// channel-level breakdowns were probed, so a member whose 5h rolling window had
// recovered stayed out of rotation until its full cooldown elapsed. Interval is
// configurable via circuit_breaker.probe_interval_minutes (floor 10min).
func (s *LLMProxyService) kimiCodeProbeLoop(stopCh <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.probeQuotaExhausted()
			s.probeMemberQuotaExhausted()
		case <-stopCh:
			return
		}
	}
}

// probeQuotaExhausted sends a recovery probe to each channel whose circuit is open
// with a quota_exhausted breakdown whose window has elapsed.
func (s *LLMProxyService) probeQuotaExhausted() {
	s.mu.RLock()
	channels := make([]*Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		channels = append(channels, ch)
	}
	s.mu.RUnlock()

	now := time.Now()
	for _, ch := range channels {
		state, class, until := ch.Health.BreakdownInfo()
		if state == CircuitClosed || class != string(llmerrors.QuotaExhausted) {
			continue
		}
		if until.IsZero() || now.Before(until) {
			continue // still within the breakdown window — wait
		}
		statusCode, respBody, _, err := s.probeChannel(ch)
		if err == nil && statusCode >= 200 && statusCode < 300 {
			ch.Health.Reset()
			middleware.GetLogger().Info("quota-exhausted channel recovered via probe",
				zap.String("channel", ch.Config.Name), zap.Int("status", statusCode))
			continue
		}
		// Still down — re-arm the breakdown silently (no alert) so the next probe is
		// ~5h out, not on the next tick.
		result := classifyChannelError(ch, statusCode, respBody)
		ch.Health.RecordClassifiedFailure(classifyError(statusCode, err), string(result.Class),
			llmerrors.BreakdownDuration(result.BreakdownUntil))
		middleware.GetLogger().Info("quota-exhausted channel still down after probe",
			zap.String("channel", ch.Config.Name), zap.Int("status", statusCode))
	}
}

// probeChannel sends a minimal protocol-appropriate probe to verify upstream
// recovery. It uses tokenID 0 (skips billing) and a synthetic caller id, and
// goes through tryChannel so adapter conversion/auth/bucket/logging apply
// identically to real traffic.
func (s *LLMProxyService) probeChannel(ch *Channel) (int, []byte, http.Header, error) {
	model := ch.Config.Name
	if len(ch.Config.Models) > 0 {
		model = ch.Config.Models[0]
	}
	adapter, err := adapterFor(ch)
	if err != nil {
		return 0, nil, nil, err
	}
	body, path := adapter.ProbeRequest(model)
	return s.tryChannel(ch, "POST", path, http.Header{}, body, "kimi-code-probe", 0)
}

// probeMemberQuotaExhausted sends a 1-token probe for each (channel, model)
// group member whose member-level quota_exhausted cooldown has elapsed. On
// success the member breakdown is cleared (RecordMemberSuccess) so the model
// re-enters rotation without waiting out the full cooldown; on failure the
// cooldown is re-armed from the fresh classification.
func (s *LLMProxyService) probeMemberQuotaExhausted() {
	s.mu.RLock()
	groups := make([]*ModelGroup, 0, len(s.modelGroups))
	for _, g := range s.modelGroups {
		groups = append(groups, g)
	}
	s.mu.RUnlock()

	now := time.Now()
	seen := make(map[string]bool) // (channel:model) probed once even if in several groups
	for _, g := range groups {
		for _, m := range g.Members {
			class, until := m.MemberBreakdownInfo()
			if class != string(llmerrors.QuotaExhausted) {
				continue
			}
			if until.IsZero() || now.Before(until) {
				continue // still within cooldown
			}
			key := memberKey(m.Config.Channel, m.Config.Model)
			if seen[key] {
				continue
			}
			seen[key] = true
			ch := m.Channel
			if !ch.Health.IsAvailable() {
				continue // channel circuit open — channel-level probe handles it
			}
			adapter, adErr := adapterFor(ch)
			if adErr != nil {
				middleware.GetLogger().Warn("probe: unsupported provider protocol",
					zap.String("channel", ch.Config.Name), zap.Error(adErr))
				continue
			}
			body, probePath := adapter.ProbeRequest(m.Config.Model)
			statusCode, respBody, _, err := s.tryChannel(ch, "POST", probePath, http.Header{}, body, "member-quota-probe", 0)
			if err == nil && statusCode >= 200 && statusCode < 300 {
				// Clear the member breakdown in every group that carries this pair.
				for _, gg := range groups {
					if mm := gg.findMember(m.Config.Channel, m.Config.Model); mm != nil {
						mm.RecordMemberSuccess()
					}
				}
				middleware.GetLogger().Info("quota-exhausted member recovered via probe",
					zap.String("channel", m.Config.Channel), zap.String("model", m.Config.Model),
					zap.Int("status", statusCode))
				continue
			}
			// Still down — re-arm from the fresh classification (dynamic reset
			// windows keep this tight instead of re-blind 24h).
			result := classifyChannelError(ch, statusCode, respBody)
			if isMemberScopedClass(result.Class) {
				for _, gg := range groups {
					if mm := gg.findMember(m.Config.Channel, m.Config.Model); mm != nil {
						mm.RecordMemberBreakdown(string(result.Class), llmerrors.BreakdownDuration(result.BreakdownUntil))
					}
				}
			} else {
				// 探针撞上的是非配额类故障（限流/5xx/超时）——归渠道健康，不延长成员配额熔断。
				ch.Health.RecordClassifiedFailure(classifyError(statusCode, err), string(result.Class),
					llmerrors.BreakdownDuration(result.BreakdownUntil))
			}
			middleware.GetLogger().Info("quota-exhausted member still down after probe",
				zap.String("channel", m.Config.Channel), zap.String("model", m.Config.Model),
				zap.Int("status", statusCode), zap.String("class", string(result.Class)))
		}
	}
}

// quotaAlertLoop runs hourly, comparing each token's usage against its quotas and
// emitting 80% / 95% / 100% alerts through the aggregator (§2.6.2).
func (s *LLMProxyService) quotaAlertLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.checkQuotas()
		case <-stopCh:
			return
		}
	}
}

func (s *LLMProxyService) checkQuotas() {
	if s.tokenRepo == nil || s.tokenUsageRepo == nil || s.alertAggregator == nil {
		return
	}
	tokens, err := s.tokenRepo.List()
	if err != nil {
		middleware.GetLogger().Warn("quota check: list tokens failed", zap.Error(err))
		return
	}
	nowUTC := time.Now().UTC()
	today := time.Now().Truncate(24 * time.Hour)
	monthStart := time.Date(nowUTC.Year(), nowUTC.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, tok := range tokens {
		if !tok.Enabled {
			continue
		}
		if tok.QuotaRequestsDaily > 0 || tok.QuotaTokensDaily > 0 {
			usages, _ := s.tokenUsageRepo.ListByToken(tok.ID, today, today)
			reqs, toks := 0, 0
			for _, u := range usages {
				reqs += u.Requests
				toks += u.PromptTokens + u.CompletionTokens
			}
			if tok.QuotaRequestsDaily > 0 {
				s.emitQuotaAlert(tok.Name, "daily requests", reqs, tok.QuotaRequestsDaily)
			}
			if tok.QuotaTokensDaily > 0 {
				s.emitQuotaAlert(tok.Name, "daily tokens", toks, tok.QuotaTokensDaily)
			}
		}
		if tok.QuotaCostMonthlyCents > 0 {
			usages, _ := s.tokenUsageRepo.ListByToken(tok.ID, monthStart, nowUTC)
			var micro int64
			for _, u := range usages {
				micro += u.CostMicroCents
			}
			s.emitQuotaAlert(tok.Name, "monthly cost (cents)", int(MicroCentsToCents(micro)), tok.QuotaCostMonthlyCents)
		}
	}
}

func (s *LLMProxyService) emitQuotaAlert(tokenName, dimension string, used, limit int) {
	if limit <= 0 {
		return
	}
	pct := float64(used) / float64(limit) * 100
	var severity string
	switch {
	case pct >= 100:
		severity = "critical"
	case pct >= 95:
		severity = "error"
	case pct >= 80:
		severity = "warning"
	default:
		return
	}
	s.recordAlert("quota_threshold", severity, tokenName,
		fmt.Sprintf("token %q %s at %.0f%% (%d/%d)", tokenName, dimension, pct, used, limit))
}

// convCleanupLoop periodically cleans up expired conversation bindings.
func (s *LLMProxyService) convCleanupLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if s.convMgr != nil {
				s.convMgr.CleanupExpired()
			}
		case <-stopCh:
			return
		}
	}
}
func (s *LLMProxyService) startGroupCleanupLoops() {
	s.mu.Lock()
	defer s.mu.Unlock()

	groups := make([]*ModelGroup, 0, len(s.modelGroups))
	for _, group := range s.modelGroups {
		groups = append(groups, group)
	}

	for _, stop := range s.groupStopChans {
		close(stop)
	}
	s.groupStopChans = make([]chan struct{}, 0, len(groups))
	for _, group := range groups {
		s.groupStopChans = append(s.groupStopChans, group.StartCleanup(time.Minute))
	}
}

func (s *LLMProxyService) startBackgroundLoops() {
	convStop := make(chan struct{})
	s.bgStopChans = append(s.bgStopChans, convStop)
	go s.convCleanupLoop(convStop)

	archiveStop := make(chan struct{})
	s.bgStopChans = append(s.bgStopChans, archiveStop)
	s.startLogArchival(archiveStop)

	if s.tokenRepo != nil {
		quotaStop := make(chan struct{})
		s.bgStopChans = append(s.bgStopChans, quotaStop)
		go s.quotaAlertLoop(quotaStop)
	}

	if s.balanceMgr != nil && s.balanceSnapshotRepo != nil {
		snapStop := make(chan struct{})
		s.bgStopChans = append(s.bgStopChans, snapStop)
		go s.balanceSnapshotLoop(snapStop)
	}

	probeStop := make(chan struct{})
	s.bgStopChans = append(s.bgStopChans, probeStop)
	probeInterval := time.Duration(s.cfg.CircuitBreaker.ProbeIntervalMinutes) * time.Minute
	if probeInterval < 10*time.Minute {
		// 下限 10min：更密的探测只是给仍在窗口内的死渠道白烧额度。
		probeInterval = 10 * time.Minute
	}
	go s.kimiCodeProbeLoop(probeStop, probeInterval)
}

// startLogArchival starts a background goroutine that archives old LLM proxy logs daily.
func (s *LLMProxyService) startLogArchival(stopCh <-chan struct{}) {
	retentionDays := s.cfg.LogRetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		// Run once at startup (after a short delay)
		select {
		case <-time.After(5 * time.Minute):
			s.archiveLogs(retentionDays)
		case <-stopCh:
			return
		}

		for {
			select {
			case <-ticker.C:
				s.archiveLogs(retentionDays)
			case <-stopCh:
				return
			}
		}
	}()
}

func (s *LLMProxyService) archiveLogs(retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	month := cutoff.Format("2006-01")

	archiveDir := "/mnt/knowledge/logs-archive/llm"
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		middleware.GetLogger().Warn("failed to create archive dir", zap.String("dir", archiveDir), zap.Error(err))
		return
	}

	archivePath := filepath.Join(archiveDir, month+".jsonl.gz")
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		middleware.GetLogger().Warn("failed to open archive file", zap.String("path", archivePath), zap.Error(err))
		return
	}
	defer file.Close() //nolint:errcheck

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close() //nolint:errcheck

	// Query old logs
	logs, err := s.repo.GetLogsBefore(cutoff)
	if err != nil {
		middleware.GetLogger().Warn("failed to query old logs", zap.Error(err))
		return
	}
	if len(logs) == 0 {
		return
	}

	encoder := json.NewEncoder(gzWriter)
	for _, log := range logs {
		if err := encoder.Encode(log); err != nil {
			middleware.GetLogger().Warn("failed to encode log", zap.Error(err))
		}
	}

	// Delete archived logs
	if err := s.repo.DeleteLogsBefore(cutoff); err != nil {
		middleware.GetLogger().Warn("failed to delete old logs", zap.Error(err))
		return
	}

	middleware.GetLogger().Info("archived LLM proxy logs",
		zap.Int("count", len(logs)),
		zap.String("archive", archivePath),
		zap.Time("cutoff", cutoff))
}
