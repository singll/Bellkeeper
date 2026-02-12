package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// WebhookConfig represents a webhook configuration
type WebhookConfig struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Name           string         `gorm:"size:200;not null" json:"name"`
	URL            string         `gorm:"size:1000;not null" json:"url"`
	Method         string         `gorm:"size:10;default:'POST'" json:"method"`
	ContentType    string         `gorm:"size:100;default:'application/json'" json:"content_type"`
	Headers        datatypes.JSON `gorm:"type:jsonb" json:"headers,omitempty"`
	BodyTemplate   string         `gorm:"type:text" json:"body_template,omitempty"`
	TimeoutSeconds int            `gorm:"default:30" json:"timeout_seconds"`
	Description    string         `gorm:"type:text" json:"description"`
	IsActive       bool           `gorm:"default:true" json:"is_active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	History []WebhookHistory `gorm:"foreignKey:WebhookID" json:"history,omitempty"`
}

// TableName specifies table name
func (WebhookConfig) TableName() string {
	return "webhook_configs"
}

// WebhookHistory represents a webhook invocation record
type WebhookHistory struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	WebhookID       uint           `gorm:"index" json:"webhook_id"`
	RequestURL      string         `gorm:"size:1000" json:"request_url"`
	RequestMethod   string         `gorm:"size:10" json:"request_method"`
	RequestHeaders  datatypes.JSON `gorm:"type:jsonb" json:"request_headers,omitempty"`
	RequestBody     string         `gorm:"type:text" json:"request_body,omitempty"`
	Status          string         `gorm:"size:20;default:'pending'" json:"status"`
	ResponseCode    int            `json:"response_code,omitempty"`
	ResponseHeaders datatypes.JSON `gorm:"type:jsonb" json:"response_headers,omitempty"`
	ResponseBody    string         `gorm:"type:text" json:"response_body,omitempty"`
	DurationMs      int            `json:"duration_ms,omitempty"`
	ErrorMessage    string         `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt       time.Time      `gorm:"index" json:"created_at"`

	// Relations
	Webhook WebhookConfig `gorm:"foreignKey:WebhookID" json:"webhook,omitempty"`
}

// TableName specifies table name
func (WebhookHistory) TableName() string {
	return "webhook_history"
}
