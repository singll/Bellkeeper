package model

import "time"

// MatrixRoom represents a registered Matrix room in the platform.
type MatrixRoom struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RoomID    string    `gorm:"size:255;uniqueIndex;notNull" json:"room_id"`
	RoomName  string    `gorm:"size:255" json:"room_name,omitempty"`
	RoomType  string    `gorm:"size:50;notNull;index" json:"room_type"` // command, notification, admin
	IsActive  bool      `gorm:"default:true;index" json:"is_active"`
	Config    string    `gorm:"type:jsonb" json:"config,omitempty"` // room-level config as JSON
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MatrixRoom) TableName() string {
	return "matrix_rooms"
}

// MatrixChannel represents a logical notification channel.
type MatrixChannel struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ChannelName string   `gorm:"size:100;uniqueIndex;notNull" json:"channel_name"` // alerts, daily, todo, qa
	RoomID     string    `gorm:"size:255;notNull;index" json:"room_id"`
	IsActive   bool      `gorm:"default:true;index" json:"is_active"`
	Priority   int       `gorm:"default:0" json:"priority"`
	Config     string    `gorm:"type:jsonb" json:"config,omitempty"` // channel-level config
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (MatrixChannel) TableName() string {
	return "matrix_channels"
}

// MatrixCommand represents a registered bot command.
type MatrixCommand struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	CommandName     string    `gorm:"size:100;uniqueIndex;notNull" json:"command_name"` // 列表, list, 新增, etc.
	HandlerType     string    `gorm:"size:100;notNull;index" json:"handler_type"` // memos_todo, ragflow_qa, n8n_workflow
	HandlerConfig   string    `gorm:"type:jsonb" json:"handler_config,omitempty"`
	PermissionLevel string    `gorm:"size:50;default:user" json:"permission_level"` // admin, user, guest
	RoomScope       string    `gorm:"size:50;default:any" json:"room_scope"` // any, specific, admin_only
	IsActive        bool      `gorm:"default:true;index" json:"is_active"`
	Description     string    `gorm:"type:text" json:"description,omitempty"`
	UsageExample    string    `gorm:"type:text" json:"usage_example,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (MatrixCommand) TableName() string {
	return "matrix_commands"
}

// MatrixEvent records a raw Matrix event for audit and dedup.
type MatrixEvent struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	EventID          string    `gorm:"size:255;uniqueIndex;notNull" json:"event_id"`
	RoomID           string    `gorm:"size:255;notNull;index" json:"room_id"`
	Sender           string    `gorm:"size:255;notNull" json:"sender"`
	EventType        string    `gorm:"size:100;notNull" json:"event_type"` // m.room.message, m.room.member
	Content          string    `gorm:"type:jsonb" json:"content,omitempty"`
	ProcessingStatus string    `gorm:"size:50;default:pending;index" json:"processing_status"` // pending, processed, failed, ignored
	ErrorMessage     string    `gorm:"type:text" json:"error_message,omitempty"`
	ProcessedAt      time.Time `json:"processed_at,omitempty"`
	CreatedAt        time.Time `gorm:"index" json:"created_at"`
}

func (MatrixEvent) TableName() string {
	return "matrix_events"
}

// MatrixNotification records a notification for audit and retry tracking.
type MatrixNotification struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	NotificationID string    `gorm:"size:100;uniqueIndex;notNull" json:"notification_id"` // idempotency key
	ChannelName    string    `gorm:"size:100;notNull;index" json:"channel_name"`
	RoomID         string    `gorm:"size:255" json:"room_id,omitempty"`
	MessageType    string    `gorm:"size:50;default:text" json:"message_type"` // text, html, markdown
	MessageContent string    `gorm:"type:text;notNull" json:"message_content"`
	Metadata       string    `gorm:"type:jsonb" json:"metadata,omitempty"`
	Status         string    `gorm:"size:50;default:pending;index" json:"status"` // pending, sent, failed, retrying
	RetryCount     int       `gorm:"default:0" json:"retry_count"`
	LastError      string    `gorm:"type:text" json:"last_error,omitempty"`
	SentEventID    string    `gorm:"size:255" json:"sent_event_id,omitempty"`
	SentAt         time.Time `json:"sent_at,omitempty"`
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (MatrixNotification) TableName() string {
	return "matrix_notifications"
}

// MatrixCommandLog records command execution for audit.
type MatrixCommandLog struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	EventID         string    `gorm:"size:255;notNull;index" json:"event_id"`
	RoomID          string    `gorm:"size:255;notNull;index" json:"room_id"`
	Sender          string    `gorm:"size:255;notNull" json:"sender"`
	CommandName     string    `gorm:"size:100;notNull;index" json:"command_name"`
	CommandArgs     string    `gorm:"type:text" json:"command_args,omitempty"`
	HandlerType     string    `gorm:"size:100" json:"handler_type,omitempty"`
	ExecutionStatus string    `gorm:"size:50;default:pending;index" json:"execution_status"` // pending, success, failed
	ExecutionTimeMs int       `json:"execution_time_ms,omitempty"`
	ErrorMessage    string    `gorm:"type:text" json:"error_message,omitempty"`
	ResponseEventID string    `gorm:"size:255" json:"response_event_id,omitempty"`
	CreatedAt       time.Time `gorm:"index" json:"created_at"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
}

func (MatrixCommandLog) TableName() string {
	return "matrix_command_logs"
}

// MatrixSyncState stores the Matrix sync token for resumable sync.
type MatrixSyncState struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BotUserID  string    `gorm:"size:255;uniqueIndex;notNull" json:"bot_user_id"`
	NextBatch  string    `gorm:"size:255" json:"next_batch,omitempty"` // Matrix sync token
	FilterID   string    `gorm:"size:100" json:"filter_id,omitempty"`
	LastSyncAt time.Time `json:"last_sync_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (MatrixSyncState) TableName() string {
	return "matrix_sync_state"
}
