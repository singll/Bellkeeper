package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CrawlJobRepository handles DB operations for the crawl queue
type CrawlJobRepository struct {
	db *gorm.DB
}

// NewCrawlJobRepository creates a new crawl job repository
func NewCrawlJobRepository(db *gorm.DB) *CrawlJobRepository {
	return &CrawlJobRepository{db: db}
}

// Enqueue inserts a new job. Skips if a non-terminal job with the same URL exists.
func (r *CrawlJobRepository) Enqueue(job *model.CrawlJob) error {
	if job.URL == "" {
		return fmt.Errorf("URL is required")
	}

	// Dedup: skip if a non-terminal job already exists for this URL
	var count int64
	r.db.Model(&model.CrawlJob{}).
		Where("url = ? AND status NOT IN ?", job.URL, []string{
			string(model.CrawlJobSuccess),
			string(model.CrawlJobSkipped),
			string(model.CrawlJobBlocked),
			string(model.CrawlJobDead),
		}).Count(&count)
	if count > 0 {
		return fmt.Errorf("duplicate: non-terminal job already exists for URL %s", job.URL)
	}

	if job.Status == "" {
		job.Status = model.CrawlJobPending
	}
	if job.MaxRetries == 0 {
		job.MaxRetries = 4
	}
	if job.ChannelType == "" {
		job.ChannelType = "auto"
	}

	return r.db.Create(job).Error
}

// Dequeue claims the next available job using SELECT FOR UPDATE SKIP LOCKED.
// channelType filters by job channel; empty string accepts any.
func (r *CrawlJobRepository) Dequeue(channelType string) (*model.CrawlJob, error) {
	var job model.CrawlJob
	tx := r.db.Model(&model.CrawlJob{}).
		Where("status IN ?", []string{string(model.CrawlJobPending), string(model.CrawlJobRetrying)}).
		Where("next_retry_at IS NULL OR next_retry_at <= ?", time.Now())

	if channelType != "" && channelType != "auto" {
		// Specific channel workers also pick up "auto" jobs
		tx = tx.Where("channel_type IN ?", []string{channelType, "auto"})
	}

	err := tx.
		Order("priority DESC, created_at ASC").
		Limit(1).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		First(&job).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // empty queue
		}
		return nil, err
	}

	now := time.Now()
	r.db.Model(&job).Updates(map[string]interface{}{
		"status":     string(model.CrawlJobRunning),
		"started_at": now,
	})

	job.Status = model.CrawlJobRunning
	job.StartedAt = &now
	return &job, nil
}

// UpdateStatus transitions a job's status and sets relevant timestamps.
func (r *CrawlJobRepository) UpdateStatus(id uint, status model.CrawlJobStatus, updates map[string]interface{}) error {
	if updates == nil {
		updates = make(map[string]interface{})
	}
	updates["status"] = string(status)

	if status == model.CrawlJobSuccess {
		updates["completed_at"] = time.Now()
	}

	return r.db.Model(&model.CrawlJob{}).Where("id = ?", id).Updates(updates).Error
}

// MarkRetry increments retry_count, sets next_retry_at, and status to retrying.
func (r *CrawlJobRepository) MarkRetry(id uint, nextRetryAt time.Time, errType, errMsg string) error {
	return r.db.Model(&model.CrawlJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        string(model.CrawlJobRetrying),
		"retry_count":   gorm.Expr("retry_count + 1"),
		"next_retry_at": nextRetryAt,
		"error_type":    errType,
		"error_message": errMsg,
		"started_at":    nil,
	}).Error
}

// MarkBlocked sets status to 'blocked' and records the block reason.
func (r *CrawlJobRepository) MarkBlocked(id uint, reason string) error {
	return r.db.Model(&model.CrawlJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       string(model.CrawlJobBlocked),
		"block_reason": reason,
		"completed_at": time.Now(),
	}).Error
}

// MarkDead sets status to 'dead' — permanent failure.
func (r *CrawlJobRepository) MarkDead(id uint, errType, errMsg string) error {
	return r.db.Model(&model.CrawlJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":       string(model.CrawlJobDead),
		"error_type":   errType,
		"error_message": errMsg,
		"completed_at": time.Now(),
	}).Error
}

// GetByURL finds a job by URL (for dedup checks).
func (r *CrawlJobRepository) GetByURL(url string) (*model.CrawlJob, error) {
	var job model.CrawlJob
	err := r.db.Where("url = ?", url).First(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

// ListCrawlJobOpts holds filtering options for listing jobs
type ListCrawlJobOpts struct {
	Status      model.CrawlJobStatus
	Domain      string
	ChannelType string
	Page        int
	Limit       int
}

// List returns filtered crawl jobs with pagination.
func (r *CrawlJobRepository) List(opts ListCrawlJobOpts) ([]model.CrawlJob, int64, error) {
	var jobs []model.CrawlJob
	var total int64

	tx := r.db.Model(&model.CrawlJob{})
	if opts.Status != "" {
		tx = tx.Where("status = ?", opts.Status)
	}
	if opts.Domain != "" {
		tx = tx.Where("source_domain = ?", opts.Domain)
	}
	if opts.ChannelType != "" {
		tx = tx.Where("channel_type = ?", opts.ChannelType)
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := 0
	if opts.Page > 1 {
		offset = (opts.Page - 1) * opts.Limit
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	if err := tx.Order("created_at DESC").Offset(offset).Limit(opts.Limit).Find(&jobs).Error; err != nil {
		return nil, 0, err
	}

	return jobs, total, nil
}

// CrawlQueueStats holds aggregate queue statistics
type CrawlQueueStats struct {
	Pending   int64 `json:"pending"`
	Running   int64 `json:"running"`
	Success   int64 `json:"success"`
	Retrying  int64 `json:"retrying"`
	Failed    int64 `json:"failed"`
	Blocked   int64 `json:"blocked"`
	Dead      int64 `json:"dead"`
	Skipped   int64 `json:"skipped"`
	Total     int64 `json:"total"`
}

// Stats returns aggregate counts by status.
func (r *CrawlJobRepository) Stats() (*CrawlQueueStats, error) {
	var stats CrawlQueueStats
	statuses := map[string]*int64{
		string(model.CrawlJobPending):  &stats.Pending,
		string(model.CrawlJobRunning):  &stats.Running,
		string(model.CrawlJobSuccess):  &stats.Success,
		string(model.CrawlJobRetrying): &stats.Retrying,
		string(model.CrawlJobFailed):   &stats.Failed,
		string(model.CrawlJobBlocked):  &stats.Blocked,
		string(model.CrawlJobDead):     &stats.Dead,
		string(model.CrawlJobSkipped):  &stats.Skipped,
	}

	for status, ptr := range statuses {
		var count int64
		r.db.Model(&model.CrawlJob{}).Where("status = ?", status).Count(&count)
		*ptr = count
		stats.Total += count
	}

	return &stats, nil
}

// GetBlockedSince returns blocked jobs created after a given time.
func (r *CrawlJobRepository) GetBlockedSince(since time.Time) ([]model.CrawlJob, error) {
	var jobs []model.CrawlJob
	err := r.db.Where("status = ? AND created_at >= ?", model.CrawlJobBlocked, since).
		Order("created_at DESC").
		Find(&jobs).Error
	return jobs, err
}

// CountByDomainAndStatus counts jobs for a domain with a given status since a time.
func (r *CrawlJobRepository) CountByDomainAndStatus(domain string, status model.CrawlJobStatus, since time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&model.CrawlJob{}).
		Where("source_domain = ? AND status = ? AND created_at >= ?", domain, status, since).
		Count(&count).Error
	return count, err
}

// RecoverOrphanedJobs resets jobs stuck in "running" from a previous crash.
func (r *CrawlJobRepository) RecoverOrphanedJobs() error {
	return r.db.Model(&model.CrawlJob{}).
		Where("status = ?", string(model.CrawlJobRunning)).
		Updates(map[string]interface{}{
			"status":     string(model.CrawlJobPending),
			"started_at": nil,
		}).Error
}

// GetRecentlyBlockedDomains returns domains that have been blocked recently.
func (r *CrawlJobRepository) GetRecentlyBlockedDomains(since time.Time) ([]string, error) {
	var domains []string
	err := r.db.Model(&model.CrawlJob{}).
		Select("DISTINCT source_domain").
		Where("status = ? AND source_domain != '' AND created_at >= ?", model.CrawlJobBlocked, since).
		Find(&domains).Error
	return domains, err
}
