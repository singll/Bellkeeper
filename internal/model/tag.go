package model

import (
	"time"

	"gorm.io/gorm"
)

// Tag represents a tag for categorizing content
type Tag struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"uniqueIndex;size:100;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Color       string         `gorm:"size:20;default:'#409EFF'" json:"color"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	DataSources     []DataSource     `gorm:"many2many:datasource_tags;" json:"data_sources,omitempty"`
	RSSFeeds        []RSSFeed        `gorm:"many2many:rss_tags;" json:"rss_feeds,omitempty"`
	DatasetMappings []DatasetMapping `gorm:"many2many:dataset_mapping_tags;" json:"dataset_mappings,omitempty"`
}

// TableName specifies table name
func (Tag) TableName() string {
	return "tags"
}
