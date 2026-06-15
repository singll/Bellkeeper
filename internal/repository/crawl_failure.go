package repository

import (
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type CrawlFailureRepository struct {
	db *gorm.DB
}

func NewCrawlFailureRepository(db *gorm.DB) *CrawlFailureRepository {
	return &CrawlFailureRepository{db: db}
}

func (r *CrawlFailureRepository) Create(failure *model.CrawlFailure) error {
	return r.db.Create(failure).Error
}

func (r *CrawlFailureRepository) FindByURL(url string) (*model.CrawlFailure, error) {
	var failure model.CrawlFailure
	err := r.db.Where("url = ?", url).First(&failure).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &failure, nil
}

func (r *CrawlFailureRepository) FindByDomain(domain string) (*model.CrawlFailure, error) {
	var failure model.CrawlFailure
	err := r.db.Where("source_domain = ? AND status != ?", domain, string(model.CrawlFailureAbandoned)).
		Order("updated_at DESC").
		First(&failure).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &failure, nil
}

// UpsertFromJob records or bumps a failure entry for a job. errType/errMsg are
// passed explicitly (the classified outcome at the call site) rather than read
// from the job row, which may still hold a stale error from a prior attempt.
func (r *CrawlFailureRepository) UpsertFromJob(job *model.CrawlJob, errType, errMsg string) error {
	if job == nil {
		return nil
	}

	var existing model.CrawlFailure
	err := r.db.Where("url = ?", job.URL).First(&existing).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("lookup crawl failure: %w", err)
	}

	now := time.Now()

	if err == gorm.ErrRecordNotFound {
		failure := &model.CrawlFailure{
			URL:              job.URL,
			SourceDomain:     job.SourceDomain,
			Title:            job.Title,
			SourceID:         job.SourceID,
			FailureCount:     1,
			LastErrorType:    errType,
			LastErrorMessage: errMsg,
			LastFailedAt:     now,
			Status:           model.CrawlFailureRecoverable,
		}
		return r.db.Create(failure).Error
	}

	updates := map[string]interface{}{
		"failure_count":      gorm.Expr("failure_count + 1"),
		"last_error_type":    errType,
		"last_error_message": errMsg,
		"last_failed_at":     now,
		"updated_at":         now,
	}
	return r.db.Model(&model.CrawlFailure{}).Where("id = ?", existing.ID).Updates(updates).Error
}

// ResolveByURL drops a URL's failure record once it crawls successfully. The
// failure table is a human to-do list, so a resolved URL falls off it.
func (r *CrawlFailureRepository) ResolveByURL(url string) error {
	return r.db.Where("url = ?", url).Delete(&model.CrawlFailure{}).Error
}

type ListCrawlFailuresOpts struct {
	Domain string
	Status model.CrawlFailureStatus
	Page   int
	Limit  int
}

func (r *CrawlFailureRepository) List(opts ListCrawlFailuresOpts) ([]model.CrawlFailure, int64, error) {
	var failures []model.CrawlFailure
	var total int64

	tx := r.db.Model(&model.CrawlFailure{})
	if opts.Domain != "" {
		tx = tx.Where("source_domain = ?", opts.Domain)
	}
	if opts.Status != "" {
		tx = tx.Where("status = ?", opts.Status)
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := opts.Page
	if page < 1 {
		page = 1
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	if err := tx.Order("updated_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&failures).Error; err != nil {
		return nil, 0, err
	}

	return failures, total, nil
}

func (r *CrawlFailureRepository) GetByID(id uint) (*model.CrawlFailure, error) {
	var failure model.CrawlFailure
	if err := r.db.Where("id = ?", id).First(&failure).Error; err != nil {
		return nil, err
	}
	return &failure, nil
}

func (r *CrawlFailureRepository) MarkRecoveryAttempt(id uint) error {
	return r.db.Model(&model.CrawlFailure{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"recovery_attempts": gorm.Expr("recovery_attempts + 1"),
			"status":            string(model.CrawlFailureCooling),
			"updated_at":        time.Now(),
		}).Error
}

func (r *CrawlFailureRepository) MarkAbandoned(id uint) error {
	return r.db.Model(&model.CrawlFailure{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     string(model.CrawlFailureAbandoned),
			"updated_at": time.Now(),
		}).Error
}

func (r *CrawlFailureRepository) MarkRecoverable(id uint) error {
	return r.db.Model(&model.CrawlFailure{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     string(model.CrawlFailureRecoverable),
			"updated_at": time.Now(),
		}).Error
}
