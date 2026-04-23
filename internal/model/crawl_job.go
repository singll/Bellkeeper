package model

import (
	"time"

	"gorm.io/gorm"
)

// CrawlJobStatus defines the status of a crawl job
type CrawlJobStatus string

const (
	CrawlJobPending   CrawlJobStatus = "pending"
	CrawlJobRunning   CrawlJobStatus = "running"
	CrawlJobSuccess   CrawlJobStatus = "success"
	CrawlJobFailed    CrawlJobStatus = "failed"
	CrawlJobRetrying  CrawlJobStatus = "retrying"
	CrawlJobSkipped   CrawlJobStatus = "skipped"
)

// CrawlJob tracks individual crawl tasks for observability and retry management.
type CrawlJob struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	SourceID      uint           `gorm:"index;not null" json:"source_id"`       // FK to crawl_sources or rss_feeds
	URL           string         `gorm:"size:1000;not null;index" json:"url"`
	Priority      int            `gorm:"default:0" json:"priority"`             // higher = more urgent
	RetryCount    int            `gorm:"default:0" json:"retry_count"`
	MaxRetries    int            `gorm:"default:4" json:"max_retries"`
	Status        CrawlJobStatus `gorm:"size:20;index;not null" json:"status"`
	ErrorMessage  string         `gorm:"size:500" json:"error_message,omitempty"`
	QualityScore  float64        `json:"quality_score,omitempty"`               // LLM quality gate score (0-1)
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	NextRetryAt   *time.Time     `json:"next_retry_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (CrawlJob) TableName() string {
	return "crawl_jobs"
}