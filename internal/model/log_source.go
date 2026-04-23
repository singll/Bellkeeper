package model

import (
	"time"

	"gorm.io/gorm"
)

type LogSource struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Name        string         `gorm:"size:64;not null;uniqueIndex" json:"name"`
	SourceType  string         `gorm:"size:32;not null" json:"source_type"` // "internal", "n8n", "external"
	Description string         `gorm:"size:256" json:"description"`
	APIKey      string         `gorm:"size:128" json:"-"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LogSource) TableName() string {
	return "log_sources"
}