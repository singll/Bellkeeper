package model

import (
	"time"

	"gorm.io/datatypes"
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
	FailureCount        int            `gorm:"default:0" json:"failure_count"`
	// 1.0 域名健康度（§2.1.1）：ConsecutiveFailures 连续失败计数（成功归零）；
	// HealthScore 0-100（失败递减、成功递增）；IsPaused 健康度跌破阈值时自动暂停
	// （DequeueFair 过滤 paused 域名，避免持续浪费 worker）。
	ConsecutiveFailures int            `gorm:"default:0" json:"consecutive_failures"`
	HealthScore         int            `gorm:"default:100" json:"health_score"`
	IsPaused            bool           `gorm:"default:false" json:"is_paused"`
	PausedReason        string         `gorm:"size:255" json:"paused_reason,omitempty"`
	PausedAt            *time.Time     `json:"paused_at,omitempty"`
	RequestOverrides    datatypes.JSON `gorm:"type:jsonb" json:"request_overrides,omitempty"`
	AnalysisResult      string         `gorm:"type:text" json:"analysis_result,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

func (CrawlDomainProfile) TableName() string {
	return "crawl_domain_profiles"
}
