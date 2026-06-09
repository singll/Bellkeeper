package repository

import (
	"errors"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

// CrawlDomainProfileRepository handles persistent per-domain crawl profiles.
type CrawlDomainProfileRepository struct {
	db *gorm.DB
}

func NewCrawlDomainProfileRepository(db *gorm.DB) *CrawlDomainProfileRepository {
	return &CrawlDomainProfileRepository{db: db}
}

func (r *CrawlDomainProfileRepository) FindOrCreate(domain string, defaultDelaySeconds, defaultMaxConcurrency int) (*model.CrawlDomainProfile, error) {
	if defaultDelaySeconds <= 0 {
		defaultDelaySeconds = 60
	}
	if defaultMaxConcurrency <= 0 {
		defaultMaxConcurrency = 1
	}

	var profile model.CrawlDomainProfile
	err := r.db.Where("domain = ?", domain).First(&profile).Error
	if err == nil {
		changed := false
		if profile.DefaultDelaySeconds <= 0 {
			profile.DefaultDelaySeconds = defaultDelaySeconds
			changed = true
		}
		if profile.MaxConcurrency <= 0 {
			profile.MaxConcurrency = defaultMaxConcurrency
			changed = true
		}
		if changed {
			if saveErr := r.db.Save(&profile).Error; saveErr != nil {
				return nil, saveErr
			}
		}
		return &profile, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	profile = model.CrawlDomainProfile{
		Domain:              domain,
		DefaultDelaySeconds: defaultDelaySeconds,
		MaxConcurrency:      defaultMaxConcurrency,
	}
	if err := r.db.Create(&profile).Error; err != nil {
		if err2 := r.db.Where("domain = ?", domain).First(&profile).Error; err2 == nil {
			return &profile, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *CrawlDomainProfileRepository) RecordStart(domain string, nextAllowedAt time.Time) error {
	return r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", domain).
		Updates(map[string]interface{}{
			"last_status":     "running",
			"next_allowed_at": nextAllowedAt,
		}).Error
}

func (r *CrawlDomainProfileRepository) RecordOutcome(domain, status, notes string, nextAllowedAt *time.Time) error {
	updates := map[string]interface{}{
		"last_status": status,
	}
	if notes != "" {
		updates["notes"] = notes
	}
	if nextAllowedAt != nil {
		updates["next_allowed_at"] = *nextAllowedAt
	}
	return r.db.Model(&model.CrawlDomainProfile{}).Where("domain = ?", domain).Updates(updates).Error
}

func (r *CrawlDomainProfileRepository) RefreshRates(domain string, since time.Time) error {
	var total int64
	if err := r.db.Model(&model.CrawlJob{}).
		Where("source_domain = ? AND updated_at >= ?", domain, since).
		Count(&total).Error; err != nil {
		return err
	}
	if total == 0 {
		return r.db.Model(&model.CrawlDomainProfile{}).
			Where("domain = ?", domain).
			Updates(map[string]interface{}{
				"success_rate_24h": 0,
				"block_rate_24h":   0,
			}).Error
	}

	var success int64
	if err := r.db.Model(&model.CrawlJob{}).
		Where("source_domain = ? AND updated_at >= ? AND status IN ?", domain, since, []string{
			string(model.CrawlJobSuccess),
			string(model.CrawlJobSkipped),
		}).
		Count(&success).Error; err != nil {
		return err
	}
	var blocked int64
	if err := r.db.Model(&model.CrawlJob{}).
		Where("source_domain = ? AND updated_at >= ? AND status = ?", domain, since, string(model.CrawlJobBlocked)).
		Count(&blocked).Error; err != nil {
		return err
	}

	return r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", domain).
		Updates(map[string]interface{}{
			"success_rate_24h": float64(success) / float64(total),
			"block_rate_24h":   float64(blocked) / float64(total),
		}).Error
}

func (r *CrawlDomainProfileRepository) List(page, limit int) ([]model.CrawlDomainProfile, int64, error) {
	var profiles []model.CrawlDomainProfile
	var total int64
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	tx := r.db.Model(&model.CrawlDomainProfile{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("updated_at DESC, domain ASC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&profiles).Error; err != nil {
		return nil, 0, err
	}
	return profiles, total, nil
}
