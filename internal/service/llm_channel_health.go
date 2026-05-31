package service

import (
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

// --- Circuit Breaker States ---

type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal: accepting requests
	CircuitOpen                         // Tripped: rejecting requests
	CircuitHalfOpen                     // Probing: allowing limited requests
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// --- Request Result ---

type requestResult struct {
	at        time.Time
	success   bool
	errorType string // "", "429", "5xx", "timeout", "error"
}

// --- Channel Health ---

// ChannelHealth tracks the health of a single upstream channel using a sliding
// window of recent results and a circuit breaker state machine.
type ChannelHealth struct {
	mu  sync.Mutex
	cfg config.CircuitBreakerConfig

	// Sliding window (ring buffer)
	results    []requestResult
	windowSize int
	writeIdx   int
	count      int

	// Circuit breaker state
	state            CircuitState
	consecutiveFails int
	circuitOpenUntil time.Time
	halfOpenInFlight int

	// Semantic breakdown (Iteration 4)
	breakdownUntil   time.Time
	breakdownReason  string
	breakdownClass   string

	// Timestamps
	lastSuccessAt time.Time
	lastErrorAt   time.Time
	lastErrorType string
}

// NewChannelHealth creates a new health tracker with the given circuit breaker config.
func NewChannelHealth(cfg config.CircuitBreakerConfig) *ChannelHealth {
	windowSize := 50
	return &ChannelHealth{
		cfg:        cfg,
		results:    make([]requestResult, windowSize),
		windowSize: windowSize,
	}
}

// IsAvailable returns true if the channel is accepting new requests.
func (h *ChannelHealth) IsAvailable() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch h.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Now().After(h.circuitOpenUntil) {
			h.state = CircuitHalfOpen
			h.halfOpenInFlight = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return h.halfOpenInFlight < h.cfg.HalfOpenMax
	default:
		return true
	}
}

// RecordSuccess records a successful request and potentially closes the circuit.
func (h *ChannelHealth) RecordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.appendResult(requestResult{at: time.Now(), success: true})
	h.consecutiveFails = 0
	h.lastSuccessAt = time.Now()

	if h.state == CircuitHalfOpen {
		h.state = CircuitClosed
		h.halfOpenInFlight = 0
	}
}

// RecordFailure records a failed request and potentially opens the circuit.
// Returns true if the circuit was tripped open.
func (h *ChannelHealth) RecordFailure(errorType string) bool {
	return h.recordFailureLocked(errorType, "", 0)
}

// RecordClassifiedFailure records a classified failure with semantic breakdown.
// class: semantic error class (e.g. "quota_exhausted", "rate_limited_retry")
// breakdownDuration: 0 = use default cooldown from config.
func (h *ChannelHealth) RecordClassifiedFailure(errorType, class string, breakdownDuration time.Duration) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.recordFailureLocked(errorType, class, breakdownDuration)
}

func (h *ChannelHealth) recordFailureLocked(errorType, class string, breakdownDuration time.Duration) bool {
	h.appendResult(requestResult{at: time.Now(), success: false, errorType: errorType})
	h.consecutiveFails++
	h.lastErrorAt = time.Now()
	h.lastErrorType = errorType

	if h.state == CircuitHalfOpen {
		h.tripOpenLocked(class, breakdownDuration)
		return true
	}

	if h.consecutiveFails >= h.cfg.FailureThreshold {
		h.tripOpenLocked(class, breakdownDuration)
		return true
	}

	return false
}

// tripOpen transitions the circuit to Open state with optional semantic duration.
func (h *ChannelHealth) tripOpen() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tripOpenLocked("", 0)
}

func (h *ChannelHealth) tripOpenLocked(class string, breakdownDuration time.Duration) {
	cooldown := time.Duration(h.cfg.CooldownSeconds) * time.Second
	if breakdownDuration > 0 {
		cooldown = breakdownDuration
	}
	h.state = CircuitOpen
	h.circuitOpenUntil = time.Now().Add(cooldown)
	h.breakdownUntil = time.Now().Add(cooldown)
	h.breakdownReason = class
	h.breakdownClass = class
	h.halfOpenInFlight = 0
}

// HealthScore returns a score from 0.0 to 1.0 based on the recent success rate
// within the sliding window.
func (h *ChannelHealth) HealthScore() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.count == 0 {
		return 1.0 // No data yet, assume healthy
	}

	now := time.Now()
	windowDuration := time.Duration(h.cfg.ErrorWindowSeconds) * time.Second
	cutoff := now.Add(-windowDuration)

	successes := 0
	total := 0
	for i := 0; i < h.count; i++ {
		idx := (h.writeIdx - 1 - i + h.windowSize) % h.windowSize
		r := h.results[idx]
		if r.at.Before(cutoff) {
			break
		}
		total++
		if r.success {
			successes++
		}
	}

	if total == 0 {
		return 1.0
	}
	return float64(successes) / float64(total)
}

// Reset clears the circuit breaker state back to closed.
func (h *ChannelHealth) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.state = CircuitClosed
	h.consecutiveFails = 0
	h.circuitOpenUntil = time.Time{}
	h.halfOpenInFlight = 0
}

// BreakdownInfo returns the circuit state plus the semantic breakdown class and the
// time the current breakdown lasts until. The Kimi Code probe loop uses it to find
// channels stuck in a long quota_exhausted breakdown that are due for a recovery
// probe (§2.6.4). Read-only: it does not transition the circuit (unlike IsAvailable).
func (h *ChannelHealth) BreakdownInfo() (state CircuitState, class string, until time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state, h.breakdownClass, h.breakdownUntil
}

// TrackHalfOpenRequest increments the half-open in-flight counter.
// Call this before sending a request when in half-open state.
func (h *ChannelHealth) TrackHalfOpenRequest() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == CircuitHalfOpen {
		h.halfOpenInFlight++
	}
}

// Status returns a snapshot of the current health state for management APIs.
func (h *ChannelHealth) Status() map[string]interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	status := map[string]interface{}{
		"state":             h.state.String(),
		"consecutive_fails": h.consecutiveFails,
		"last_error_type":   h.lastErrorType,
		"breakdown_reason":  h.breakdownReason,
		"breakdown_class":   h.breakdownClass,
	}

	// Calculate recent success rate
	if h.count > 0 {
		now := time.Now()
		windowDuration := time.Duration(h.cfg.ErrorWindowSeconds) * time.Second
		cutoff := now.Add(-windowDuration)
		successes, total := 0, 0
		for i := 0; i < h.count; i++ {
			idx := (h.writeIdx - 1 - i + h.windowSize) % h.windowSize
			r := h.results[idx]
			if r.at.Before(cutoff) {
				break
			}
			total++
			if r.success {
				successes++
			}
		}
		if total > 0 {
			status["recent_success_rate"] = float64(successes) / float64(total)
		} else {
			status["recent_success_rate"] = 1.0
		}
	} else {
		status["recent_success_rate"] = 1.0
	}

	if !h.lastSuccessAt.IsZero() {
		status["last_success_at"] = h.lastSuccessAt.Format(time.RFC3339)
	}
	if !h.lastErrorAt.IsZero() {
		status["last_error_at"] = h.lastErrorAt.Format(time.RFC3339)
	}
	if h.state == CircuitOpen && !h.circuitOpenUntil.IsZero() {
		status["circuit_open_until"] = h.circuitOpenUntil.Format(time.RFC3339)
	}

	return status
}

// appendResult adds a result to the ring buffer.
func (h *ChannelHealth) appendResult(r requestResult) {
	h.results[h.writeIdx] = r
	h.writeIdx = (h.writeIdx + 1) % h.windowSize
	if h.count < h.windowSize {
		h.count++
	}
}

// classifyError maps HTTP status codes and errors to error type strings.
func classifyError(statusCode int, err error) string {
	if err != nil {
		return "timeout"
	}
	if statusCode == 429 {
		return "429"
	}
	if statusCode >= 500 {
		return "5xx"
	}
	return "error"
}
