package model

import (
	"log"
	"os"
	"time"

	"gorm.io/gorm"
)

// MatrixRoom represents a registered Matrix room in the platform.
type MatrixRoom struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RoomID    string    `gorm:"size:255;uniqueIndex;notNull" json:"room_id"`
	RoomName  string    `gorm:"size:255" json:"room_name,omitempty"`
	RoomType  string    `gorm:"size:50;notNull;index" json:"room_type"` // command, notification, admin
	IsActive  bool      `gorm:"default:true;index" json:"is_active"`
	Config    *string   `gorm:"type:jsonb" json:"config,omitempty"`
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
	Config     *string   `gorm:"type:jsonb" json:"config,omitempty"`
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
	HandlerType     string    `gorm:"size:100;notNull;index" json:"handler_type"` // memos_todo, knowledge_qa, n8n_workflow
	HandlerConfig   *string   `gorm:"type:jsonb" json:"handler_config,omitempty"`
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
	Content          *string   `gorm:"type:jsonb" json:"content,omitempty"`
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
	Metadata       *string   `gorm:"type:jsonb" json:"metadata,omitempty"`
	Status         string    `gorm:"size:50;default:pending;index" json:"status"` // pending, sent, failed, retrying
	RetryCount     int       `gorm:"default:0" json:"retry_count"`
	LastError      string    `gorm:"type:text" json:"last_error,omitempty"`
	SentEventID    string    `gorm:"size:255" json:"sent_event_id,omitempty"`
	DedupKey       string    `gorm:"size:255;index" json:"dedup_key,omitempty"`
	Severity       string    `gorm:"size:50;default:info" json:"severity,omitempty"`
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

// MatrixUserRole stores user roles in rooms.
type MatrixUserRole struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"size:255;notNull;index:idx_user_room,unique" json:"user_id"` // Matrix user ID
	RoomID    string    `gorm:"size:255;notNull;index:idx_user_room,unique" json:"room_id"` // Matrix room ID
	Role      string    `gorm:"size:50;notNull" json:"role"`                               // owner, admin, member, guest
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MatrixUserRole) TableName() string {
	return "matrix_user_roles"
}

// SeedMatrixPlatform seeds initial Matrix platform data (rooms, channels, commands).
func SeedMatrixPlatform(db *gorm.DB) error {
	// Check if already seeded
	var roomCount int64
	db.Model(&MatrixRoom{}).Count(&roomCount)
	if roomCount > 0 {
		return nil // Already seeded
	}

	log.Println("info: seeding Matrix platform initial data...")

	// Seed rooms from environment variables
	type roomSeed struct {
		RoomID   string
		RoomName string
		RoomType string
	}

	// Note: Room IDs will be loaded from environment variables at runtime
	// For now, we just create the structure. Actual room registration
	// will happen when the Matrix gateway starts.

	// Seed default channels (logical channels mapped to rooms via env vars)
	emptyJSON := func(s string) *string { return &s }
	channels := []MatrixChannel{
		{ChannelName: "alerts", RoomID: os.Getenv("BELLKEEPER_MATRIX_ROOM_ALERTS"), IsActive: true, Priority: 100, Config: emptyJSON("{}")},
		{ChannelName: "daily", RoomID: os.Getenv("BELLKEEPER_MATRIX_ROOM_DAILY"), IsActive: true, Priority: 50, Config: emptyJSON("{}")},
		{ChannelName: "todo", RoomID: os.Getenv("BELLKEEPER_MATRIX_ROOM_TODO"), IsActive: true, Priority: 30, Config: emptyJSON("{}")},
		{ChannelName: "qa", RoomID: os.Getenv("BELLKEEPER_MATRIX_ROOM_QA"), IsActive: true, Priority: 30, Config: emptyJSON("{}")},
	}

	for _, ch := range channels {
		var existing MatrixChannel
		if err := db.Where("channel_name = ?", ch.ChannelName).First(&existing).Error; err == nil {
			// Channel exists â update RoomID if empty and env var is set
			if existing.RoomID == "" && ch.RoomID != "" {
				if err := db.Model(&existing).Update("room_id", ch.RoomID).Error; err != nil {
					log.Printf("warn: failed to update RoomID for channel %q: %v", ch.ChannelName, err)
				} else {
					log.Printf("info: updated RoomID for channel %q from env var", ch.ChannelName)
				}
			}
		} else {
			if err := db.Create(&ch).Error; err != nil {
				log.Printf("warn: failed to seed Matrix channel %q: %v", ch.ChannelName, err)
				continue
			}
			log.Printf("info: seeded Matrix channel %q", ch.ChannelName)
		}
	}

	// Seed default commands
	commands := []MatrixCommand{
		{
			CommandName:     "help",
			HandlerType:     "builtin_help",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "显示可用命令列表",
			UsageExample:    "!help",
		},
		{
			CommandName:     "status",
			HandlerType:     "builtin_status",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "显示系统状态",
			UsageExample:    "!status",
		},
		{
			CommandName:     "列表",
			HandlerType:     "memos_todo_list",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "查看 Memos 待办列表",
			UsageExample:    "!列表",
		},
		{
			CommandName:     "新增",
			HandlerType:     "memos_todo_create",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "创建 Memos 待办事项",
			UsageExample:    "!新增 完成项目文档",
		},
		{
			CommandName:     "问",
			HandlerType:     "knowledge_qa",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "向知识库提问",
			UsageExample:    "!问 什么是 GORM？",
		},
		{
			CommandName:     "搜",
			HandlerType:     "knowledge_search",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "在知识库中搜索",
			UsageExample:    "!搜 Docker 部署",
		},
		{
			CommandName:     "待办",
			HandlerType:     "memos_todo",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "Memos 待办管理",
			UsageExample:    "!待办 列表",
		},
		{
			CommandName:     "完成",
			HandlerType:     "memos_todo",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "标记待办完成",
			UsageExample:    "!完成 123",
		},
		{
			CommandName:     "search",
			HandlerType:     "knowledge_search",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "Search the knowledge base",
			UsageExample:    "!search Docker",
		},
		{
			CommandName:     "ping",
			HandlerType:     "builtin_ping",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "测试机器人响应",
			UsageExample:    "!ping",
		},
		{
			CommandName:     "commands",
			HandlerType:     "builtin_commands",
			PermissionLevel: "user",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "列出所有可用命令",
			UsageExample:    "!commands",
		},
		{
			CommandName:     "health",
			HandlerType:     "builtin_health",
			PermissionLevel: "admin",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "显示系统健康状态",
			UsageExample:    "!health",
		},
		{
			CommandName:     "rooms",
			HandlerType:     "builtin_rooms",
			PermissionLevel: "admin",
			RoomScope:       "any",
			IsActive:        true,
			HandlerConfig:   emptyJSON("{}"),
			Description:     "列出 Matrix 房间",
			UsageExample:    "!rooms",
		},
	}

	for _, cmd := range commands {
		if err := db.Create(&cmd).Error; err != nil {
			log.Printf("warn: failed to seed Matrix command %q: %v", cmd.CommandName, err)
			continue
		}
		log.Printf("info: seeded Matrix command %q", cmd.CommandName)
	}

	log.Println("info: Matrix platform seeding completed")
	return nil
}

