package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type LogAlertRule struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Name          string         `gorm:"size:128;not null" json:"name"`
	Condition     datatypes.JSON `gorm:"type:jsonb;not null" json:"condition"` // {"module":"rss_fetch","level":"error","threshold":5,"window_minutes":30}
	NotifyChannel string         `gorm:"size:64" json:"notify_channel"`       // "daily", "alerts"
	IsActive      bool           `gorm:"default:true" json:"is_active"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LogAlertRule) TableName() string {
	return "log_alert_rules"
}