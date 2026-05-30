package model

import "time"

// LLMTokenUsageDaily aggregates token usage per token per day.
type LLMTokenUsageDaily struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TokenID         uint      `gorm:"index;not null" json:"token_id"`
	Date            time.Time `gorm:"type:date;index;not null" json:"date"`
	Requests        int       `gorm:"default:0" json:"requests"`
	PromptTokens    int       `gorm:"default:0" json:"prompt_tokens"`
	CompletionTokens int      `gorm:"default:0" json:"completion_tokens"`
	CachedTokens    int       `gorm:"default:0" json:"cached_tokens"`
	CostCents       int       `gorm:"default:0" json:"cost_cents"`
	ErrorCount      int       `gorm:"default:0" json:"error_count"`
}

func (LLMTokenUsageDaily) TableName() string {
	return "llm_token_usage_daily"
}
