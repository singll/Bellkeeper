package service

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

// --- Sticky Binding ---

// StickyBinding maps a task key to a specific channel for the duration of a task.
type StickyBinding struct {
	Channel   *Channel
	Model     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// StickyBindingTable is a thread-safe in-memory table of sticky bindings with TTL.
type StickyBindingTable struct {
	mu       sync.RWMutex
	bindings map[string]*StickyBinding
}

func NewStickyBindingTable() *StickyBindingTable {
	return &StickyBindingTable{
		bindings: make(map[string]*StickyBinding),
	}
}

// Get returns the binding for the given key, or nil if not found or expired.
func (t *StickyBindingTable) Get(key string) *StickyBinding {
	t.mu.RLock()
	defer t.mu.RUnlock()

	b, ok := t.bindings[key]
	if !ok {
		return nil
	}
	if time.Now().After(b.ExpiresAt) {
		return nil
	}
	return b
}

// Set creates or replaces a sticky binding with the given TTL.
func (t *StickyBindingTable) Set(key string, ch *Channel, model string, ttlSec int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	t.bindings[key] = &StickyBinding{
		Channel:   ch,
		Model:     model,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Duration(ttlSec) * time.Second),
	}
}

// Remove deletes a sticky binding.
func (t *StickyBindingTable) Remove(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.bindings, key)
}

// Renew extends the TTL of an existing binding.
func (t *StickyBindingTable) Renew(key string, ttlSec int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if b, ok := t.bindings[key]; ok {
		b.ExpiresAt = time.Now().Add(time.Duration(ttlSec) * time.Second)
	}
}

// Cleanup removes all expired bindings. Called periodically.
func (t *StickyBindingTable) Cleanup() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	removed := 0
	for k, b := range t.bindings {
		if now.After(b.ExpiresAt) {
			delete(t.bindings, k)
			removed++
		}
	}
	return removed
}

// Clear removes all bindings.
func (t *StickyBindingTable) Clear() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	n := len(t.bindings)
	t.bindings = make(map[string]*StickyBinding)
	return n
}

// Count returns the number of active (non-expired) bindings.
func (t *StickyBindingTable) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now()
	count := 0
	for _, b := range t.bindings {
		if now.Before(b.ExpiresAt) {
			count++
		}
	}
	return count
}

// --- Model Group ---

// ModelGroupMemberRuntime holds the resolved runtime state for a group member.
type ModelGroupMemberRuntime struct {
	Config      config.ModelGroupMember
	Channel     *Channel
	totalReqs   int64
	totalErrors int64
	ewmaLatency float64 // milliseconds, exponential weighted moving average
	mu          sync.Mutex
}

// RecordLatency updates the EWMA latency score.
// alpha=0.1 gives 10% weight to the latest sample.
func (m *ModelGroupMemberRuntime) RecordLatency(durationMs int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalReqs++
	if m.ewmaLatency == 0 {
		m.ewmaLatency = float64(durationMs)
	} else {
		const alpha = 0.1
		m.ewmaLatency = alpha*float64(durationMs) + (1-alpha)*m.ewmaLatency
	}
}

// RecordError increments the error counter.
func (m *ModelGroupMemberRuntime) RecordError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalErrors++
}

// ErrorRate returns the fraction of requests that errored.
func (m *ModelGroupMemberRuntime) ErrorRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.totalReqs == 0 {
		return 0
	}
	return float64(m.totalErrors) / float64(m.totalReqs)
}

// Score returns a composite score for least_latency strategy.
// Lower is better. Combines EWMA latency with error-rate penalty.
func (m *ModelGroupMemberRuntime) Score() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	errRate := 0.0
	if m.totalReqs > 0 {
		errRate = float64(m.totalErrors) / float64(m.totalReqs)
	}
	return m.ewmaLatency * (1 + errRate*10) // 100% error rate = 10x penalty
}

// ModelGroup represents a virtual model name that maps to multiple real channels.
type ModelGroup struct {
	Config  config.ModelGroupConfig
	Members []*ModelGroupMemberRuntime
	Sticky  *StickyBindingTable
}

// NewModelGroup creates a model group from config, resolving channel references.
func NewModelGroup(cfg config.ModelGroupConfig, channels map[string]*Channel) (*ModelGroup, error) {
	g := &ModelGroup{
		Config: cfg,
	}

	if cfg.StickyTTLSeconds > 0 {
		g.Sticky = NewStickyBindingTable()
	}

	for _, m := range cfg.Members {
		ch, ok := channels[m.Channel]
		if !ok {
			log.Printf("llm-proxy: warning: model group %q references unknown channel %q, skipping member", cfg.Name, m.Channel)
			continue
		}
		weight := m.Weight
		if weight <= 0 {
			weight = 1
		}
		g.Members = append(g.Members, &ModelGroupMemberRuntime{
			Config:  config.ModelGroupMember{Channel: m.Channel, Model: m.Model, Weight: weight},
			Channel: ch,
		})
	}

	return g, nil
}

// SelectChannel picks a channel for the given task key, respecting sticky bindings
// and health state.
func (g *ModelGroup) SelectChannel(taskKey string) (*Channel, string) {
	// 1. Check sticky binding
	if taskKey != "" && g.Sticky != nil {
		if binding := g.Sticky.Get(taskKey); binding != nil {
			if binding.Channel.Health.IsAvailable() {
				return binding.Channel, binding.Model
			}
			// Bound channel is unhealthy, clear binding and re-select
			g.Sticky.Remove(taskKey)
		}
	}

	// 2. Filter available members
	available := g.filterAvailable()
	if len(available) == 0 {
		return nil, ""
	}

	// 3. Select by strategy
	var selected *ModelGroupMemberRuntime
	switch g.Config.Strategy {
	case "round-robin":
		selected = weightedSelect(available)
	case "least_latency":
		selected = leastLatencySelect(available)
	case "balance_aware":
		selected = balanceAwareSelect(available)
	default: // "priority-health"
		selected = priorityHealthSelect(available)
	}

	if selected == nil {
		return nil, ""
	}

	// 4. Establish sticky binding
	if taskKey != "" && g.Sticky != nil {
		g.Sticky.Set(taskKey, selected.Channel, selected.Config.Model, g.Config.StickyTTLSeconds)
	}

	return selected.Channel, selected.Config.Model
}

// GetStickyBinding returns the current sticky binding for a task key without modifying it.
func (g *ModelGroup) GetStickyBinding(taskKey string) (*Channel, string) {
	if taskKey == "" || g.Sticky == nil {
		return nil, ""
	}
	if binding := g.Sticky.Get(taskKey); binding != nil {
		return binding.Channel, binding.Model
	}
	return nil, ""
}

// filterAvailable returns members whose channels are not circuit-broken
// and have tokens remaining.
func (g *ModelGroup) filterAvailable() []*ModelGroupMemberRuntime {
	var available []*ModelGroupMemberRuntime
	for _, m := range g.Members {
		if m.Channel.Health.IsAvailable() {
			available = append(available, m)
		}
	}
	return available
}

// priorityHealthSelect sorts candidates by priority (asc), health score (desc),
// then token availability (desc), and returns the best one.
func priorityHealthSelect(candidates []*ModelGroupMemberRuntime) *ModelGroupMemberRuntime {
	if len(candidates) == 0 {
		return nil
	}

	type scored struct {
		member      *ModelGroupMemberRuntime
		priority    int
		healthScore float64
		tokenRatio  float64
	}

	items := make([]scored, len(candidates))
	for i, m := range candidates {
		bucketStatus := m.Channel.Bucket.Status()
		avail, _ := bucketStatus["available_tokens"].(int)
		maxT, _ := bucketStatus["max_tokens"].(int)
		tokenRatio := 0.0
		if maxT > 0 {
			tokenRatio = float64(avail) / float64(maxT)
		}

		items[i] = scored{
			member:      m,
			priority:    m.Channel.Config.Priority,
			healthScore: m.Channel.Health.HealthScore(),
			tokenRatio:  tokenRatio,
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].priority != items[j].priority {
			return items[i].priority < items[j].priority
		}
		if items[i].healthScore != items[j].healthScore {
			return items[i].healthScore > items[j].healthScore
		}
		return items[i].tokenRatio > items[j].tokenRatio
	})

	return items[0].member
}

// weightedSelect does a simple weighted random selection among candidates.
func weightedSelect(candidates []*ModelGroupMemberRuntime) *ModelGroupMemberRuntime {
	if len(candidates) == 0 {
		return nil
	}

	totalWeight := 0
	for _, m := range candidates {
		totalWeight += m.Config.Weight
	}
	if totalWeight == 0 {
		return candidates[0]
	}

	// Use a deterministic approach: pick the highest-weight available member.
	// This is simpler and more predictable than random for small pools.
	best := candidates[0]
	for _, m := range candidates[1:] {
		if m.Config.Weight > best.Config.Weight {
			best = m
		}
	}
	return best
}

// leastLatencySelect picks the member with the lowest EWMA latency score.
func leastLatencySelect(candidates []*ModelGroupMemberRuntime) *ModelGroupMemberRuntime {
	if len(candidates) == 0 {
		return nil
	}
	best := candidates[0]
	bestScore := best.Channel.EWMALatency()
	for _, m := range candidates[1:] {
		if s := m.Channel.EWMALatency(); s < bestScore {
			best = m
			bestScore = s
		}
	}
	return best
}

// balanceAwareSelect prefers members with positive balance, then falls back to least_latency.
func balanceAwareSelect(candidates []*ModelGroupMemberRuntime) *ModelGroupMemberRuntime {
	if len(candidates) == 0 {
		return nil
	}
	// Filter members with known positive balance (if balance info is available)
	var positive []*ModelGroupMemberRuntime
	for _, m := range candidates {
		// Balance awareness is applied at channel level via balance.Manager
		// For now, we use a heuristic: free channels are deprioritized if paid channels have balance
		if !m.Channel.Config.IsFree {
			positive = append(positive, m)
		}
	}
	if len(positive) > 0 {
		return leastLatencySelect(positive)
	}
	return leastLatencySelect(candidates)
}

// Status returns a snapshot of the model group state for management APIs.
func (g *ModelGroup) Status() map[string]interface{} {
	status := map[string]interface{}{
		"name":               g.Config.Name,
		"description":        g.Config.Description,
		"strategy":           g.Config.Strategy,
		"sticky_ttl_seconds": g.Config.StickyTTLSeconds,
	}

	members := make([]map[string]interface{}, 0, len(g.Members))
	for _, m := range g.Members {
		ms := map[string]interface{}{
			"channel":   m.Config.Channel,
			"model":     m.Config.Model,
			"weight":    m.Config.Weight,
			"available": m.Channel.Health.IsAvailable(),
			"health":    m.Channel.Health.Status(),
		}
		members = append(members, ms)
	}
	status["members"] = members

	if g.Sticky != nil {
		status["sticky_bindings"] = g.Sticky.Count()
	} else {
		status["sticky_bindings"] = 0
	}

	return status
}

// StartCleanup launches a background goroutine that periodically cleans up
// expired sticky bindings. Returns a stop channel.
func (g *ModelGroup) StartCleanup(interval time.Duration) chan struct{} {
	stop := make(chan struct{})
	if g.Sticky == nil {
		return stop
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if removed := g.Sticky.Cleanup(); removed > 0 {
					log.Printf("llm-proxy: group %q cleaned up %d expired sticky bindings", g.Config.Name, removed)
				}
			case <-stop:
				return
			}
		}
	}()

	return stop
}

// rewriteModel replaces the "model" field in a JSON request body with a new model name.
func rewriteModel(body []byte, newModel string) []byte {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	req["model"] = newModel
	rewritten, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return rewritten
}
