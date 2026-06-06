package model

import (
	"time"
)

// LLMModelRateLimit stores adaptive rate-limit learning state per channel×model.
type LLMModelRateLimit struct {
	ID                     uint      `gorm:"primaryKey" json:"id"`
	ChannelID              uint      `gorm:"index;not null" json:"channel_id"`
	Model                  string    `gorm:"size:200;index;not null" json:"model"`
	ConfiguredRPM          int       `gorm:"default:0" json:"configured_rpm"`
	ConfiguredRPD          int       `gorm:"default:0" json:"configured_rpd"`
	LearnedRPMSafe         int       `gorm:"default:0" json:"learned_rpm_safe"`
	LearnedRPDSafe         int       `gorm:"default:0" json:"learned_rpd_safe"`
	LearnedConcurrentMax   int       `gorm:"default:0" json:"learned_concurrent_max"`
	ResetPattern           string    `gorm:"size:50" json:"reset_pattern"` // sliding_60s | fixed_minute | sliding_5h | sliding_7d | daily_utc8 | daily_utc
	ConfidenceScore        float64   `gorm:"default:0" json:"confidence_score"`
	Last429At              *time.Time `json:"last_429_at"`
	Last429ObservedRPM     int       `gorm:"default:0" json:"last_429_observed_rpm"`
	LastAdjustAt           *time.Time `json:"last_adjust_at"`
	Locked                 bool      `gorm:"default:false" json:"locked"`
	AdjustmentLog          *string   `gorm:"type:jsonb" json:"adjustment_log,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (LLMModelRateLimit) TableName() string {
	return "llm_model_rate_limits"
}
