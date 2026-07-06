package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/eventbus"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// NotificationService handles notification dispatch
type NotificationService struct {
	cfg          config.NATSConfig
	redis        *infra.RedisClient
	bus          *eventbus.Client
	repos        *repository.Repositories
	channelsMu   sync.RWMutex
	channels     map[string]*model.MatrixChannel // cached channel config

	// Aggregation
	aggMu      sync.Mutex
	aggBuf     map[string]*aggEntry // dedup_key -> aggregated entry
	aggStopCh  chan struct{}
	aggWg      sync.WaitGroup
}

type aggEntry struct {
	Count     int
	FirstMsg  string
	Channel   string
	RoomID    string
	Severity  string
	LastSeen  time.Time
}

// NotificationRequest represents a notification API request
type NotificationRequest struct {
	Channel     string            `json:"channel" binding:"required"` // alerts, daily, todo, qa
	Message     string            `json:"message" binding:"required"`
	MessageType string            `json:"message_type"` // text, html, markdown (default: text)
	RoomID      string            `json:"room_id"`      // optional, overrides channel room
	Metadata    map[string]string `json:"metadata"`     // optional metadata
	ID          string            `json:"id"`           // idempotency key, auto-generated if empty
	DedupKey    string            `json:"dedup_key"`    // deduplication key for suppression
	Severity    string            `json:"severity"`     // severity level: critical, warning, info
}

// NotificationResponse represents the API response
type NotificationResponse struct {
	Success        bool   `json:"success"`
	NotificationID string `json:"notification_id,omitempty"`
	Message        string `json:"message,omitempty"`
	RetryCount     int    `json:"retry_count,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	cfg config.NATSConfig,
	redis *infra.RedisClient,
	bus *eventbus.Client,
	repos *repository.Repositories,
) *NotificationService {
	svc := &NotificationService{
		cfg:       cfg,
		redis:     redis,
		bus:       bus,
		repos:     repos,
		channels:  make(map[string]*model.MatrixChannel),
		aggBuf:    make(map[string]*aggEntry),
		aggStopCh: make(chan struct{}),
	}

	// Load channel config
	svc.loadChannels(context.Background())

	return svc
}

// loadChannels loads channel configuration from database
func (s *NotificationService) loadChannels(ctx context.Context) {
	channels, err := s.repos.MatrixChannel.GetAllActive(ctx)
	if err != nil {
		log.Printf("[Notify] failed to load channels: %v", err)
		return
	}

	next := make(map[string]*model.MatrixChannel, len(channels))
	for _, ch := range channels {
		next[ch.ChannelName] = ch
		log.Printf("[Notify] loaded channel: %s -> %s", ch.ChannelName, ch.RoomID)
	}

	s.channelsMu.Lock()
	s.channels = next
	s.channelsMu.Unlock()
}

// ReloadChannels reloads channel configuration
func (s *NotificationService) ReloadChannels(ctx context.Context) {
	s.loadChannels(ctx)
}

// Send queues a notification for async delivery
func (s *NotificationService) Send(ctx context.Context, req *NotificationRequest) (*NotificationResponse, error) {
	// Generate idempotency key if not provided
	if req.ID == "" {
		hash := sha256.Sum256([]byte(req.Channel + req.Message + time.Now().Format(time.RFC3339Nano)))
		req.ID = hex.EncodeToString(hash[:16])
	}

	// Check rate limit (频道级)
	if err := s.checkRateLimit(ctx, req.Channel); err != nil {
		return &NotificationResponse{
			Success: false,
			Message: fmt.Sprintf("rate limited: %v", err),
		}, nil
	}

	// Route to channel or specific room
	var roomID string
	if req.RoomID != "" {
		roomID = req.RoomID
	} else if channel := s.getChannel(req.Channel); channel != nil {
		roomID = channel.RoomID
	} else {
		return &NotificationResponse{
			Success: false,
			Message: fmt.Sprintf("unknown channel: %s", req.Channel),
		}, nil
	}

	// Guard against empty RoomID — silent failure is worse than an explicit error
	if roomID == "" {
		envHint := fmt.Sprintf("BELLKEEPER_MATRIX_ROOM_%s", req.Channel)
		return &NotificationResponse{
			Success: false,
			Message: fmt.Sprintf("channel '%s' has no RoomID configured - set via admin API or %s env var", req.Channel, envHint),
		}, nil
	}

	// Set default message type
	if req.MessageType == "" {
		req.MessageType = "text"
	}

	// Dedup suppression: if DedupKey is set and Severity is not "critical", check Redis
	if req.DedupKey != "" && req.Severity != "critical" {
		dedupKey := fmt.Sprintf("notify:dedup:%s", req.DedupKey)
		exists, err := s.redis.IncrRateLimit(ctx, dedupKey, time.Hour)
		if err != nil {
			log.Printf("[Notify] dedup check failed: %v", err)
		} else if exists > 1 {
			// Aggregate: bump count for summary
			s.aggMu.Lock()
			if entry, ok := s.aggBuf[req.DedupKey]; ok {
				entry.Count++
				entry.LastSeen = time.Now()
			} else {
				s.aggBuf[req.DedupKey] = &aggEntry{
					Count:    1,
					FirstMsg: req.Message,
					Channel:  req.Channel,
					RoomID:   roomID,
					Severity: req.Severity,
					LastSeen: time.Now(),
				}
			}
			s.aggMu.Unlock()

			log.Printf("[Notify] suppressed duplicate (dedup_key=%s, count=%d)", req.DedupKey, exists)
			return &NotificationResponse{
				Success: true,
				Message: "suppressed duplicate",
			}, nil
		}
	}

	// Create notification record
	metadataJSON, _ := json.Marshal(req.Metadata)
	metadataStr := string(metadataJSON)
	notification := &model.MatrixNotification{
		NotificationID: req.ID,
		ChannelName:    req.Channel,
		RoomID:         roomID,
		MessageType:    req.MessageType,
		MessageContent: req.Message,
		Metadata:       &metadataStr,
		Status:         "pending",
		DedupKey:       req.DedupKey,
		Severity:       req.Severity,
	}

	// Store in DB for audit
	if err := s.repos.MatrixNotification.Create(ctx, notification); err != nil {
		log.Printf("[Notify] failed to create notification record: %v", err)
		// Continue anyway, queue it
	}

	// Publish to NATS queue
	queueMsg := &NotificationQueueMessage{
		NotificationID: req.ID,
		RoomID:         roomID,
		Message:        req.Message,
		MessageType:    req.MessageType,
		RetryCount:     0,
		DedupKey:       req.DedupKey,
		Severity:       req.Severity,
	}

	msgBytes, err := json.Marshal(queueMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal queue message: %w", err)
	}

	subject := fmt.Sprintf("%s.%s", s.cfg.Streams.Notifications, req.Channel)
	if err := s.bus.Publish(subject, msgBytes); err != nil {
		if err := s.repos.MatrixNotification.UpdateStatus(ctx, req.ID, "failed", err.Error()); err != nil {
			log.Printf("[Notify] failed to update notification %s status to failed: %v", req.ID, err)
		}
		return nil, fmt.Errorf("failed to publish to queue: %w", err)
	}

	log.Printf("[Notify] queued notification %s to channel %s", req.ID, req.Channel)

	return &NotificationResponse{
		Success:        true,
		NotificationID: req.ID,
		Message:        "queued for delivery",
	}, nil
}

// checkRateLimit checks if the channel is rate limited
func (s *NotificationService) checkRateLimit(ctx context.Context, channel string) error {
	// 频道级限流：每分钟最多 20 条
	key := fmt.Sprintf("notify:%s", channel)
	count, err := s.redis.IncrRateLimit(ctx, key, time.Minute)
	if err != nil {
		log.Printf("[Notify] failed to check rate limit: %v", err)
		return nil // 失败时放行
	}

	if count > 20 {
		return fmt.Errorf("exceeded 20 messages per minute")
	}

	return nil
}

// GetStatus returns notification status
func (s *NotificationService) GetStatus(ctx context.Context, id string) (*model.MatrixNotification, error) {
	return s.repos.MatrixNotification.GetByNotificationID(ctx, id)
}

// GetChannels returns all configured channels
func (s *NotificationService) GetChannels(ctx context.Context) []*model.MatrixChannel {
	s.channelsMu.RLock()
	defer s.channelsMu.RUnlock()

	result := make([]*model.MatrixChannel, 0, len(s.channels))
	for _, ch := range s.channels {
		result = append(result, ch)
	}
	return result
}

func (s *NotificationService) getChannel(name string) *model.MatrixChannel {
	s.channelsMu.RLock()
	defer s.channelsMu.RUnlock()
	return s.channels[name]
}

// NotificationQueueMessage represents a message in the NATS queue
type NotificationQueueMessage struct {
	NotificationID string `json:"notification_id"`
	RoomID         string `json:"room_id"`
	Message        string `json:"message"`
	MessageType    string `json:"message_type"`
	RetryCount     int    `json:"retry_count"`
	DedupKey       string `json:"dedup_key,omitempty"`
	Severity       string `json:"severity,omitempty"`
}

// Start begins the aggregation flush loop
func (s *NotificationService) Start() {
	s.aggWg.Add(1)
	go s.aggFlushLoop()
	log.Printf("[Notify] aggregation loop started")
}

// Stop stops the aggregation flush loop
func (s *NotificationService) Stop() {
	close(s.aggStopCh)
	s.aggWg.Wait()
	s.flushAggregation()
	log.Printf("[Notify] aggregation loop stopped")
}

func (s *NotificationService) aggFlushLoop() {
	defer s.aggWg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.flushAggregation()
		case <-s.aggStopCh:
			return
		}
	}
}

func (s *NotificationService) flushAggregation() {
	s.aggMu.Lock()
	entries := make(map[string]*aggEntry, len(s.aggBuf))
	for k, v := range s.aggBuf {
		entries[k] = v
	}
	s.aggBuf = make(map[string]*aggEntry)
	s.aggMu.Unlock()

	for dedupKey, entry := range entries {
		if entry.Count <= 1 {
			continue
		}
		summaryMsg := fmt.Sprintf("%s\n\n(同类通知 ×%d，已合并)", entry.FirstMsg, entry.Count)
		queueMsg := &NotificationQueueMessage{
			NotificationID: fmt.Sprintf("agg:%s:%d", dedupKey, time.Now().UnixNano()),
			RoomID:         entry.RoomID,
			Message:        summaryMsg,
			MessageType:    "text",
			RetryCount:     0,
			DedupKey:       dedupKey,
			Severity:       entry.Severity,
		}
		msgBytes, err := json.Marshal(queueMsg)
		if err != nil {
			log.Printf("[Notify] failed to marshal aggregation summary: %v", err)
			continue
		}
		subject := fmt.Sprintf("%s.%s", s.cfg.Streams.Notifications, entry.Channel)
		if err := s.bus.Publish(subject, msgBytes); err != nil {
			log.Printf("[Notify] failed to publish aggregation summary: %v", err)
		} else {
			log.Printf("[Notify] sent aggregation summary for dedup_key=%s (×%d)", dedupKey, entry.Count)
		}
	}
}
