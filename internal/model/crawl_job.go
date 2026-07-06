package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// CrawlJobStatus defines the status of a crawl job
type CrawlJobStatus string

const (
	CrawlJobPending  CrawlJobStatus = "pending"
	CrawlJobRunning  CrawlJobStatus = "running"
	CrawlJobCrawled  CrawlJobStatus = "crawled" // 1.0: 抓取+提取完成，待 extract-worker 入库（中间态）
	CrawlJobSuccess  CrawlJobStatus = "success"
	CrawlJobFailed   CrawlJobStatus = "failed"
	CrawlJobRetrying CrawlJobStatus = "retrying"
	CrawlJobSkipped  CrawlJobStatus = "skipped"
	CrawlJobBlocked  CrawlJobStatus = "blocked" // paywall/anti-crawl detected
	CrawlJobDead     CrawlJobStatus = "dead"    // permanently failed, no more retries
)

// TerminalStatuses are statuses that indicate a job is finished and won't change
var TerminalStatuses = []CrawlJobStatus{CrawlJobSuccess, CrawlJobSkipped, CrawlJobBlocked, CrawlJobDead}

// IsTerminal returns true if the job status is a terminal state
func (s CrawlJobStatus) IsTerminal() bool {
	for _, ts := range TerminalStatuses {
		if s == ts {
			return true
		}
	}
	return false
}

// CrawlJob tracks individual crawl tasks in the persistent queue.
type CrawlJob struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	SourceID      uint           `gorm:"index;not null" json:"source_id"`
	URL           string         `gorm:"size:1000;not null;index" json:"url"`
	Priority      int            `gorm:"default:0" json:"priority"`
	RetryCount    int            `gorm:"default:0" json:"retry_count"`
	MaxRetries    int            `gorm:"default:4" json:"max_retries"`
	Status        CrawlJobStatus `gorm:"size:20;index;not null" json:"status"`
	ErrorMessage  string         `gorm:"size:500" json:"error_message,omitempty"`
	QualityScore  float64        `json:"quality_score,omitempty"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	NextRetryAt   *time.Time     `json:"next_retry_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	// Extended fields for crawl queue
	Title         string         `gorm:"size:1000" json:"title,omitempty"`
	ErrorType     string         `gorm:"size:50;index" json:"error_type,omitempty"`
	ChannelType   string         `gorm:"size:30;index" json:"channel_type,omitempty"`
	ContentLength int            `gorm:"default:0" json:"content_length,omitempty"`
	ExtractorUsed string         `gorm:"size:50" json:"extractor_used,omitempty"`
	SourceDomain  string         `gorm:"size:255;index" json:"source_domain,omitempty"`
	BlockReason   string         `gorm:"size:500" json:"block_reason,omitempty"`
	Metadata      datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
}

func (CrawlJob) TableName() string {
	return "crawl_jobs"
}
