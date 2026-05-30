package model

import "time"

// LLMAlertEvent stores an alert event for aggregation and routing.
type LLMAlertEvent struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AlertType  string    `gorm:"size:50;index;not null" json:"alert_type"` // e.g. "circuit_open", "quota_threshold", "balance_zero"
	Severity   string    `gorm:"size:20;index;not null" json:"severity"`   // info | warning | error | critical
	ChannelID  uint      `gorm:"index" json:"channel_id"`
	ChannelName string   `gorm:"size:100" json:"channel_name"`
	Message    string    `gorm:"type:text" json:"message"`
	DedupKey   string    `gorm:"size:200;index" json:"dedup_key"`
	FlushedAt  *time.Time `json:"flushed_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (LLMAlertEvent) TableName() string {
	return "llm_alert_events"
}
