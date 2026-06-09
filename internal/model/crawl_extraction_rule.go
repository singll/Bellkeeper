package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ExtractionRuleStatus string

const (
	ExtractionRuleCandidate ExtractionRuleStatus = "candidate"
	ExtractionRuleActive    ExtractionRuleStatus = "active"
	ExtractionRuleRejected  ExtractionRuleStatus = "rejected"
	ExtractionRuleRollback  ExtractionRuleStatus = "rollback"
)

type ExtractionStrategy string

const (
	StrategyRSSHub      ExtractionStrategy = "rsshub"
	StrategyTrafilatura ExtractionStrategy = "trafilatura"
	StrategyFirecrawl   ExtractionStrategy = "firecrawl"
	StrategyReadability ExtractionStrategy = "readability"
	StrategyPlaywright  ExtractionStrategy = "playwright"
)

type ExtractionRuleCreatedBy string

const (
	RuleCreatedByLLM   ExtractionRuleCreatedBy = "llm"
	RuleCreatedByHuman ExtractionRuleCreatedBy = "human"
	RuleCreatedBySeed  ExtractionRuleCreatedBy = "seed"
)

type CrawlExtractionRule struct {
	ID                 uint                  `gorm:"primaryKey" json:"id"`
	Domain             string                `gorm:"size:255;not null;index:idx_crawl_ext_rules_domain,where:deleted_at IS NULL" json:"domain"`
	MatchPattern       string                `gorm:"size:500" json:"match_pattern,omitempty"`
	Strategy           ExtractionStrategy    `gorm:"size:30;not null" json:"strategy"`
	RSSHubRoute        string                `gorm:"size:500" json:"rsshub_route,omitempty"`
	CSSTitleSelector   string                `gorm:"size:500" json:"css_title_selector,omitempty"`
	CSSContentSelector string                `gorm:"size:500" json:"css_content_selector,omitempty"`
	CSSRemoveSelectors string                `gorm:"type:text" json:"css_remove_selectors,omitempty"`
	FirecrawlOptions   datatypes.JSON        `gorm:"type:jsonb" json:"firecrawl_options,omitempty"`
	TrafilaturaOptions datatypes.JSON        `gorm:"type:jsonb" json:"trafilatura_options,omitempty"`
	QualityMinChars    int                   `gorm:"default:200" json:"quality_min_chars"`
	Version            int                   `gorm:"default:1" json:"version"`
	Status             ExtractionRuleStatus  `gorm:"size:20;not null;default:candidate;index" json:"status"`
	CreatedBy          ExtractionRuleCreatedBy `gorm:"size:20;not null;default:llm" json:"created_by"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
	DeletedAt          gorm.DeletedAt        `gorm:"index" json:"-"`
}

func (CrawlExtractionRule) TableName() string {
	return "crawl_extraction_rules"
}

type CrawlRuleTrial struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RuleID       uint      `gorm:"not null;index" json:"rule_id"`
	SampleURLs   datatypes.JSON `gorm:"type:jsonb" json:"sample_urls"`
	Attempt      int       `gorm:"default:1" json:"attempt"`
	BeforeError  string    `gorm:"size:500" json:"before_error,omitempty"`
	AfterStatus  string    `gorm:"size:30" json:"after_status,omitempty"`
	ContentLen   int       `gorm:"default:0" json:"content_len"`
	QualityScore float64   `gorm:"default:0" json:"quality_score"`
	DiffSummary  string    `gorm:"type:text" json:"diff_summary,omitempty"`
	CreatedAt    time.Time `json:"created_at"`

	Rule CrawlExtractionRule `gorm:"foreignKey:RuleID" json:"-"`
}

func (CrawlRuleTrial) TableName() string {
	return "crawl_rule_trials"
}
