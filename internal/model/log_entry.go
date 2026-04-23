package model

import (
	"time"

	"gorm.io/datatypes"
)

type LogEntry struct {
	ID         uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	SourceID   uint           `gorm:"index" json:"source_id"`
	Source     *LogSource     `gorm:"foreignKey:SourceID" json:"source,omitempty"`
	Module     string         `gorm:"size:64;index" json:"module"`
	Action     string         `gorm:"size:64" json:"action"`
	Level      string         `gorm:"size:16;index" json:"level"` // "debug", "info", "warn", "error"
	Status     string         `gorm:"size:32;index" json:"status"` // "success", "failed", "skipped"
	Summary    string         `gorm:"size:500" json:"summary"`
	Detail     datatypes.JSON `gorm:"type:jsonb" json:"detail,omitempty"`
	RefID      string         `gorm:"size:128;index" json:"ref_id,omitempty"`
	DurationMs int            `json:"duration_ms,omitempty"`
	TraceID    string         `gorm:"size:64;index" json:"trace_id,omitempty"`
	CreatedAt  time.Time      `gorm:"autoCreateTime;index" json:"created_at"`
}

func (LogEntry) TableName() string {
	return "log_entries"
}