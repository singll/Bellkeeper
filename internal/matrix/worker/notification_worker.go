package worker

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/service"
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

	log.Printf("[NotifyWorker] starting notification worker...")

	// Subscribe to notifications stream
	subject := w.cfg.Streams.Notifications + ".*" // notifications.<channel>
	durableName := "bellkeeper-notify-worker"

	sub, err := w.nats.Subscribe(subject, durableName)
	if err != nil {
		return err
	}

	w.wg.Add(1)
	go w.consumeMessages(ctx, sub)

	log.Printf("[NotifyWorker] started, consuming from %s", subject)
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
	log.Printf("[NotifyWorker] stopped")
}

// consumeMessages processes incoming messages
func (w *NotificationWorker) consumeMessages(ctx context.Context, sub *nats.Subscription) {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopCh:
			log.Printf("[NotifyWorker] received stop signal")
			return
		case <-ctx.Done():
			log.Printf("[NotifyWorker] context cancelled")
			return
		default:
			// Fetch messages with timeout
			msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
			if err != nil {
				if err == nats.ErrTimeout {
					continue // normal, no messages available
				}
				log.Printf("[NotifyWorker] fetch error: %v", err)
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
		log.Printf("[NotifyWorker] failed to unmarshal message: %v", err)
		msg.Nak() // negative ack, will be redelivered
		return
	}

	log.Printf("[NotifyWorker] processing notification %s (retry: %d)", notification.NotificationID, notification.RetryCount)

	if err := w.sender.Send(ctx, &notification); err != nil {
		log.Printf("[NotifyWorker] failed to send notification %s: %v", notification.NotificationID, err)

		if notification.RetryCount < w.maxRetry {
			// Will be retried via dead letter or re-queued
			log.Printf("[NotifyWorker] notification %s will be retried (attempt %d/%d)",
				notification.NotificationID, notification.RetryCount+1, w.maxRetry)
			msg.Nak() // negative ack for redelivery
		} else {
			log.Printf("[NotifyWorker] notification %s exceeded max retries, marking as failed", notification.NotificationID)
			msg.Ack() // ack to prevent infinite redelivery
		}
		return
	}

	// Success
	msg.Ack()
	log.Printf("[NotifyWorker] successfully sent notification %s", notification.NotificationID)
}
