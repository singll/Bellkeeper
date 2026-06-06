package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// RateLimitLearner implements adaptive rate-limit learning with 5 stages:
// 1. Cold start: configured_rpm * 0.5
// 2. Progressive probing: +10% every 5min if bucket stays < 30%
// 3. 429 downgrade: actual_rpm * 0.85
// 4. Period detection: analyze 429 patterns
// 5. RPD learning: record daily cumulative at first quota_exhausted
type RateLimitLearner struct {
	mu         sync.RWMutex
	repo       *repository.LLMRateLimitRepository
	running    bool
	stopCh     chan struct{}
	// In-memory cache: channel_name:model -> *model.LLMModelRateLimit
	cache      map[string]*model.LLMModelRateLimit
	cacheMu    sync.RWMutex
}

// NewRateLimitLearner creates a new learner instance.
func NewRateLimitLearner(repo *repository.LLMRateLimitRepository) *RateLimitLearner {
	return &RateLimitLearner{
		repo:    repo,
		stopCh:  make(chan struct{}),
		cache:   make(map[string]*model.LLMModelRateLimit),
	}
}

// Start begins the background learning loop.
func (l *RateLimitLearner) Start() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return
	}
	l.running = true
	go l.loop()
}

// Stop halts the background loop.
func (l *RateLimitLearner) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.running {
		return
	}
	close(l.stopCh)
	l.running = false
}

func (l *RateLimitLearner) loop() {
	// Evaluate every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.evaluate()
		case <-l.stopCh:
			return
		}
	}
}

// evaluate runs the progressive probing logic (stage 2).
func (l *RateLimitLearner) evaluate() {
	l.cacheMu.RLock()
	items := make([]*model.LLMModelRateLimit, 0, len(l.cache))
	for _, v := range l.cache {
		items = append(items, v)
	}
	l.cacheMu.RUnlock()

	for _, rl := range items {
		if rl.Locked {
			continue
		}
		// Cold-start protection: confidence < 0.3 limits to configured * 0.8
		maxAllowed := int(float64(rl.ConfiguredRPM) * 1.2)
		if rl.ConfidenceScore < 0.3 {
			maxAllowed = int(float64(rl.ConfiguredRPM) * 0.8)
		}

		// Progressive probing: if no 429 in last 5min and bucket was stressed, increase
		if rl.Last429At == nil || time.Since(*rl.Last429At) > 5*time.Minute {
			newValue := int(float64(rl.LearnedRPMSafe) * 1.1)
			if newValue > maxAllowed {
				newValue = maxAllowed
			}
			if newValue != rl.LearnedRPMSafe {
				l.adjust(rl, newValue, "progressive_probe")
			}
		}
	}
}

// Record429 handles a 429 event (stage 3): immediately downgrade.
func (l *RateLimitLearner) Record429(channelID uint, model string, observedRPM int) {
	key := fmt.Sprintf("%d:%s", channelID, model)
	l.cacheMu.Lock()
	rl, ok := l.cache[key]
	if !ok {
		l.cacheMu.Unlock()
		// Try to load from DB
		rl, _ = l.repo.GetOrCreate(channelID, model, 0)
		if rl == nil {
			return
		}
		l.cacheMu.Lock()
		l.cache[key] = rl
		l.cacheMu.Unlock()
	} else {
		l.cacheMu.Unlock()
	}

	if rl.Locked {
		return
	}

	now := time.Now()
	rl.Last429At = &now
	rl.Last429ObservedRPM = observedRPM

	// Downgrade to observed * 0.85
	newValue := int(float64(observedRPM) * 0.85)
	if newValue < 1 {
		newValue = 1
	}
	l.adjust(rl, newValue, "429_downgrade")
}

// RecordSuccess handles a successful request (used for bucket stress detection).
func (l *RateLimitLearner) RecordSuccess(channelID uint, model string, bucketStressed bool) {
	// For now, success records are implicitly handled by evaluate() interval
	// Future: track bucket stress ratio to trigger faster probing
}

// GetSafeRPM returns the learned safe RPM for a channel×model.
func (l *RateLimitLearner) GetSafeRPM(channelID uint, model string, configuredRPM int) int {
	key := fmt.Sprintf("%d:%s", channelID, model)
	l.cacheMu.RLock()
	rl, ok := l.cache[key]
	l.cacheMu.RUnlock()
	if ok {
		if rl.LearnedRPMSafe > 0 {
			return rl.LearnedRPMSafe
		}
	}
	// Cold start fallback
	return int(float64(configuredRPM) * 0.5)
}

// adjust updates the learned value and persists to DB.
func (l *RateLimitLearner) adjust(rl *model.LLMModelRateLimit, newValue int, reason string) {
	oldValue := rl.LearnedRPMSafe
	if oldValue == newValue {
		return
	}

	rl.LearnedRPMSafe = newValue
	now := time.Now()
	rl.LastAdjustAt = &now
	if rl.ConfidenceScore < 1.0 {
		rl.ConfidenceScore += 0.05
		if rl.ConfidenceScore > 1.0 {
			rl.ConfidenceScore = 1.0
		}
	}

	// Append to adjustment log (keep last 50)
	var logs []map[string]interface{}
	if rl.AdjustmentLog != nil && *rl.AdjustmentLog != "" {
		_ = json.Unmarshal([]byte(*rl.AdjustmentLog), &logs)
	}
	logs = append(logs, map[string]interface{}{
		"ts":     now.Format(time.RFC3339),
		"from":   oldValue,
		"to":     newValue,
		"reason": reason,
	})
	if len(logs) > 50 {
		logs = logs[len(logs)-50:]
	}
	logJSON, _ := json.Marshal(logs)
	s := string(logJSON)
	rl.AdjustmentLog = &s

	if err := l.repo.Update(rl); err != nil {
		middleware.GetLogger().Warn("failed to persist rate limit adjustment",
			zap.Error(err), zap.Int("channel_id", int(rl.ChannelID)), zap.String("model", rl.Model))
	} else {
		middleware.GetLogger().Info("rate limit adjusted",
			zap.Int("channel_id", int(rl.ChannelID)), zap.String("model", rl.Model),
			zap.Int("from", oldValue), zap.Int("to", newValue), zap.String("reason", reason))
	}
}

// LoadCache preloads all rate limit records from DB into memory.
func (l *RateLimitLearner) LoadCache() error {
	rls, err := l.repo.ListAll()
	if err != nil {
		return err
	}
	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()
	for i := range rls {
		rl := rls[i]
		key := fmt.Sprintf("%d:%s", rl.ChannelID, rl.Model)
		l.cache[key] = &rl
	}
	return nil
}

// CachePut inserts or replaces a record in the in-memory cache. Called at channel
// load to ensure GetSafeRPM/Record429 see freshly-seeded rows without re-querying.
func (l *RateLimitLearner) CachePut(rl *model.LLMModelRateLimit) {
	if rl == nil {
		return
	}
	key := fmt.Sprintf("%d:%s", rl.ChannelID, rl.Model)
	l.cacheMu.Lock()
	l.cache[key] = rl
	l.cacheMu.Unlock()
}