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

	// Relations
	Tags []Tag `gorm:"many2many:rss_tags;" json:"tags,omitempty"`
}

// TableName specifies table name
func (RSSFeed) TableName() string {
	return "rss_feeds"
}
