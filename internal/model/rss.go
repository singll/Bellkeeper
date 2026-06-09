package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RSSFeed represents an RSS feed subscription
type RSSFeed struct {
	ID                   uint           `gorm:"primaryKey" json:"id"`
	Name                 string         `gorm:"size:200;not null" json:"name"`
	URL                  string         `gorm:"size:1000;uniqueIndex;not null" json:"url"`
	Category             string         `gorm:"size:100;index" json:"category"`
	Description          string         `gorm:"type:text" json:"description"`
	IsActive             bool           `gorm:"default:true" json:"is_active"`
	LastFetchedAt        *time.Time     `json:"last_fetched_at,omitempty"`
	FetchIntervalMinutes int            `gorm:"default:60" json:"fetch_interval_minutes"`
	Metadata             datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`

	// Health tracking fields
	HealthScore           int        `gorm:"default:100" json:"health_score"`                     // 0-100, decreases on consecutive failures
	ConsecutiveFailures   int        `gorm:"default:0" json:"consecutive_failures"`                // resets to 0 on success
	LastFailureReason     string     `gorm:"size:500" json:"last_failure_reason,omitempty"`         // most recent error message
	IsPaused              bool       `gorm:"default:false" json:"is_paused"`                       // auto-paused when health_score < threshold
	PausedAt              *time.Time `json:"paused_at,omitempty"`                                  // when auto-pause occurred
	MaxConcurrency        int            `gorm:"default:3" json:"max_concurrency"`                     // per-source fetch concurrency limit
	TotalFetched          int            `gorm:"default:0" json:"total_fetched"`                       // lifetime successful fetches count
	TotalFailed           int            `gorm:"default:0" json:"total_failed"`                        // lifetime failed fetches count
	RSSHubParams          datatypes.JSON `gorm:"type:jsonb" json:"rsshub_params,omitempty"`            // RSSHub query params: {"limit":200,"filter":"AI","mode":"fulltext"}

	// Relations
	Tags []Tag `gorm:"many2many:rss_tags;" json:"tags,omitempty"`
}

// TableName specifies table name
func (RSSFeed) TableName() string {
	return "rss_feeds"
}
