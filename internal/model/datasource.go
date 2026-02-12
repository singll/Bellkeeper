package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DataSource represents a data source for knowledge collection
type DataSource struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:200;not null" json:"name"`
	URL         string         `gorm:"size:1000;not null" json:"url"`
	Type        string         `gorm:"size:50;default:'website'" json:"type"`
	Category    string         `gorm:"size:100;index" json:"category"`
	Description string         `gorm:"type:text" json:"description"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	Metadata    datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	Tags []Tag `gorm:"many2many:datasource_tags;" json:"tags,omitempty"`
}

// TableName specifies table name
func (DataSource) TableName() string {
	return "data_sources"
}
