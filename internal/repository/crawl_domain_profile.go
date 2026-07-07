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
//
// failure_count 与退避冷却对「所有」失败生效（单页失败也应给该域名一点喘息，
// 避免连打）；但 ConsecutiveFailures 递增与 HealthScore 递减「只对域名级故障
// 生效」——由 countsTowardHealth 控制。单 URL 的 not_found/empty_content 或
// 抓取器自身 400/连接失败传 false，否则会把 arxiv/archive 等健康大站误判为
// 不健康并整域暂停（1.0 §2.1.1 修复）。
func (r *CrawlDomainProfileRepository) EnterCooling(domain string, base, max time.Duration, countsTowardHealth bool) error {
	if _, err := r.FindOrCreate(domain, 0, 0); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"failure_count": gorm.Expr("failure_count + 1"),
	}
	if countsTowardHealth {
		updates["consecutive_failures"] = gorm.Expr("consecutive_failures + 1")
		updates["health_score"] = gorm.Expr("GREATEST(health_score - 10, 0)")
	}
	if err := r.db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", domain).
		Updates(updates).Error; err != nil {
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
// DomainHealthResult 携带一次健康度评估的结果与快照，供通知层按可用性语义分级。
type DomainHealthResult struct {
	Action              string // "paused" | "resumed" | "none"
	HealthScore         int    // 评估后健康度（0 表示已探底 → 事实不可用）
	ConsecutiveFailures int    // 评估时的连续失败数
	PausedReason        string // 暂停原因（域名级 errType 累计）
}

// EvaluateDomainHealth 按阈值评估域名健康度并在越过暂停阈值时自动暂停（1.0 §2.1.1）。
// ConsecutiveFailures≥pauseThreshold 且未暂停 → 暂停；其余返回 none。
// 边缘触发：仅在从「未暂停」翻转到「暂停」时返回 paused，稳态返回 none。
//
// 恢复不在此处：暂停域名不再出队 → 无 crawl.failed 事件 → 此函数对暂停域名不会
// 被再次调用；恢复统一由 HalfOpenRecoverDomains 时间驱动（见该方法）。resumeThreshold
// 保留仅为签名兼容与语义文档，此函数不再据此恢复。
func (r *CrawlDomainProfileRepository) EvaluateDomainHealth(domain string, pauseThreshold, resumeThreshold int) (DomainHealthResult, error) {
	if pauseThreshold <= 0 {
		pauseThreshold = 5
	}
	var profile model.CrawlDomainProfile
	if err := r.db.Where("domain = ?", domain).First(&profile).Error; err != nil {
		return DomainHealthResult{Action: "none"}, err
	}
	res := DomainHealthResult{
		Action:              "none",
		HealthScore:         profile.HealthScore,
		ConsecutiveFailures: profile.ConsecutiveFailures,
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
			return res, err
		}
		res.Action = "paused"
		res.PausedReason = reason
		return res, nil
	}
	return res, nil
}

// HalfOpenRecoverDomains 对暂停时长已超过 pausedBefore 冷静期的域名做 half-open
// 探测恢复：解除暂停、清零 ConsecutiveFailures、把 HealthScore 抬到中性值给一个
// 公平的重试窗口，返回被恢复的域名列表（供 worker 发「已恢复/重试」通知）。
//
// 为何改时间驱动（而非原 health_score≥阈值）：暂停域名被 DequeueFair 过滤后不再
// 出队 → HealthScore 冻结永不回升 → 原恢复条件对已暂停域名是死代码（线上实测 122
// 个域名卡死暂停）。half-open 是断路器标准模式：冷静一段时间后放一次流量探测，
// 失败则重新累计再暂停，成功则 ClearCooling 自然回血。
//
// 并发安全：先取快照再用同一 WHERE 原子批量 UPDATE；多 worker 最多重复发一次
// 恢复通知，由通知层 dedup 兜住。
func (r *CrawlDomainProfileRepository) HalfOpenRecoverDomains(pausedBefore time.Time) ([]string, error) {
	var domains []string
	if err := r.db.Model(&model.CrawlDomainProfile{}).
		Where("is_paused = ? AND paused_at IS NOT NULL AND paused_at < ?", true, pausedBefore).
		Pluck("domain", &domains).Error; err != nil {
		return nil, err
	}
	if len(domains) == 0 {
		return nil, nil
	}
	if err := r.db.Model(&model.CrawlDomainProfile{}).
		Where("is_paused = ? AND paused_at IS NOT NULL AND paused_at < ?", true, pausedBefore).
		Updates(map[string]interface{}{
			"is_paused":            false,
			"paused_reason":        "",
			"paused_at":            nil,
			"consecutive_failures": 0,
			// 抬到中性值：给一个完整的 5 次域名级失败窗口才会再次暂停，避免
			// 探测出队第一次失败就秒回探底。
			"health_score": gorm.Expr("GREATEST(health_score, 50)"),
		}).Error; err != nil {
		return nil, err
	}
	return domains, nil
}

// FindLongUnavailableDomains 返回「长期不可用」域名：已暂停且 paused_at 早于
// olderThan、健康度探底（health_score<=0）——即持续域名级失败、迟迟无法恢复，
// 疑似已失效。供 worker 每日升级告警一次（区别于短暂/可恢复的暂停）。
func (r *CrawlDomainProfileRepository) FindLongUnavailableDomains(olderThan time.Time) ([]model.CrawlDomainProfile, error) {
	var profiles []model.CrawlDomainProfile
	err := r.db.
		Where("is_paused = ? AND health_score <= 0 AND paused_at IS NOT NULL AND paused_at < ?", true, olderThan).
		Find(&profiles).Error
	return profiles, err
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
