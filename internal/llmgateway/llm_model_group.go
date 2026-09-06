package llmgateway

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

// memberKey uniquely identifies a model-group member by (channel, model).
// This allows the same physical channel to appear multiple times in a group
// with different upstream models (e.g. sensenova/glm-5.2 and sensenova/deepseek-v4-flash).
func memberKey(channel, model string) string { return channel + ":" + model }

// memberExcluded reports whether a (channel, model) pair is in the tried set.
// For backward compatibility it also accepts a bare channel name as an exclusion key.
func memberExcluded(exclude map[string]bool, channel, model string) bool {
	if exclude == nil {
		return false
	}
	if exclude[memberKey(channel, model)] {
		return true
	}
	return exclude[channel]
}

// --- Model Group ---

// ModelGroupMemberRuntime holds the resolved runtime state for a group member.
type ModelGroupMemberRuntime struct {
	Config      config.ModelGroupMember
	Channel     *Channel
	totalReqs   int64
	totalErrors int64
	ewmaLatency float64 // milliseconds, exponential weighted moving average
	// Member-level breakdown (e.g. per-model credit pools on a shared channel
	// like SenseNova, where glm-5.2 quota and flash-lite quota are independent).
	// Set for member-scoped error classes so one model's exhausted quota does
	// not take its sibling models on the same channel out of rotation.
	breakdownUntil time.Time
	breakdownClass string
	mu             sync.Mutex
}

// RecordMemberBreakdown puts this member into a cooldown of the given duration.
// Longer of (existing, new) wins so repeated failures don't shorten it.
func (m *ModelGroupMemberRuntime) RecordMemberBreakdown(class string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	until := time.Now().Add(d)
	if until.After(m.breakdownUntil) {
		m.breakdownUntil = until
		m.breakdownClass = class
	}
}

// RecordMemberSuccess clears any member-level breakdown (the member served a
// request, so its credit pool is alive again).
func (m *ModelGroupMemberRuntime) RecordMemberSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.breakdownUntil = time.Time{}
	m.breakdownClass = ""
}

// memberBreakdownActive reports whether the member is still in cooldown.
func (m *ModelGroupMemberRuntime) memberBreakdownActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.Now().Before(m.breakdownUntil)
}

// MemberBreakdownInfo returns the member-level breakdown class and expiry
// (zero time = none) for status APIs.
func (m *ModelGroupMemberRuntime) MemberBreakdownInfo() (string, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Now().After(m.breakdownUntil) {
		return "", time.Time{}
	}
	return m.breakdownClass, m.breakdownUntil
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
			Config: config.ModelGroupMember{
				Channel:          m.Channel,
				Model:            m.Model,
				Weight:           weight,
				MaxContextTokens: m.MaxContextTokens,
			},
			Channel: ch,
		})
	}

	return g, nil
}

// SelectChannel picks a channel for the given task key, respecting sticky bindings,
// health, task-type eligibility, and (for coding) tier ordering. `exclude` holds
// member keys (channel:model) already tried this request so retries advance
// deterministically and the same physical channel can be reused with a different
// model. For backward compatibility a bare channel name is also accepted.
// `balances` (channel→remaining USD) feeds the balance_aware strategy.
func (g *ModelGroup) SelectChannel(taskKey string, taskType TaskType, codingPref string, balances map[string]float64, exclude map[string]bool) (*Channel, string) {
	// 1. Check sticky binding (skip if excluded/unhealthy/member-broken)
	if taskKey != "" && g.Sticky != nil {
		if binding := g.Sticky.Get(taskKey); binding != nil {
			if !memberExcluded(exclude, binding.Channel.Config.Name, binding.Model) &&
				binding.Channel.Health.IsAvailable() &&
				!g.memberBreakdownActive(binding.Channel.Config.Name, binding.Model) {
				return binding.Channel, binding.Model
			}
			// Bound channel is unhealthy, in member breakdown, or already tried
			// — clear binding and re-select
			g.Sticky.Remove(taskKey)
		}
	}

	// 2. Filter to available, not-excluded, task-eligible members
	candidates := g.eligibleMembers(taskType, exclude)
	if len(candidates) == 0 {
		return nil, ""
	}

	// 3. Select. Coding tasks honor tier ordering (free/standard/premium) per the
	//    configured sub-strategy; other tasks use the group's base strategy.
	var selected *ModelGroupMemberRuntime
	if taskType == TaskCoding {
		selected = g.selectCodingTiered(candidates, codingPref, balances)
	} else {
		selected = g.selectByStrategy(candidates, balances)
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

// eligibleMembers returns available, non-excluded members, then applies two
// filters, each with a never-fail fallback:
//  1. task-type eligibility: a member whose non-empty TaskTypes excludes
//     taskType is dropped (so e.g. classify/summary never route to a coding-only
//     channel like kimi-code);
//  2. member-level breakdown: a member whose (per-model) quota pool is in
//     cooldown is dropped — this is what lets flash-lite stay in rotation on
//     the SenseNova channel while glm-5.2's separate credit pool is exhausted.
func (g *ModelGroup) eligibleMembers(taskType TaskType, exclude map[string]bool) []*ModelGroupMemberRuntime {
	var available []*ModelGroupMemberRuntime
	for _, m := range g.Members {
		if memberExcluded(exclude, m.Channel.Config.Name, m.Config.Model) {
			continue
		}
		if m.Channel.Health.IsAvailable() {
			available = append(available, m)
		}
	}
	var eligible []*ModelGroupMemberRuntime
	for _, m := range available {
		tt := m.Channel.Config.TaskTypes
		if len(tt) == 0 || containsString(tt, string(taskType)) {
			eligible = append(eligible, m)
		}
	}
	if len(eligible) == 0 {
		eligible = available // never fail a request purely on task tagging
	}
	var live []*ModelGroupMemberRuntime
	for _, m := range eligible {
		if !m.memberBreakdownActive() {
			live = append(live, m)
		}
	}
	if len(live) == 0 {
		return eligible // never fail a request purely on member breakdown
	}
	return live
}

// findMember returns the runtime member for a (channel, model) pair, or nil.
func (g *ModelGroup) findMember(channel, model string) *ModelGroupMemberRuntime {
	for _, m := range g.Members {
		if m.Channel.Config.Name == channel && m.Config.Model == model {
			return m
		}
	}
	return nil
}

// memberBreakdownActive reports whether the (channel, model) member is in a
// member-level cooldown. Returns false for unknown pairs.
func (g *ModelGroup) memberBreakdownActive(channel, model string) bool {
	m := g.findMember(channel, model)
	return m != nil && m.memberBreakdownActive()
}

// contextExcluded returns the set of member keys whose declared context window
// is smaller than requiredTokens (the estimated prompt + output budget).
// Returns nil when nothing needs excluding. The caller is expected to drop the
// filter if it would exclude every member (never fail purely on estimation).
func (g *ModelGroup) contextExcluded(requiredTokens int) map[string]bool {
	if requiredTokens <= 0 {
		return nil
	}
	var excluded map[string]bool
	for _, m := range g.Members {
		if m.Config.MaxContextTokens > 0 && requiredTokens > m.Config.MaxContextTokens {
			if excluded == nil {
				excluded = make(map[string]bool)
			}
			excluded[memberKey(m.Channel.Config.Name, m.Config.Model)] = true
		}
	}
	return excluded
}

// selectByStrategy applies the group's base load-balancing strategy.
func (g *ModelGroup) selectByStrategy(candidates []*ModelGroupMemberRuntime, balances map[string]float64) *ModelGroupMemberRuntime {
	switch g.Config.Strategy {
	case "best-weight":
		return bestWeightSelect(candidates)
	case "least_latency":
		return leastLatencySelect(candidates)
	case "balance_aware":
		return balanceAwareSelect(candidates, balances)
	default: // "priority-health"
		return priorityHealthSelect(candidates)
	}
}

// selectCodingTiered partitions candidates by tier and picks within the first
// non-empty tier in the order dictated by the coding sub-strategy. Within a tier
// the group's base strategy breaks ties.
func (g *ModelGroup) selectCodingTiered(candidates []*ModelGroupMemberRuntime, codingPref string, balances map[string]float64) *ModelGroupMemberRuntime {
	byTier := map[string][]*ModelGroupMemberRuntime{}
	for _, m := range candidates {
		t := memberTier(m)
		byTier[t] = append(byTier[t], m)
	}
	for _, tier := range codingTierOrder(codingPref) {
		if members := byTier[tier]; len(members) > 0 {
			return g.selectByStrategy(members, balances)
		}
	}
	// Candidates only in non-standard tiers — fall back to the whole set.
	return g.selectByStrategy(candidates, balances)
}

// memberTier resolves a member's tier, defaulting from IsFree when unset.
func memberTier(m *ModelGroupMemberRuntime) string {
	if t := m.Channel.Config.Tier; t != "" {
		return t
	}
	if m.Channel.Config.IsFree {
		return "free"
	}
	return "standard"
}

// codingTierOrder maps a coding preference to a tier visit order. free/standard/
// premium ≈ 免费 / Kimi-Code-sunk-cost / 付费. (ROADMAP §2.6.5)
func codingTierOrder(pref string) []string {
	switch pref {
	case "quality_first", "complex":
		// Kimi Code (sunk cost) → paid → free fallback
		return []string{"standard", "premium", "free"}
	default: // free_first, simple, medium
		return []string{"free", "standard", "premium"}
	}
}

// containsString reports whether s is in arr.
func containsString(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
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

// bestWeightSelect picks the candidate with the highest weight.
// Named best-weight to distinguish from round-robin (the actual behavior is
// deterministic highest-weight selection, not round-robin rotation).
func bestWeightSelect(candidates []*ModelGroupMemberRuntime) *ModelGroupMemberRuntime {
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

// balanceAwareSelect scores members by score = latency_ewma × (1 + error_rate) /
// max(remaining_usd, 1) — lower is better — using the live balance snapshot. When
// no balance is known for any candidate (provider unsupported / stale), it degrades
// to priority-health rather than guessing. (§2.3.5 / §2.6.3)
func balanceAwareSelect(candidates []*ModelGroupMemberRuntime, balances map[string]float64) *ModelGroupMemberRuntime {
	if len(candidates) == 0 {
		return nil
	}
	hasBalance := false
	for _, m := range candidates {
		if _, ok := balances[m.Channel.Config.Name]; ok {
			hasBalance = true
			break
		}
	}
	if !hasBalance {
		return priorityHealthSelect(candidates)
	}
	best := candidates[0]
	bestScore := balanceScore(best, balances)
	for _, m := range candidates[1:] {
		if s := balanceScore(m, balances); s < bestScore {
			best = m
			bestScore = s
		}
	}
	return best
}

func balanceScore(m *ModelGroupMemberRuntime, balances map[string]float64) float64 {
	latency := m.Channel.EWMALatency()
	if latency <= 0 {
		latency = 1 // no latency samples yet — neutral weight
	}
	remaining := balances[m.Channel.Config.Name]
	if remaining < 1 {
		remaining = 1 // floor so near-zero balances don't divide by ~0
	}
	return latency * (1 + m.ErrorRate()) / remaining
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
			"channel":            m.Config.Channel,
			"model":              m.Config.Model,
			"weight":             m.Config.Weight,
			"max_context_tokens": m.Config.MaxContextTokens,
			"available":          m.Channel.Health.IsAvailable(),
			"health":             m.Channel.Health.Status(),
		}
		if class, until := m.MemberBreakdownInfo(); class != "" {
			ms["member_breakdown_class"] = class
			ms["member_breakdown_until"] = until.Format(time.RFC3339)
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
