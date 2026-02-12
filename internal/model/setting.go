package model

import (
	"time"

	"gorm.io/gorm"
)

// Setting represents a configuration setting
type Setting struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	Key         string         `gorm:"size:255;uniqueIndex;not null" json:"key"`
	Value       string         `gorm:"type:text" json:"value"`
	ValueType   string         `gorm:"size:50;default:'string'" json:"value_type"` // string, int, bool, json
	Category    string         `gorm:"size:100;index" json:"category"`             // api, feature, ui
	Description string         `gorm:"type:text" json:"description"`
	IsSecret    bool           `gorm:"default:false" json:"is_secret"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies table name
func (Setting) TableName() string {
	return "settings"
}

// MaskedValue returns masked value for secret settings
func (s *Setting) MaskedValue() string {
	if s.IsSecret && len(s.Value) > 0 {
		return "******"
	}
	return s.Value
}
