package model

import "time"

// LLMProxyLog records each LLM API proxy request for observability
type LLMProxyLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ChannelName  string    `gorm:"size:100;index" json:"channel_name"`
	Model        string    `gorm:"size:100;index" json:"model"`
	RequestPath  string    `gorm:"size:500" json:"request_path"`
	StatusCode   int       `json:"status_code"`
	IsRateLimit  bool      `gorm:"index" json:"is_rate_limit"`
	RetryCount   int       `json:"retry_count"`
	DurationMs   int       `json:"duration_ms"`
	PromptTokens int       `json:"prompt_tokens,omitempty"`
	CompTokens   int       `json:"comp_tokens,omitempty"`
	ErrorMessage string    `gorm:"type:text" json:"error_message,omitempty"`
	CallerID     string    `gorm:"size:100;index" json:"caller_id"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (LLMProxyLog) TableName() string {
	return "llm_proxy_logs"
}
