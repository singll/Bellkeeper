package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type CrawlFailureStatus string

const (
	CrawlFailureCooling      CrawlFailureStatus = "cooling"
	CrawlFailureAbandoned    CrawlFailureStatus = "abandoned"
	CrawlFailureRecoverable  CrawlFailureStatus = "recoverable"
)

type CrawlFailure struct {
	ID               uint               `gorm:"primaryKey" json:"id"`
	URL              string             `gorm:"size:1000;not null;uniqueIndex:idx_crawl_failures_url,where:deleted_at IS NULL" json:"url"`
	SourceDomain     string             `gorm:"size:255;not null;index" json:"source_domain"`
	Title            string             `gorm:"size:1000" json:"title,omitempty"`
	SourceID         uint               `gorm:"index" json:"source_id"`
	FailureCount     int                `gorm:"default:1" json:"failure_count"`
	LastErrorType    string             `gorm:"size:50" json:"last_error_type,omitempty"`
	LastErrorMessage string             `gorm:"size:500" json:"last_error_message,omitempty"`
	LastFailedAt     time.Time          `json:"last_failed_at"`
	Status           CrawlFailureStatus `gorm:"size:20;not null;default:recoverable;index" json:"status"`
	RecoveryAttempts int                `gorm:"default:0" json:"recovery_attempts"`
	RequestOverrides datatypes.JSON     `gorm:"type:jsonb" json:"request_overrides,omitempty"`
	AnalysisResult   string             `gorm:"type:text" json:"analysis_result,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
	DeletedAt        gorm.DeletedAt     `gorm:"index" json:"-"`
}

func (CrawlFailure) TableName() string {
	return "crawl_failures"
}
