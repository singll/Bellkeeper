package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
)

// LLMToken represents an API key for accessing the LLM Proxy.
// Each token has independent quotas, model allowlists, and expiration.
type LLMToken struct {
	ID                   uint           `gorm:"primaryKey" json:"id"`
	Name                 string         `gorm:"size:100;not null" json:"name"`
	KeyHash              string         `gorm:"size:64;uniqueIndex;not null" json:"-"` // sha256 of full key
	KeyPrefix            string         `gorm:"size:16" json:"key_prefix"`             // first 8 chars for display
	CallerID             string         `gorm:"size:100;uniqueIndex;not null" json:"caller_id"`
	AllowedModels        string         `gorm:"type:text" json:"allowed_models"` // JSON array, empty = all
	AllowedGroups        string         `gorm:"type:text" json:"allowed_groups"` // JSON array, empty = all
	QuotaRequestsDaily   int            `gorm:"default:0" json:"quota_requests_daily"`   // 0 = unlimited
	QuotaTokensDaily     int            `gorm:"default:0" json:"quota_tokens_daily"`     // 0 = unlimited
	QuotaCostMonthlyCents int           `gorm:"default:0" json:"quota_cost_monthly_cents"` // 0 = unlimited
	ExpiresAt            *time.Time     `json:"expires_at"`
	Enabled              bool           `gorm:"default:true" json:"enabled"`
	LastUsedAt           *time.Time     `json:"last_used_at"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LLMToken) TableName() string {
	return "llm_tokens"
}

// HashKey computes the sha256 hash of a raw API key.
func HashKey(raw string) string {
	h := sha256.New()
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}

// GetAllowedModels parses the JSON allowed_models string.
func (t *LLMToken) GetAllowedModels() []string {
	return parseJSONStringArray(t.AllowedModels)
}

// SetAllowedModels serializes a slice into JSON.
func (t *LLMToken) SetAllowedModels(models []string) {
	t.AllowedModels = marshalJSONStringArray(models)
}

// GetAllowedGroups parses the JSON allowed_groups string.
func (t *LLMToken) GetAllowedGroups() []string {
	return parseJSONStringArray(t.AllowedGroups)
}

// SetAllowedGroups serializes a slice into JSON.
func (t *LLMToken) SetAllowedGroups(groups []string) {
	t.AllowedGroups = marshalJSONStringArray(groups)
}
