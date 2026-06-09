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
	now := time.Now()
	err := r.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ?", []string{string(model.CrawlJobPending), string(model.CrawlJobRetrying)}).
			Where("next_retry_at IS NULL OR next_retry_at <= ?", now)

		if channelType != "" && channelType != "auto" {
			// Specific channel workers also pick up "auto" jobs
			query = query.Where("channel_type IN ?", []string{channelType, "auto"})
		}

		err := query.
			Order("priority DESC, created_at ASC, id ASC").
			Limit(1).
			First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		result := tx.Model(&model.CrawlJob{}).
			Where("id = ? AND status IN ?", job.ID, []string{string(model.CrawlJobPending), string(model.CrawlJobRetrying)}).
			Updates(map[string]interface{}{
				"status":     string(model.CrawlJobRunning),
				"started_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			job = model.CrawlJob{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if job.ID == 0 {
		return nil, nil // empty queue or lost claim
	}

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

	if status.IsTerminal() {
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

// DelayJob requeues a claimed job without consuming retry budget.
func (r *CrawlJobRepository) DelayJob(id uint, nextRetryAt time.Time, errType, errMsg string) error {
	return r.db.Model(&model.CrawlJob{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":        string(model.CrawlJobRetrying),
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
		"status":        string(model.CrawlJobDead),
		"error_type":    errType,
		"error_message": errMsg,
		"completed_at":  time.Now(),
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
	Since       time.Time // only return jobs updated after this time
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
	if !opts.Since.IsZero() {
		tx = tx.Where("updated_at >= ?", opts.Since)
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
	Pending  int64 `json:"pending"`
	Running  int64 `json:"running"`
	Success  int64 `json:"success"`
	Retrying int64 `json:"retrying"`
	Failed   int64 `json:"failed"`
	Blocked  int64 `json:"blocked"`
	Dead     int64 `json:"dead"`
	Skipped  int64 `json:"skipped"`
	Total    int64 `json:"total"`
}

// CrawlAuditBucket is a grouped count used by crawl audit reports.
type CrawlAuditBucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// CrawlExtractorAudit reports extraction outcomes grouped by extractor.
type CrawlExtractorAudit struct {
	Extractor   string  `json:"extractor"`
	Success     int64   `json:"success"`
	Failed      int64   `json:"failed"`
	Total       int64   `json:"total"`
	SuccessRate float64 `json:"success_rate"`
}

// CrawlAuditStats holds recent crawl health aggregates.
type CrawlAuditStats struct {
	Since         time.Time             `json:"since"`
	Total         int64                 `json:"total"`
	ByStatus      []CrawlAuditBucket    `json:"by_status"`
	TopDomains    []CrawlAuditBucket    `json:"top_domains"`
	TopErrorTypes []CrawlAuditBucket    `json:"top_error_types"`
	Extractors    []CrawlExtractorAudit `json:"extractors"`
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

// Audit returns grouped recent crawl failures and extractor success rates.
func (r *CrawlJobRepository) Audit(since time.Time, limit int) (*CrawlAuditStats, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	base := func() *gorm.DB {
		return r.db.Model(&model.CrawlJob{}).Where("updated_at >= ?", since)
	}

	stats := &CrawlAuditStats{Since: since}
	if err := base().Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	if err := base().
		Select("status AS key, count(*) AS count").
		Group("status").
		Order("count DESC").
		Scan(&stats.ByStatus).Error; err != nil {
		return nil, err
	}

	failureStatuses := []string{
		string(model.CrawlJobFailed),
		string(model.CrawlJobRetrying),
		string(model.CrawlJobBlocked),
		string(model.CrawlJobDead),
	}

	if err := base().
		Where("status IN ? AND source_domain <> ''", failureStatuses).
		Select("source_domain AS key, count(*) AS count").
		Group("source_domain").
		Order("count DESC").
		Limit(limit).
		Scan(&stats.TopDomains).Error; err != nil {
		return nil, err
	}

	errorExpr := "COALESCE(NULLIF(error_type, ''), NULLIF(block_reason, ''), status)"
	if err := base().
		Where("status IN ?", failureStatuses).
		Select(errorExpr + " AS key, count(*) AS count").
		Group(errorExpr).
		Order("count DESC").
		Limit(limit).
		Scan(&stats.TopErrorTypes).Error; err != nil {
		return nil, err
	}

	if err := base().
		Select(`COALESCE(NULLIF(extractor_used, ''), 'unknown') AS extractor,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success,
			SUM(CASE WHEN status IN ('failed', 'retrying', 'blocked', 'dead') THEN 1 ELSE 0 END) AS failed,
			COUNT(*) AS total`).
		Group("COALESCE(NULLIF(extractor_used, ''), 'unknown')").
		Order("total DESC").
		Scan(&stats.Extractors).Error; err != nil {
		return nil, err
	}
	for i := range stats.Extractors {
		denominator := stats.Extractors[i].Success + stats.Extractors[i].Failed
		if denominator > 0 {
			stats.Extractors[i].SuccessRate = float64(stats.Extractors[i].Success) / float64(denominator)
		}
	}

	return stats, nil
}

// GetBlockedSince returns blocked jobs created after a given time.
func (r *CrawlJobRepository) GetBlockedSince(since time.Time) ([]model.CrawlJob, error) {
	var jobs []model.CrawlJob
	err := r.db.Where("status = ? AND created_at >= ?", model.CrawlJobBlocked, since).
		Order("created_at DESC").
		Find(&jobs).Error
	return jobs, err
}

// GetDeadSince returns dead jobs updated after a given time.
// Uses updated_at (not created_at) because dead is a terminal state —
// updated_at reflects when the job transitioned to dead.
func (r *CrawlJobRepository) GetDeadSince(since time.Time) ([]model.CrawlJob, error) {
	var jobs []model.CrawlJob
	err := r.db.Where("status = ? AND updated_at >= ?", model.CrawlJobDead, since).
		Order("updated_at DESC").
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

// CountRunningByDomain counts currently claimed jobs for a domain.
func (r *CrawlJobRepository) CountRunningByDomain(domain string) (int64, error) {
	var count int64
	err := r.db.Model(&model.CrawlJob{}).
		Where("source_domain = ? AND status = ?", domain, string(model.CrawlJobRunning)).
		Count(&count).Error
	return count, err
}

// CountRunningDomainRank returns this job's 1-based rank among running jobs for the domain.
func (r *CrawlJobRepository) CountRunningDomainRank(domain string, jobID uint, startedAt *time.Time) (int64, error) {
	if startedAt == nil {
		return r.CountRunningByDomain(domain)
	}
	var count int64
	err := r.db.Model(&model.CrawlJob{}).
		Where("source_domain = ? AND status = ?", domain, string(model.CrawlJobRunning)).
		Where("started_at < ? OR (started_at = ? AND id <= ?)", *startedAt, *startedAt, jobID).
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

// RecoverStaleRunningJobs resets jobs that have been in "running" status
// longer than staleTimeout. Unlike RecoverOrphanedJobs (which resets all
// running jobs for crash recovery), this targets jobs stuck due to goroutine leaks.
func (r *CrawlJobRepository) RecoverStaleRunningJobs(staleTimeout time.Duration) (int64, error) {
	cutoff := time.Now().Add(-staleTimeout)
	result := r.db.Model(&model.CrawlJob{}).
		Where("status = ? AND started_at < ?", string(model.CrawlJobRunning), cutoff).
		Updates(map[string]interface{}{
			"status":     string(model.CrawlJobPending),
			"started_at": nil,
		})
	return result.RowsAffected, result.Error
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
