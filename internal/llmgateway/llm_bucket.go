package llmgateway

import (
	"fmt"
	"sync"
	"time"
)

// --- Token Bucket Rate Limiter ---

// TokenBucket implements a token-bucket rate limiter with daily counter.
// It proactively limits request rate on the client side to avoid triggering
// upstream 429 responses.
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time

	dailyCount int
	dailyLimit int
	dayStart   time.Time

	// quotaWindow, when > 0, switches the long-window counter from
	// calendar-day reset to a rolling window of this length (e.g. 5h for
	// SenseNova / OpenCode Go rolling quota windows). Events are kept in a
	// timestamp ring and expired lazily on access. 0 = calendar-day behavior.
	quotaWindow  time.Duration
	windowEvents []time.Time
}

func NewTokenBucket(rpm, rpd, defaultBucketRPM int, quotaWindow ...time.Duration) *TokenBucket {
	maxTokens := float64(rpm)
	if maxTokens == 0 {
		maxTokens = float64(defaultBucketRPM)
	}
	refillRate := maxTokens / 60.0

	tb := &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
		dailyLimit: rpd,
		dayStart:   startOfDay(time.Now()),
	}
	if len(quotaWindow) > 0 && quotaWindow[0] > 0 {
		tb.quotaWindow = quotaWindow[0]
	}
	return tb
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// TryAcquire attempts to consume one token without blocking.
// Returns (allowed, suggestedWaitTime).
func (tb *TokenBucket) TryAcquire() (bool, time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// Reset daily counter at midnight / expire rolling-window events
	if tb.quotaWindow > 0 {
		cutoff := now.Add(-tb.quotaWindow)
		kept := tb.windowEvents[:0]
		for _, t := range tb.windowEvents {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		tb.windowEvents = kept
		tb.dailyCount = len(tb.windowEvents)
	} else {
		today := startOfDay(now)
		if today.After(tb.dayStart) {
			tb.dailyCount = 0
			tb.dayStart = today
		}
	}

	// Check daily limit
	if tb.dailyLimit > 0 && tb.dailyCount >= tb.dailyLimit {
		if tb.quotaWindow > 0 {
			// Rolling window: wait until the oldest in-window event expires.
			if len(tb.windowEvents) > 0 {
				wait := tb.windowEvents[0].Add(tb.quotaWindow).Sub(now)
				if wait < 0 {
					wait = 0
				}
				return false, wait
			}
			return false, 0
		}
		nextDay := tb.dayStart.Add(24 * time.Hour)
		return false, nextDay.Sub(now)
	}

	// Refill tokens
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		tb.dailyCount++
		if tb.quotaWindow > 0 {
			tb.windowEvents = append(tb.windowEvents, now)
		}
		return true, 0
	}

	wait := time.Duration((1.0 - tb.tokens) / tb.refillRate * float64(time.Second))
	return false, wait
}

// Status returns current bucket state without consuming tokens.
func (tb *TokenBucket) Status() map[string]interface{} {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	currentTokens := tb.tokens + elapsed*tb.refillRate
	if currentTokens > tb.maxTokens {
		currentTokens = tb.maxTokens
	}

	dailyCount := tb.dailyCount
	if tb.quotaWindow > 0 {
		cutoff := now.Add(-tb.quotaWindow)
		n := 0
		for _, t := range tb.windowEvents {
			if t.After(cutoff) {
				n++
			}
		}
		dailyCount = n
	}

	return map[string]interface{}{
		"available_tokens":  int(currentTokens),
		"max_tokens":        int(tb.maxTokens),
		"daily_used":        dailyCount,
		"daily_limit":       tb.dailyLimit,
		"refill_rate_per_s": fmt.Sprintf("%.2f", tb.refillRate),
		"window_seconds":    int(tb.quotaWindow / time.Second),
	}
}

// Resize adjusts the bucket's RPM (max tokens + refill rate) while preserving the
// current token level and daily counter. Used when adaptive rate-limit learning
// updates the safe RPM and the config is reloaded.
func (tb *TokenBucket) Resize(rpm, defaultBucketRPM int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	maxTokens := float64(rpm)
	if maxTokens == 0 {
		maxTokens = float64(defaultBucketRPM)
	}
	tb.maxTokens = maxTokens
	tb.refillRate = maxTokens / 60.0
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
}

// SetQuotaWindow switches the long-window counter between calendar-day and
// rolling-window mode, preserving the current count: calendar→rolling seeds
// synthetic events spread over the last window (approximate, avoids both
// over- and under-counting); rolling→calendar just adopts the live count.
func (tb *TokenBucket) SetQuotaWindow(d time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if d == tb.quotaWindow {
		return
	}
	if tb.quotaWindow == 0 && d > 0 {
		// Calendar → rolling: synthesize the last min(dailyCount, dailyLimit)
		// events evenly across the tail half of the window.
		n := tb.dailyCount
		if tb.dailyLimit > 0 && n > tb.dailyLimit {
			n = tb.dailyLimit
		}
		tb.windowEvents = tb.windowEvents[:0]
		now := time.Now()
		for i := 0; i < n; i++ {
			offset := time.Duration(int64(d) / 2 * int64(i+1) / int64(n+1))
			tb.windowEvents = append(tb.windowEvents, now.Add(-offset))
		}
	} else if tb.quotaWindow > 0 && d == 0 {
		// Rolling → calendar: adopt the in-window count; next midnight resets.
		cutoff := time.Now().Add(-tb.quotaWindow)
		n := 0
		for _, t := range tb.windowEvents {
			if t.After(cutoff) {
				n++
			}
		}
		tb.dailyCount = n
		tb.windowEvents = nil
	}
	tb.quotaWindow = d
}
