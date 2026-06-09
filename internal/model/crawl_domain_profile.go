package model

import (
	"time"

	"gorm.io/gorm"
)

// CrawlDomainProfile stores per-domain crawl scheduling and health state.
type CrawlDomainProfile struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	Domain              string         `gorm:"size:255;not null;uniqueIndex:idx_crawl_domain_profiles_domain,where:deleted_at IS NULL" json:"domain"`
	DefaultDelaySeconds int            `gorm:"default:60" json:"default_delay_seconds"`
	MaxConcurrency      int            `gorm:"default:1" json:"max_concurrency"`
	SuccessRate24h      float64        `gorm:"column:success_rate_24h;default:0" json:"success_rate_24h"`
	BlockRate24h        float64        `gorm:"column:block_rate_24h;default:0" json:"block_rate_24h"`
	LastStatus          string         `gorm:"size:50" json:"last_status,omitempty"`
	NextAllowedAt       *time.Time     `gorm:"index" json:"next_allowed_at,omitempty"`
	RobotsCheckedAt     *time.Time     `json:"robots_checked_at,omitempty"`
	Notes               string         `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

func (CrawlDomainProfile) TableName() string {
	return "crawl_domain_profiles"
}
