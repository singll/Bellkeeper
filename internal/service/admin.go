package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/singll/bellkeeper/internal/matrix/policy"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// AdminService provides admin operations for Matrix platform
type AdminService struct {
	repos  *repository.Repositories
	policy *policy.Checker
}

// NewAdminService creates a new admin service
func NewAdminService(repos *repository.Repositories, policyChecker *policy.Checker) *AdminService {
	return &AdminService{
		repos:  repos,
		policy: policyChecker,
	}
}

// GetUserRolePolicy gets a user's role as a policy.Role (for permission checks)
func (s *AdminService) GetUserRolePolicy(ctx context.Context, userID, roomID string) (policy.Role, error) {
	userRole, err := s.repos.MatrixUserRole.GetByUserAndRoom(ctx, userID, roomID)
	if err != nil {
		return "", err
	}
	if userRole == nil {
		return "", nil // Not found
	}
	return policy.ParseRole(userRole.Role), nil
}

// ============ Room Management ============

// RoomResponse represents room info for API response
type RoomResponse struct {
	RoomID   string `json:"room_id"`
	Name     string `json:"room_name"`
	Type     string `json:"room_type"`
	IsActive bool   `json:"is_active"`
}

// ListRooms returns all registered rooms
func (s *AdminService) ListRooms(ctx context.Context) ([]*RoomResponse, error) {
	rooms, err := s.repos.MatrixRoom.List(false)
	if err != nil {
		return nil, err
	}

	result := make([]*RoomResponse, len(rooms))
	for i, r := range rooms {
		result[i] = &RoomResponse{
			RoomID:   r.RoomID,
			Name:     r.RoomName,
			Type:     r.RoomType,
			IsActive: r.IsActive,
		}
	}
	return result, nil
}

// CreateRoom creates a new room registration
func (s *AdminService) CreateRoom(ctx context.Context, roomID, name, roomType string) error {
	room := &model.MatrixRoom{
		RoomID:   roomID,
		RoomName: name,
		RoomType: roomType,
		IsActive: true,
	}
	return s.repos.MatrixRoom.Create(room)
}

// DeleteRoom deletes a room by room ID
func (s *AdminService) DeleteRoom(ctx context.Context, roomID string) error {
	return s.repos.MatrixRoom.Delete(roomID)
}

// ============ Channel Management ============

// ChannelResponse represents channel info for API response
type ChannelResponse struct {
	Name     string                 `json:"name"`
	RoomID   string                 `json:"room_id"`
	IsActive bool                   `json:"is_active"`
	Priority int                    `json:"priority"`
	Config   map[string]interface{} `json:"config,omitempty"`
}

// ListChannels returns all configured channels
func (s *AdminService) ListChannels(ctx context.Context) ([]*ChannelResponse, error) {
	channels, err := s.repos.MatrixChannel.List(false)
	if err != nil {
		return nil, err
	}

	result := make([]*ChannelResponse, len(channels))
	for i, c := range channels {
		var config map[string]interface{}
		if c.Config != nil {
			json.Unmarshal([]byte(*c.Config), &config)
		}

		result[i] = &ChannelResponse{
			Name:     c.ChannelName,
			RoomID:   c.RoomID,
			IsActive: c.IsActive,
			Priority: c.Priority,
			Config:   config,
		}
	}
	return result, nil
}

// UpdateChannel updates a channel configuration
func (s *AdminService) UpdateChannel(ctx context.Context, name string, updates map[string]interface{}) error {
	channel, err := s.repos.MatrixChannel.GetByName(name)
	if err != nil || channel == nil {
		return err
	}

	if active, ok := updates["is_active"].(bool); ok {
		channel.IsActive = active
	}
	if priority, ok := updates["priority"].(int); ok {
		channel.Priority = priority
	}
	if roomID, ok := updates["room_id"].(string); ok {
		channel.RoomID = roomID
	}

	return s.repos.MatrixChannel.Update(channel)
}

// ============ Command Management ============

// CommandResponse represents command info for API response
type CommandResponse struct {
	Name     string `json:"name"`
	Handler  string `json:"handler_type"`
	Perm     string `json:"permission_level"`
	IsActive bool   `json:"is_active"`
	Desc     string `json:"description"`
}

// ListCommands returns all registered commands
func (s *AdminService) ListCommands(ctx context.Context) ([]*CommandResponse, error) {
	commands, err := s.repos.MatrixCommand.List(false)
	if err != nil {
		return nil, err
	}

	result := make([]*CommandResponse, len(commands))
	for i, c := range commands {
		result[i] = &CommandResponse{
			Name:     c.CommandName,
			Handler:  c.HandlerType,
			Perm:     c.PermissionLevel,
			IsActive: c.IsActive,
			Desc:     c.Description,
		}
	}
	return result, nil
}

// ============ Audit ============

// EventLog represents an event log entry
type EventLog struct {
	EventID   string    `json:"event_id"`
	RoomID    string    `json:"room_id"`
	Sender    string    `json:"sender"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationLog represents a notification log entry
type NotificationLog struct {
	ID             uint      `json:"id"`
	ChannelName    string    `json:"channel_name"`
	Status         string    `json:"status"`
	Retries        int       `json:"retry_count"`
	MessageContent string    `json:"message_content,omitempty"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// GetEventLogs returns recent event logs
func (s *AdminService) GetEventLogs(ctx context.Context, limit int) ([]*EventLog, error) {
	events, err := s.repos.MatrixEvent.GetRecent(ctx, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*EventLog, len(events))
	for i, e := range events {
		result[i] = &EventLog{
			EventID:   e.EventID,
			RoomID:    e.RoomID,
			Sender:    e.Sender,
			Type:      e.EventType,
			Status:    e.ProcessingStatus,
			CreatedAt: e.CreatedAt,
		}
	}
	return result, nil
}

// GetNotificationLogs returns recent notification logs
func (s *AdminService) GetNotificationLogs(ctx context.Context, limit int) ([]*NotificationLog, error) {
	notifications, err := s.repos.MatrixNotification.GetRecent(ctx, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*NotificationLog, len(notifications))
	for i, n := range notifications {
		result[i] = &NotificationLog{
			ID:             n.ID,
			ChannelName:    n.ChannelName,
			Status:         n.Status,
			Retries:        n.RetryCount,
			MessageContent: n.MessageContent,
			ErrorMessage:   n.LastError,
			CreatedAt:      n.CreatedAt,
		}
	}
	return result, nil
}

// GetStats returns platform statistics
func (s *AdminService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Event stats
	events, _ := s.repos.MatrixEvent.GetRecent(ctx, 1000)
	stats["events_24h"] = len(events)

	// Notification stats
	notifications, _ := s.repos.MatrixNotification.GetRecent(ctx, 1000)
	stats["notifications_24h"] = len(notifications)

	// Room/Channel/Command counts
	rooms, _ := s.repos.MatrixRoom.List(false)
	channels, _ := s.repos.MatrixChannel.List(false)
	commands, _ := s.repos.MatrixCommand.List(false)

	stats["rooms"] = len(rooms)
	stats["channels"] = len(channels)
	stats["commands"] = len(commands)

	// Active counts
	activeRooms := 0
	for _, r := range rooms {
		if r.IsActive {
			activeRooms++
		}
	}
	stats["active_rooms"] = activeRooms

	return stats, nil
}

// ============ User Role Management ============

// ListUserRoles returns all user roles in a room
func (s *AdminService) ListUserRoles(ctx context.Context, roomID string) ([]*model.MatrixUserRole, error) {
	return s.repos.MatrixUserRole.GetByRoom(ctx, roomID)
}

// GetUserRole returns a user's role in a room
func (s *AdminService) GetUserRole(ctx context.Context, userID, roomID string) (*model.MatrixUserRole, error) {
	return s.repos.MatrixUserRole.GetByUserAndRoom(ctx, userID, roomID)
}

// SetUserRole sets a user's role in a room
func (s *AdminService) SetUserRole(ctx context.Context, userID, roomID, role string) error {
	return s.repos.MatrixUserRole.Upsert(ctx, userID, roomID, role)
}

// RemoveUserRole removes a user's role from a room
func (s *AdminService) RemoveUserRole(ctx context.Context, userID, roomID string) error {
	return s.repos.MatrixUserRole.Delete(ctx, userID, roomID)
}

// ListAllUserRoles returns all user roles
func (s *AdminService) ListAllUserRoles(ctx context.Context, limit, offset int) ([]*model.MatrixUserRole, int64, error) {
	roles, err := s.repos.MatrixUserRole.List(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	count, err := s.repos.MatrixUserRole.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	return roles, count, nil
}

// ============ Command Log Management ============

// CommandLogResponse represents a command log for API response
type CommandLogResponse struct {
	ID              uint       `json:"id"`
	EventID         string     `json:"event_id"`
	Command         string     `json:"command"`
	UserID          string     `json:"user_id"`
	RoomID          string     `json:"room_id"`
	Args            string     `json:"args"`
	ExecutionStatus string     `json:"execution_status"`
	ExecutionTimeMs int        `json:"execution_time_ms"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	ResponseEventID string     `json:"response_event_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// ListCommandLogs returns command logs with optional filtering and pagination
func (s *AdminService) ListCommandLogs(ctx context.Context, page, pageSize int, command, status string) ([]*CommandLogResponse, int64, error) {
	logs, total, err := s.repos.MatrixCommandLog.List(page, pageSize, command, status)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*CommandLogResponse, len(logs))
	for i, log := range logs {
		result[i] = &CommandLogResponse{
			ID:              log.ID,
			EventID:         log.EventID,
			Command:         log.CommandName,
			UserID:          log.Sender,
			RoomID:          log.RoomID,
			Args:            log.CommandArgs,
			ExecutionStatus: log.ExecutionStatus,
			ExecutionTimeMs: log.ExecutionTimeMs,
			ErrorMessage:    log.ErrorMessage,
			ResponseEventID: log.ResponseEventID,
			CreatedAt:       log.CreatedAt,
		}
		if !log.CompletedAt.IsZero() {
			completedAt := log.CompletedAt
			result[i].CompletedAt = &completedAt
		}
	}
	return result, total, nil
}
