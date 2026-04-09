package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// NotificationService handles notification dispatch
type NotificationService struct {
	cfg          config.NATSConfig
	redis        *infra.RedisClient
	nats         *infra.NATSClient
	repos        *repository.Repositories
	channels     map[string]*model.MatrixChannel // cached channel config
	notifySender *NotificationSender
}

// NotificationRequest represents a notification API request
type NotificationRequest struct {
	Channel    string            `json:"channel" binding:"required"` // alerts, daily, todo, qa
	Message    string            `json:"message" binding:"required"`
	MessageType string           `json:"message_type"` // text, html, markdown (default: text)
	RoomID     string            `json:"room_id"`       // optional, overrides channel room
	Metadata   map[string]string `json:"metadata"`     // optional metadata
	ID         string            `json:"id"`           // idempotency key, auto-generated if empty
}

// NotificationResponse represents the API response
type NotificationResponse struct {
	Success         bool   `json:"success"`
	NotificationID  string `json:"notification_id,omitempty"`
	Message         string `json:"message,omitempty"`
	RetryCount      int    `json:"retry_count,omitempty"`
	LastError       string `json:"last_error,omitempty"`
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	cfg config.NATSConfig,
	redis *infra.RedisClient,
	nats *infra.NATSClient,
	repos *repository.Repositories,
) *NotificationService {
	svc := &NotificationService{
		cfg:       cfg,
		redis:     redis,
		nats:      nats,
		repos:     repos,
		channels:  make(map[string]*model.MatrixChannel),
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

	for _, ch := range channels {
		s.channels[ch.ChannelName] = ch
		log.Printf("[Notify] loaded channel: %s -> %s", ch.ChannelName, ch.RoomID)
	}
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
	} else if channel, ok := s.channels[req.Channel]; ok {
		roomID = channel.RoomID
	} else {
		return &NotificationResponse{
			Success: false,
			Message: fmt.Sprintf("unknown channel: %s", req.Channel),
		}, nil
	}

	// Set default message type
	if req.MessageType == "" {
		req.MessageType = "text"
	}

	// Create notification record
	metadataJSON, _ := json.Marshal(req.Metadata)
	notification := &model.MatrixNotification{
		NotificationID: req.ID,
		ChannelName:    req.Channel,
		RoomID:         roomID,
		MessageType:    req.MessageType,
		MessageContent: req.Message,
		Metadata:       string(metadataJSON),
		Status:         "pending",
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
	}

	msgBytes, err := json.Marshal(queueMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal queue message: %w", err)
	}

	subject := fmt.Sprintf("%s.%s", s.cfg.Streams.Notifications, req.Channel)
	if err := s.nats.Publish(subject, msgBytes); err != nil {
		s.repos.MatrixNotification.UpdateStatus(ctx, req.ID, "failed", err.Error())
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
	result := make([]*model.MatrixChannel, 0, len(s.channels))
	for _, ch := range s.channels {
		result = append(result, ch)
	}
	return result
}

// NotificationQueueMessage represents a message in the NATS queue
type NotificationQueueMessage struct {
	NotificationID string `json:"notification_id"`
	RoomID         string `json:"room_id"`
	Message        string `json:"message"`
	MessageType    string `json:"message_type"`
	RetryCount     int    `json:"retry_count"`
}
