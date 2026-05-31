package model

import "time"

// LLMChannelCredential stores per-channel provider credentials encrypted at rest.
// CredentialJSON holds the AES-256-GCM ciphertext (see internal/pkg/crypto); it is
// never serialized to API clients (json:"-") — handlers expose a masked preview only.
type LLMChannelCredential struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	ChannelID       uint       `gorm:"index;not null" json:"channel_id"`
	ProviderType    string     `gorm:"size:50" json:"provider_type"`
	CredentialJSON  string     `gorm:"type:text" json:"-"` // encrypted; never returned to API
	Status          string     `gorm:"size:20;default:'active'" json:"status"` // active | error | expired
	ErrorMessage    string     `gorm:"type:text" json:"error_message,omitempty"`
	LastRefreshedAt *time.Time `json:"last_refreshed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (LLMChannelCredential) TableName() string {
	return "llm_channel_credentials"
}
