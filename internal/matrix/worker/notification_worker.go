package worker

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/service"
	"go.uber.org/zap"
)

// NotificationWorker consumes notification messages from NATS
type NotificationWorker struct {
	cfg       config.NATSConfig
	nats      *infra.NATSClient
	sender    *service.NotificationSender
	maxRetry  int
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   bool
	mu        sync.Mutex
}

// UpdateMatrixClient updates the matrix client for the sender
func (w *NotificationWorker) UpdateMatrixClient(client *gateway.Client) {
	w.sender.UpdateClient(client)
}

// NewNotificationWorker creates a new notification worker
func NewNotificationWorker(
	cfg config.NATSConfig,
	nats *infra.NATSClient,
	sender *service.NotificationSender,
	maxRetry int,
) *NotificationWorker {
	return &NotificationWorker{
		cfg:      cfg,
		nats:     nats,
		sender:   sender,
		maxRetry: maxRetry,
		stopCh:   make(chan struct{}),
	}
}

// Start begins consuming messages
func (w *NotificationWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	middleware.GetLogger().Info("starting notification worker")

	// Subscribe to notifications stream
	subject := w.cfg.Streams.Notifications + ".*" // notifications.<channel>
	durableName := "bellkeeper-notify-worker"

	sub, err := w.nats.Subscribe(subject, durableName)
	if err != nil {
		return err
	}

	w.wg.Add(1)
	go w.consumeMessages(ctx, sub)

	middleware.GetLogger().Info("notification worker started", zap.String("subject", subject))
	return nil
}

// Stop gracefully stops the worker
func (w *NotificationWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()
	middleware.GetLogger().Info("notification worker stopped")
}

// consumeMessages processes incoming messages
func (w *NotificationWorker) consumeMessages(ctx context.Context, sub *nats.Subscription) {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopCh:
			middleware.GetLogger().Info("notification worker received stop signal")
			return
		case <-ctx.Done():
			middleware.GetLogger().Info("notification worker context cancelled")
			return
		default:
			// Fetch messages with timeout
			msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
			if err != nil {
				if err == nats.ErrTimeout {
					continue // normal, no messages available
				}
				middleware.GetLogger().Warn("notification worker fetch error", zap.Error(err))
				time.Sleep(time.Second) // back off on error
				continue
			}

			for _, msg := range msgs {
				w.processMessage(ctx, msg)
			}
		}
	}
}

// processMessage handles a single notification message
func (w *NotificationWorker) processMessage(ctx context.Context, msg *nats.Msg) {
	var notification service.NotificationQueueMessage
	if err := json.Unmarshal(msg.Data, &notification); err != nil {
		middleware.GetLogger().Error("failed to unmarshal notification message", zap.Error(err))
		msg.Nak() // negative ack, will be redelivered
		return
	}

	middleware.GetLogger().Info("processing notification",
		zap.String("notification_id", notification.NotificationID),
		zap.Int("retry_count", notification.RetryCount))

	if err := w.sender.Send(ctx, &notification); err != nil {
		middleware.GetLogger().Error("failed to send notification",
			zap.String("notification_id", notification.NotificationID),
			zap.Error(err))

		if notification.RetryCount < w.maxRetry {
			// Will be retried via dead letter or re-queued
			middleware.GetLogger().Warn("notification will be retried",
				zap.String("notification_id", notification.NotificationID),
				zap.Int("attempt", notification.RetryCount+1),
				zap.Int("max", w.maxRetry))
			msg.Nak() // negative ack for redelivery
		} else {
			middleware.GetLogger().Warn("notification exceeded max retries",
				zap.String("notification_id", notification.NotificationID))
			msg.Ack() // ack to prevent infinite redelivery
		}
		return
	}

	// Success
	msg.Ack()
	middleware.GetLogger().Info("notification sent successfully",
		zap.String("notification_id", notification.NotificationID))
}
