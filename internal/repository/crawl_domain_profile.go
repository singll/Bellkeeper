package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
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

// FindByDomain returns the profile for a domain, or (nil, nil) if none exists.
// Unlike FindOrCreate it never inserts a row — safe to call on the hot path.
func (r *CrawlDomainProfileRepository) FindByDomain(domain string) (*model.CrawlDomainProfile, error) {
	var profile model.CrawlDomainProfile
	err := r.db.Where("domain = ?", domain).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// FindCoolingWithoutOverrides returns domains currently cooling (next_allowed_at
// in the future) that have no request_overrides yet — i.e. candidates for the
// rule optimizer to analyze. Ordered by failure_count so the worst offenders go first.
func (r *CrawlDomainProfileRepository) FindCoolingWithoutOverrides(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	var domains []string
	err := r.db.Model(&model.CrawlDomainProfile{}).
		Where("next_allowed_at IS NOT NULL AND next_allowed_at > ?", time.Now()).
		Where("request_overrides IS NULL OR request_overrides::text IN ('null', '{}')").
		Order("failure_count DESC").
		Limit(limit).
		Pluck("domain", &domains).Error
	return domains, err
}

// EnterCooling marks a domain as cooling: increments failure_count and pushes
// next_allowed_at out by exponential backoff (base*2^(n-1), capped at max).
// 1.0 同时递增 ConsecutiveFailures、递减 HealthScore（健康度评估依据）。
func (r *CrawlDomainProfileRepository) EnterCooling(domain string, base, max time.Duration) error {
	if _, err := r.FindOrCreate(domain, 0, 0); err != nil {
		return err
	}
	if err := r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", domain).
		Updates(map[string]interface{}{
			"failure_count":         gorm.Expr("failure_count + 1"),
			"consecutive_failures":  gorm.Expr("consecutive_failures + 1"),
			"health_score":          gorm.Expr("GREATEST(health_score - 10, 0)"),
		}).Error; err != nil {
		return err
	}

	var profile model.CrawlDomainProfile
	if err := r.db.Where("domain = ?", domain).First(&profile).Error; err != nil {
		return err
	}

	dur := base
	for i := 1; i < profile.FailureCount; i++ {
		dur *= 2
		if dur >= max {
			dur = max
			break
		}
	}
	if dur <= 0 || dur > max {
		dur = max
	}

	next := time.Now().Add(dur)
	return r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", domain).
		Updates(map[string]interface{}{
			"next_allowed_at": next,
			"last_status":     "cooling",
		}).Error
}

// ClearCooling resets cooling state after a success: zeroes failure_count and clears next_allowed_at.
// 1.0 同时重置 ConsecutiveFailures、回血 HealthScore（成功恢复健康度）。
func (r *CrawlDomainProfileRepository) ClearCooling(domain string) error {
	return r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", domain).
		Updates(map[string]interface{}{
			"failure_count":        0,
			"consecutive_failures": 0,
			"health_score":         gorm.Expr("LEAST(health_score + 20, 100)"),
			"next_allowed_at":      nil,
		}).Error
}

// EvaluateDomainHealth 按阈值评估域名健康度并自动暂停/恢复（1.0 §2.1.1）。
// ConsecutiveFailures≥pauseThreshold → 暂停；HealthScore≥resumeThreshold 且已暂停 → 恢复。
// 返回 (action, error)：action ∈ {"paused","resumed","none"}。
func (r *CrawlDomainProfileRepository) EvaluateDomainHealth(domain string, pauseThreshold, resumeThreshold int) (string, error) {
	if pauseThreshold <= 0 {
		pauseThreshold = 5
	}
	if resumeThreshold <= 0 {
		resumeThreshold = 30
	}
	var profile model.CrawlDomainProfile
	if err := r.db.Where("domain = ?", domain).First(&profile).Error; err != nil {
		return "none", err
	}
	if !profile.IsPaused && profile.ConsecutiveFailures >= pauseThreshold {
		now := time.Now()
		reason := fmt.Sprintf("consecutive_failures=%d >= %d", profile.ConsecutiveFailures, pauseThreshold)
		if err := r.db.Model(&model.CrawlDomainProfile{}).
			Where("domain = ?", domain).
			Updates(map[string]interface{}{
				"is_paused":     true,
				"paused_reason": reason,
				"paused_at":     &now,
			}).Error; err != nil {
			return "none", err
		}
		return "paused", nil
	}
	if profile.IsPaused && profile.HealthScore >= resumeThreshold {
		if err := r.db.Model(&model.CrawlDomainProfile{}).
			Where("domain = ?", domain).
			Updates(map[string]interface{}{
				"is_paused":     false,
				"paused_reason": "",
				"paused_at":     nil,
			}).Error; err != nil {
			return "none", err
		}
		return "resumed", nil
	}
	return "none", nil
}

// IsCooling reports whether the domain is currently within its cooling window.
func (r *CrawlDomainProfileRepository) IsCooling(domain string) (bool, error) {
	var count int64
	err := r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ? AND next_allowed_at > ?", domain, time.Now()).
		Count(&count).Error
	return count > 0, err
}

// UpdateOverrides stores the domain-level request overrides and analysis (written by RuleOptimizer).
func (r *CrawlDomainProfileRepository) UpdateOverrides(domain string, overrides datatypes.JSON, analysis string) error {
	return r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", domain).
		Updates(map[string]interface{}{
			"request_overrides": overrides,
			"analysis_result":   analysis,
		}).Error
}
