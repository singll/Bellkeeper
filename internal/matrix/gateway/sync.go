package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"

	"github.com/singll/bellkeeper/internal/model"
)

// SyncLoop manages the Matrix sync loop
type SyncLoop struct {
	client         *Client
	syncer         *mautrix.DefaultSyncer
	stopCh         chan struct{}
	stopped        bool
	commandService interface {
		ExecuteMessage(ctx context.Context, roomID, sender, eventID, content string) error
	}
}

// NewSyncLoop creates a new sync loop
func NewSyncLoop(client *Client) *SyncLoop {
	syncer := client.client.Syncer.(*mautrix.DefaultSyncer)

	loop := &SyncLoop{
		client: client,
		syncer: syncer,
		stopCh: make(chan struct{}),
	}

	// Register event handlers
	loop.registerHandlers()

	return loop
}

// SetCommandService sets the command service for processing commands
func (s *SyncLoop) SetCommandService(svc interface {
	ExecuteMessage(ctx context.Context, roomID, sender, eventID, content string) error
}) {
	s.commandService = svc
}

// registerHandlers registers Matrix event handlers
func (s *SyncLoop) registerHandlers() {
	// Handle room messages
	s.syncer.OnEventType(event.EventMessage, func(ctx context.Context, evt *event.Event) {
		if err := s.handleRoomMessage(ctx, evt); err != nil {
			log.Printf("[Matrix] error handling message: %v", err)
		}
	})

	// Handle room member events (invites, joins, leaves)
	s.syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		if err := s.handleMemberEvent(ctx, evt); err != nil {
			log.Printf("[Matrix] error handling member event: %v", err)
		}
	})
}

// Start starts the sync loop
func (s *SyncLoop) Start(ctx context.Context) error {
	// Load sync token from Redis
	token, err := s.client.redis.GetSyncToken(ctx, s.client.config.BotUserID)
	if err != nil {
		return fmt.Errorf("failed to load sync token: %w", err)
	}

	if token != "" {
		log.Printf("[Matrix] resuming sync from token: %s...", token[:min(len(token), 20)])
		s.client.client.Store.SaveNextBatch(context.Background(), s.client.client.UserID, token)
	} else {
		log.Printf("[Matrix] starting fresh sync (no previous token)")
	}

	// Also try to load from database
	syncState, err := s.client.repos.MatrixSyncState.GetByUserID(ctx, s.client.config.BotUserID)
	if err == nil && syncState != nil && syncState.NextBatch != "" {
		log.Printf("[Matrix] found DB sync token: %s...", syncState.NextBatch[:min(len(syncState.NextBatch), 20)])
		s.client.client.Store.SaveNextBatch(context.Background(), s.client.client.UserID, syncState.NextBatch)
	}

	// Start sync in background
	go func() {
		log.Printf("[Matrix] starting sync loop")
		if err := s.client.client.Sync(); err != nil {
			log.Printf("[Matrix] sync stopped: %v", err)
		}
	}()

	// Start token persistence goroutine
	go s.persistTokenPeriodically(ctx)

	return nil
}

// Stop stops the sync loop
func (s *SyncLoop) Stop() {
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.stopCh)
	s.client.client.StopSync()
	log.Printf("[Matrix] sync loop stopped")
}

// persistTokenPeriodically saves sync token to Redis and DB every 30 seconds
func (s *SyncLoop) persistTokenPeriodically(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.persistToken(ctx); err != nil {
				log.Printf("[Matrix] failed to persist sync token: %v", err)
			}
		case <-s.stopCh:
			// Final save before exit
			if err := s.persistToken(ctx); err != nil {
				log.Printf("[Matrix] failed to persist sync token on shutdown: %v", err)
			}
			return
		}
	}
}

// persistToken saves the current sync token to Redis and DB
func (s *SyncLoop) persistToken(ctx context.Context) error {
	token, err := s.client.client.Store.LoadNextBatch(ctx, s.client.client.UserID)
	if err != nil || token == "" {
		return nil // No token to save yet
	}

	// Save to Redis (hot storage)
	if err := s.client.redis.SetSyncToken(ctx, s.client.config.BotUserID, token); err != nil {
		return fmt.Errorf("failed to save to Redis: %w", err)
	}

	// Save to DB (cold storage)
	syncState := &model.MatrixSyncState{
		BotUserID:  s.client.config.BotUserID,
		NextBatch:  token,
		LastSyncAt: time.Now(),
	}

	if err := s.client.repos.MatrixSyncState.Upsert(ctx, syncState); err != nil {
		return fmt.Errorf("failed to save to DB: %w", err)
	}

	return nil
}

// handleRoomMessage processes incoming room messages
func (s *SyncLoop) handleRoomMessage(ctx context.Context, evt *event.Event) error {
	// Skip messages from the bot itself
	if evt.Sender.String() == s.client.config.BotUserID {
		return nil
	}

	// Check if event already processed (deduplication)
	processed, err := s.client.redis.CheckEventProcessed(ctx, evt.ID.String())
	if err != nil {
		return fmt.Errorf("failed to check event processed: %w", err)
	}
	if processed {
		log.Printf("[Matrix] skipping already processed event: %s", evt.ID)
		return nil
	}

	// Mark as processed
	if err := s.client.redis.MarkEventProcessed(ctx, evt.ID.String()); err != nil {
		return fmt.Errorf("failed to mark event processed: %w", err)
	}

	// Store event in audit log
	contentJSON, _ := json.Marshal(evt.Content.Raw)
	matrixEvent := &model.MatrixEvent{
		EventID:          evt.ID.String(),
		RoomID:           evt.RoomID.String(),
		Sender:           evt.Sender.String(),
		EventType:        evt.Type.String(),
		Content:          string(contentJSON),
		ProcessingStatus: "pending",
	}

	if err := s.client.repos.MatrixEvent.Create(matrixEvent); err != nil {
		log.Printf("[Matrix] failed to store event in DB: %v", err)
		// Don't return error, continue processing
	}

	// Extract message content
	content := evt.Content.AsMessage()
	if content == nil {
		return nil
	}

	log.Printf("[Matrix] received message in %s from %s: %s", evt.RoomID, evt.Sender, content.Body)

	// Route to command handler if command service is set
	if s.commandService != nil {
		if err := s.commandService.ExecuteMessage(ctx, evt.RoomID.String(), evt.Sender.String(), evt.ID.String(), content.Body); err != nil {
			log.Printf("[Matrix] command execution failed: %v", err)
			s.client.repos.MatrixEvent.UpdateStatus(evt.ID.String(), "failed", err.Error())
			return nil
		}
	}

	// Mark as processed
	if err := s.client.repos.MatrixEvent.UpdateStatus(evt.ID.String(), "processed", ""); err != nil {
		log.Printf("[Matrix] failed to update event status: %v", err)
	}

	return nil
}

// handleMemberEvent processes room member events (invites, joins, leaves)
func (s *SyncLoop) handleMemberEvent(ctx context.Context, evt *event.Event) error {
	content := evt.Content.AsMember()
	if content == nil {
		return nil
	}

	// Auto-accept invites
	if content.Membership == event.MembershipInvite && evt.GetStateKey() == s.client.config.BotUserID {
		log.Printf("[Matrix] received invite to room %s from %s", evt.RoomID, evt.Sender)
		if err := s.client.JoinRoom(ctx, evt.RoomID.String()); err != nil {
			return fmt.Errorf("failed to accept invite: %w", err)
		}
	}

	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
