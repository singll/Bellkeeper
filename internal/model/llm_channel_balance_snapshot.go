package model

import "time"

// LLMChannelBalanceSnapshot is a point-in-time record of a channel's upstream
// balance, captured periodically from the balance manager. It backs the balance
// history endpoint and trend analysis. BalanceRaw holds the full balance.Info JSON.
type LLMChannelBalanceSnapshot struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ChannelID    uint      `gorm:"index:idx_bal_channel_fetched" json:"channel_id"`
	ChannelName  string    `gorm:"size:100;index" json:"channel_name"`
	BalanceUSD   float64   `json:"balance_usd"`
	Currency     string    `gorm:"size:20" json:"currency"`
	TotalGranted float64   `json:"total_granted"`
	TotalUsed    float64   `json:"total_used"`
	BalanceRaw   string    `gorm:"type:text" json:"balance_raw"` // full balance.Info JSON
	LatencyMs    int       `json:"latency_ms"`
	FetchedAt    time.Time `gorm:"index:idx_bal_channel_fetched" json:"fetched_at"`
	CreatedAt    time.Time `json:"created_at"`
}

func (LLMChannelBalanceSnapshot) TableName() string {
	return "llm_channel_balance_snapshots"
}
