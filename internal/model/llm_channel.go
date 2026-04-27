package model

import (
	"encoding/json"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// LLMChannel represents an LLM API channel configuration stored in the database.
// API keys are referenced by environment variable name (not stored directly).
type LLMChannel struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"size:100;uniqueIndex;not null" json:"name"`
	BaseURL   string         `gorm:"size:500;not null" json:"base_url"`
	APIKeyEnv    string         `gorm:"size:200" json:"api_key_env"`
	ProviderType string         `gorm:"size:50;default:'openai'" json:"provider_type"`
	RPM          int            `gorm:"default:500" json:"rpm"`
	RPD       int            `gorm:"default:50000" json:"rpd"`
	Priority  int            `gorm:"default:1" json:"priority"`
	IsFree    bool           `gorm:"default:false" json:"is_free"`
	IsEnabled bool           `gorm:"default:true" json:"is_enabled"`
	Models    string         `gorm:"type:text" json:"models"` // JSON array string
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LLMChannel) TableName() string {
	return "llm_channels"
}

// GetModels parses the JSON models string into a slice.
func (c *LLMChannel) GetModels() []string {
	var models []string
	if c.Models == "" {
		return models
	}
	if err := json.Unmarshal([]byte(c.Models), &models); err != nil {
		middleware.GetLogger().Warn("failed to parse models JSON for channel",
			zap.String("channel", c.Name), zap.Error(err))
		return []string{}
	}
	return models
}

// SetModels serializes a slice of model names into JSON.
func (c *LLMChannel) SetModels(models []string) {
	data, err := json.Marshal(models)
	if err != nil {
		middleware.GetLogger().Warn("failed to marshal models for channel",
			zap.String("channel", c.Name), zap.Error(err))
		c.Models = "[]"
		return
	}
	c.Models = string(data)
}
