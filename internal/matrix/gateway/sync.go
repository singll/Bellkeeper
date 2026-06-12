package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"go.uber.org/zap"
)

// SyncLoop manages the Matrix sync loop
type SyncLoop struct {
	client            *Client
	syncer            *mautrix.DefaultSyncer
	stopCh            chan struct{}
	stopped           atomic.Bool
	dbPersistFailSince time.Time
	commandService    interface {
		ExecuteMessage(ctx context.Context, roomID, sender, eventID, content string) error
	}
	dispatcherWg sync.WaitGroup
	dispatchSem  chan struct{}
}

// NewSyncLoop creates a new sync loop
func NewSyncLoop(client *Client) *SyncLoop {
	syncer := client.client.Syncer.(*mautrix.DefaultSyncer)

	loop := &SyncLoop{
		client:      client,
		syncer:      syncer,
		stopCh:      make(chan struct{}),
		dispatchSem: make(chan struct{}, 8),
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
			middleware.GetLogger().Error("error handling Matrix message", zap.Error(err))
		}
	})

	// Handle room member events (invites, joins, leaves)
	s.syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		if err := s.handleMemberEvent(ctx, evt); err != nil {
			middleware.GetLogger().Error("error handling member event", zap.Error(err))
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
		middleware.GetLogger().Info("resuming sync from token", zap.String("token_prefix", token[:min(len(token), 20)]))
		s.client.client.Store.SaveNextBatch(context.Background(), s.client.client.UserID, token)
	} else {
		middleware.GetLogger().Info("starting fresh sync (no previous token)")
	}

	// Also try to load from database
	syncState, err := s.client.repos.MatrixSyncState.GetByUserID(ctx, s.client.config.BotUserID)
	if err == nil && syncState != nil && syncState.NextBatch != "" {
		middleware.GetLogger().Info("found DB sync token", zap.String("token_prefix", syncState.NextBatch[:min(len(syncState.NextBatch), 20)]))
		s.client.client.Store.SaveNextBatch(context.Background(), s.client.client.UserID, syncState.NextBatch)
	}

	// Room auto-discovery: upsert joined rooms
	s.discoverJoinedRooms(ctx)

	// Start sync in background
	go func() {
		middleware.GetLogger().Info("starting sync loop")
		if err := s.client.client.Sync(); err != nil {
			middleware.GetLogger().Error("sync stopped", zap.Error(err))
		}
	}()

	// Start token persistence goroutine
	go s.persistTokenPeriodically(ctx)

	return nil
}

// Stop stops the sync loop
func (s *SyncLoop) Stop() {
	if s.stopped.Load() {
		return
	}
	s.stopped.Store(true)
	close(s.stopCh)
	s.dispatcherWg.Wait()
	s.client.client.StopSync()
	middleware.GetLogger().Info("sync loop stopped")
}

// persistTokenPeriodically saves sync token to Redis and DB every 30 seconds
func (s *SyncLoop) persistTokenPeriodically(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.persistToken(ctx); err != nil {
				if s.dbPersistFailSince.IsZero() {
					s.dbPersistFailSince = time.Now()
				}
				if time.Since(s.dbPersistFailSince) > 10*time.Minute {
					middleware.GetLogger().Error("sync token persistence failing for over 10 minutes", zap.Error(err), zap.Duration("duration", time.Since(s.dbPersistFailSince)))
				} else {
					middleware.GetLogger().Warn("failed to persist sync token", zap.Error(err))
				}
			} else {
				s.dbPersistFailSince = time.Time{}
			}
		case <-s.stopCh:
			// Final save before exit
			if err := s.persistToken(ctx); err != nil {
				middleware.GetLogger().Warn("failed to persist sync token on shutdown", zap.Error(err))
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

	// Room whitelist check
	roomID := evt.RoomID.String()
	if !s.isRoomEnabled(ctx, roomID) {
		middleware.GetLogger().Debug("skipping message from non-whitelisted room", zap.String("room", roomID))
		return nil
	}

	// Check if event already processed (deduplication)
	processed, err := s.client.redis.CheckEventProcessed(ctx, evt.ID.String())
	if err != nil {
		return fmt.Errorf("failed to check event processed: %w", err)
	}
	if processed {
		middleware.GetLogger().Info("skipping already processed event", zap.String("event_id", evt.ID.String()))
		return nil
	}

	// Mark as processed
	if err := s.client.redis.MarkEventProcessed(ctx, evt.ID.String()); err != nil {
		return fmt.Errorf("failed to mark event processed: %w", err)
	}

	// Store event in audit log
	contentJSON, _ := json.Marshal(evt.Content.Raw)
	contentStr := string(contentJSON)
	matrixEvent := &model.MatrixEvent{
		EventID:          evt.ID.String(),
		RoomID:           evt.RoomID.String(),
		Sender:           evt.Sender.String(),
		EventType:        evt.Type.String(),
		Content:          &contentStr,
		ProcessingStatus: "pending",
	}

	if err := s.client.repos.MatrixEvent.Create(matrixEvent); err != nil {
		middleware.GetLogger().Warn("failed to store event in DB", zap.Error(err))
	}

	// Extract message content
	content := evt.Content.AsMessage()
	if content == nil {
		return nil
	}

	middleware.GetLogger().Info("received message",
		zap.String("room", evt.RoomID.String()),
		zap.String("sender", evt.Sender.String()),
		zap.String("body", content.Body))

	// Dispatch command execution asynchronously with bounded worker pool
	if s.commandService != nil {
		s.dispatchCommand(ctx, evt, content.Body)
	}

	return nil
}

// isRoomEnabled checks if a room is in the whitelist
func (s *SyncLoop) isRoomEnabled(ctx context.Context, roomID string) bool {
	room, err := s.client.repos.MatrixRoom.GetByRoomID(roomID)
	if err != nil || room == nil {
		return false
	}
	return room.IsActive
}

// dispatchCommand dispatches command execution to the bounded worker pool
func (s *SyncLoop) dispatchCommand(ctx context.Context, evt *event.Event, body string) {
	s.dispatchSem <- struct{}{}
	s.dispatcherWg.Add(1)
	go func() {
		defer func() {
			<-s.dispatchSem
			s.dispatcherWg.Done()
		}()
		if err := s.commandService.ExecuteMessage(ctx, evt.RoomID.String(), evt.Sender.String(), evt.ID.String(), body); err != nil {
			middleware.GetLogger().Warn("command execution failed", zap.Error(err))
			s.client.repos.MatrixEvent.UpdateStatus(evt.ID.String(), "failed", err.Error())
			return
		}
		if err := s.client.repos.MatrixEvent.UpdateStatus(evt.ID.String(), "processed", ""); err != nil {
			middleware.GetLogger().Warn("failed to update event status", zap.Error(err))
		}
	}()
}

// discoverJoinedRooms upserts all joined rooms into the database
func (s *SyncLoop) discoverJoinedRooms(ctx context.Context) {
	rooms, err := s.client.JoinedRooms(ctx)
	if err != nil {
		middleware.GetLogger().Warn("failed to discover joined rooms", zap.Error(err))
		return
	}

	count := 0
	for _, roomID := range rooms {
		room := &model.MatrixRoom{
			RoomID:   roomID,
			RoomType: "command",
			IsActive: true,
		}
		if err := s.client.repos.MatrixRoom.Upsert(ctx, room); err != nil {
			middleware.GetLogger().Warn("failed to upsert room", zap.String("room_id", roomID), zap.Error(err))
		} else {
			count++
		}
	}

	middleware.GetLogger().Info("discovered joined rooms", zap.Int("count", count))
}

// handleMemberEvent processes room member events (invites, joins, leaves)
func (s *SyncLoop) handleMemberEvent(ctx context.Context, evt *event.Event) error {
	content := evt.Content.AsMember()
	if content == nil {
		return nil
	}

	// Auto-accept invites
	if content.Membership == event.MembershipInvite && evt.GetStateKey() == s.client.config.BotUserID {
		middleware.GetLogger().Info("received room invite",
			zap.String("room", evt.RoomID.String()),
			zap.String("sender", evt.Sender.String()))
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
