package model

import "time"

// ActivityLog records key operations across all Bellkeeper modules for observability.
type ActivityLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Module     string    `gorm:"size:50;index" json:"module"`
	Action     string    `gorm:"size:100;index" json:"action"`
	Status     string    `gorm:"size:20;index" json:"status"`
	Summary    string    `gorm:"size:500" json:"summary"`
	Detail     string    `gorm:"type:text" json:"detail,omitempty"`
	RefID      string    `gorm:"size:100;index" json:"ref_id,omitempty"`
	DurationMs int       `json:"duration_ms,omitempty"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

func (ActivityLog) TableName() string {
	return "activity_logs"
}
