package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type CrawlExtractionRuleRepository struct {
	db *gorm.DB
}

func NewCrawlExtractionRuleRepository(db *gorm.DB) *CrawlExtractionRuleRepository {
	return &CrawlExtractionRuleRepository{db: db}
}

func (r *CrawlExtractionRuleRepository) FindActiveByDomain(domain string) (*model.CrawlExtractionRule, error) {
	var rule model.CrawlExtractionRule
	err := r.db.Where("domain = ? AND status = ?", domain, model.ExtractionRuleActive).
		Order("version DESC").
		First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *CrawlExtractionRuleRepository) FindCandidateByDomain(domain string) (*model.CrawlExtractionRule, error) {
	var rule model.CrawlExtractionRule
	err := r.db.Where("domain = ? AND status = ?", domain, model.ExtractionRuleCandidate).
		Order("version DESC").
		First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *CrawlExtractionRuleRepository) Create(rule *model.CrawlExtractionRule) error {
	return r.db.Create(rule).Error
}

func (r *CrawlExtractionRuleRepository) UpdateStatus(id uint, status model.ExtractionRuleStatus) error {
	return r.db.Model(&model.CrawlExtractionRule{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": string(status)}).Error
}

func (r *CrawlExtractionRuleRepository) ListByDomain(domain string) ([]model.CrawlExtractionRule, error) {
	var rules []model.CrawlExtractionRule
	err := r.db.Where("domain = ?", domain).Order("version DESC").Find(&rules).Error
	return rules, err
}

func (r *CrawlExtractionRuleRepository) List(opts ListExtractionRuleOpts) ([]model.CrawlExtractionRule, int64, error) {
	var rules []model.CrawlExtractionRule
	var total int64
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Limit > 200 {
		opts.Limit = 200
	}

	tx := r.db.Model(&model.CrawlExtractionRule{})
	if opts.Domain != "" {
		tx = tx.Where("domain = ?", opts.Domain)
	}
	if opts.Status != "" {
		tx = tx.Where("status = ?", opts.Status)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Order("updated_at DESC, domain ASC").
		Offset((opts.Page - 1) * opts.Limit).
		Limit(opts.Limit).
		Find(&rules).Error; err != nil {
		return nil, 0, err
	}
	return rules, total, nil
}

type ListExtractionRuleOpts struct {
	Domain string
	Status string
	Page   int
	Limit  int
}

func (r *CrawlExtractionRuleRepository) CreateTrial(trial *model.CrawlRuleTrial) error {
	return r.db.Create(trial).Error
}

func (r *CrawlExtractionRuleRepository) ListTrialsByRule(ruleID uint) ([]model.CrawlRuleTrial, error) {
	var trials []model.CrawlRuleTrial
	err := r.db.Where("rule_id = ?", ruleID).Order("created_at DESC").Find(&trials).Error
	return trials, err
}

type DomainFailureSample struct {
	URL          string `json:"url"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
	Extractor    string `json:"extractor"`
}

func (r *CrawlExtractionRuleRepository) CollectFailureSamples(domain string, limit int, since time.Time) ([]DomainFailureSample, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	var samples []DomainFailureSample
	err := r.db.Model(&model.CrawlJob{}).
		Select("url, error_type, error_message, extractor_used as extractor").
		Where("source_domain = ? AND status IN ? AND updated_at >= ?",
			domain,
			[]string{string(model.CrawlJobDead), string(model.CrawlJobBlocked), string(model.CrawlJobRetrying)},
			since,
		).
		Order("updated_at DESC").
		Limit(limit).
		Find(&samples).Error
	return samples, err
}

func (r *CrawlExtractionRuleRepository) CountCandidateTrials(ruleID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.CrawlRuleTrial{}).Where("rule_id = ?", ruleID).Count(&count).Error
	return count, err
}
