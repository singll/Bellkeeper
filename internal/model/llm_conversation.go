package model

import "time"

// LLMConversationBinding binds a conversation_id to a specific channel+model
// for the duration of a session, protecting prompt cache across requests.
type LLMConversationBinding struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ConversationID string   `gorm:"size:64;uniqueIndex;not null" json:"conversation_id"`
	ChannelID     uint      `gorm:"index;not null" json:"channel_id"`
	ChannelName   string    `gorm:"size:100" json:"channel_name"`
	Model         string    `gorm:"size:200" json:"model"`
	TaskType      string    `gorm:"size:50" json:"task_type"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	RequestCount  int       `gorm:"default:0" json:"request_count"`
	TotalTokens   int       `gorm:"default:0" json:"total_tokens"`
	TotalCostCents int      `gorm:"default:0" json:"total_cost_cents"`
}

func (LLMConversationBinding) TableName() string {
	return "llm_conversation_bindings"
}
