package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// CrawlSourceType defines the type of crawl source
type CrawlSourceType string

const (
	CrawlSourceRSS    CrawlSourceType = "rss"
	CrawlSourceURL    CrawlSourceType = "url_seed"
	CrawlSourceAPI    CrawlSourceType = "api"
	CrawlSourceCustom CrawlSourceType = "custom"
)

// CrawlSource represents a generic content source for the crawler.
// This is the future expansion point for URL seeds, API sources, etc.
// Currently RSS sources are managed via RSSFeed, but this model will
// unify all source types when the crawler expands beyond RSS.
type CrawlSource struct {
	ID                  uint            `gorm:"primaryKey" json:"id"`
	Name                string          `gorm:"size:200;not null" json:"name"`
	SourceType          CrawlSourceType `gorm:"size:50;not null;index" json:"source_type"`
	URL                 string          `gorm:"size:1000;not null" json:"url"`
	Category            string          `gorm:"size:100;index" json:"category"`
	Tags                datatypes.JSON  `gorm:"type:jsonb" json:"tags,omitempty"`        // JSON array of tag names
	IsActive            bool            `gorm:"default:true" json:"is_active"`
	HealthScore         int             `gorm:"default:100" json:"health_score"`        // 0-100
	ConsecutiveFailures int             `gorm:"default:0" json:"consecutive_failures"`
	LastFailureReason   string          `gorm:"size:500" json:"last_failure_reason,omitempty"`
	IsPaused            bool            `gorm:"default:false" json:"is_paused"`
	PausedAt            *time.Time      `json:"paused_at,omitempty"`
	MaxConcurrency      int             `gorm:"default:3" json:"max_concurrency"`       // per-source fetch concurrency limit
	TotalFetched        int             `gorm:"default:0" json:"total_fetched"`         // lifetime successful fetches count
	TotalFailed         int             `gorm:"default:0" json:"total_failed"`          // lifetime failed fetches count
	LastFetchedAt       *time.Time      `json:"last_fetched_at,omitempty"`
	FetchInterval       int             `gorm:"default:60" json:"fetch_interval"`       // minutes between fetches
	Metadata            datatypes.JSON  `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	DeletedAt           gorm.DeletedAt  `gorm:"index" json:"-"`
}

func (CrawlSource) TableName() string {
	return "crawl_sources"
}