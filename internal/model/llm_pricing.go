package model

import "time"

// LLMModelPricing stores per-channel, per-model pricing configuration.
type LLMModelPricing struct {
	ID                        uint      `gorm:"primaryKey" json:"id"`
	ChannelName               string    `gorm:"size:100;index;not null" json:"channel_name"`
	Model                     string    `gorm:"size:200;index;not null" json:"model"`
	InputPricePer1MCents      int       `gorm:"default:0" json:"input_price_per_1m_cents"`
	OutputPricePer1MCents     int       `gorm:"default:0" json:"output_price_per_1m_cents"`
	CachedInputPricePer1MCents int      `gorm:"default:0" json:"cached_input_price_per_1m_cents"` // 0 = use input_price * 0.1
	Currency                  string    `gorm:"size:10;default:'USD'" json:"currency"`
	EffectiveFrom             time.Time `json:"effective_from"`
	Notes                     string    `gorm:"type:text" json:"notes"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

func (LLMModelPricing) TableName() string {
	return "llm_model_pricing"
}

// GetCachedInputPrice returns the cached input price, falling back to input * 0.1 if not set.
func (p *LLMModelPricing) GetCachedInputPrice() int {
	if p.CachedInputPricePer1MCents > 0 {
		return p.CachedInputPricePer1MCents
	}
	return p.InputPricePer1MCents / 10
}
