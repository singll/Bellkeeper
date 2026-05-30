package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// AlertNotifier is the interface for sending aggregated alerts to a destination.
type AlertNotifier interface {
	Notify(message string) error
}

// AlertAggregator buffers LLM alert events and flushes them periodically
// as aggregated notifications to avoid message bombing.
type AlertAggregator struct {
	mu        sync.Mutex
	repo      *repository.LLMProxyRepository
	notifier  AlertNotifier
	buffer    []model.LLMAlertEvent
	flushInterval time.Duration
	dedupWindow   time.Duration
	lastSent      map[string]time.Time // dedup_key -> last sent time
	stopCh        chan struct{}
	running       bool
}

// NewAlertAggregator creates a new aggregator with the given notifier.
// If notifier is nil, alerts are only persisted to DB (no real-time delivery).
func NewAlertAggregator(repo *repository.LLMProxyRepository, notifier AlertNotifier) *AlertAggregator {
	return &AlertAggregator{
		repo:          repo,
		notifier:      notifier,
		buffer:        make([]model.LLMAlertEvent, 0),
		flushInterval: 5 * time.Minute,
		dedupWindow:   1 * time.Hour,
		lastSent:      make(map[string]time.Time),
		stopCh:        make(chan struct{}),
	}
}

// Start begins the background flush loop.
func (a *AlertAggregator) Start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return
	}
	a.running = true
	go a.loop()
}

// Stop halts the background loop.
func (a *AlertAggregator) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return
	}
	close(a.stopCh)
	a.running = false
}

func (a *AlertAggregator) loop() {
	ticker := time.NewTicker(a.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.Flush()
		case <-a.stopCh:
			return
		}
	}
}

// Record buffers a new alert event. It performs immediate dedup checks.
func (a *AlertAggregator) Record(alertType, severity, channelName, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dedupKey := fmt.Sprintf("%s:%s:%s", alertType, channelName, severity)

	// Skip if same dedup key was sent recently
	if last, ok := a.lastSent[dedupKey]; ok && time.Since(last) < a.dedupWindow {
		return
	}

	event := model.LLMAlertEvent{
		AlertType:   alertType,
		Severity:    severity,
		ChannelName: channelName,
		Message:     message,
		DedupKey:    dedupKey,
	}

	a.buffer = append(a.buffer, event)

	// Immediate flush for critical events
	if severity == "critical" {
		go a.Flush()
	}
}

// Flush sends aggregated alerts and clears the buffer.
func (a *AlertAggregator) Flush() {
	a.mu.Lock()
	if len(a.buffer) == 0 {
		a.mu.Unlock()
		return
	}
	batch := make([]model.LLMAlertEvent, len(a.buffer))
	copy(batch, a.buffer)
	a.buffer = a.buffer[:0]
	a.mu.Unlock()

	// Persist all events to DB
	for _, ev := range batch {
		now := time.Now()
		ev.FlushedAt = &now
		if err := a.repo.SaveAlertEvent(&ev); err != nil {
			middleware.GetLogger().Warn("failed to persist alert event", zap.Error(err))
		}
	}

	// Only notify for error+ severity
	var notifyEvents []model.LLMAlertEvent
	for _, ev := range batch {
		if ev.Severity == "error" || ev.Severity == "critical" {
			notifyEvents = append(notifyEvents, ev)
		}
	}

	if len(notifyEvents) == 0 || a.notifier == nil {
		return
	}

	// Build aggregated message
	msg := a.renderAggregation(notifyEvents)
	if err := a.notifier.Notify(msg); err != nil {
		middleware.GetLogger().Warn("failed to send aggregated alert", zap.Error(err))
	} else {
		// Update dedup timestamps on successful send
		a.mu.Lock()
		for _, ev := range notifyEvents {
			a.lastSent[ev.DedupKey] = time.Now()
		}
		a.mu.Unlock()
	}
}

func (a *AlertAggregator) renderAggregation(events []model.LLMAlertEvent) string {
	if len(events) == 1 {
		ev := events[0]
		return fmt.Sprintf("⚠️ LLM Proxy [%s] %s: %s", ev.Severity, ev.ChannelName, ev.Message)
	}

	msg := fmt.Sprintf("⚠️ LLM Proxy %d 个事件:\n", len(events))
	for _, ev := range events {
		msg += fmt.Sprintf("• [%s] %s: %s\n", ev.Severity, ev.ChannelName, ev.Message)
	}
	return msg
}
