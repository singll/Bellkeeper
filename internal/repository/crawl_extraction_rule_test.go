package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
)

func TestCrawlExtractionRuleRepository_CreateAndUpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	rule := &model.CrawlExtractionRule{
		Domain:       "example.com",
		MatchPattern: "/blog/*",
		Strategy:     model.StrategyRSSHub,
		RSSHubRoute:  "/example/blog",
		Status:       model.ExtractionRuleCandidate,
		CreatedBy:    model.RuleCreatedByLLM,
	}
	assertNoError(t, repo.Create(rule), "Create")
	assertEqual(t, rule.ID > 0, true)

	assertNoError(t, repo.UpdateStatus(rule.ID, model.ExtractionRuleActive), "UpdateStatus")

	rules, err := repo.ListByDomain("example.com")
	assertNoError(t, err, "ListByDomain")
	assertEqual(t, len(rules), 1)
	assertEqual(t, rules[0].Status, model.ExtractionRuleActive)
}

func TestCrawlExtractionRuleRepository_ListByDomain(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyRSSHub, Status: model.ExtractionRuleCandidate, CreatedBy: model.RuleCreatedByLLM,
	}), "Create 1")
	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "b.com", Strategy: model.StrategyTrafilatura, Status: model.ExtractionRuleActive, CreatedBy: model.RuleCreatedByHuman,
	}), "Create 2")

	rules, err := repo.ListByDomain("a.com")
	assertNoError(t, err, "ListByDomain a.com")
	assertEqual(t, len(rules), 1)
}

func TestCrawlExtractionRuleRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyRSSHub, Status: model.ExtractionRuleCandidate, CreatedBy: model.RuleCreatedByLLM,
	}), "Create 1")
	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "b.com", Strategy: model.StrategyTrafilatura, Status: model.ExtractionRuleActive, CreatedBy: model.RuleCreatedByHuman,
	}), "Create 2")

	rules, total, err := repo.List(ListExtractionRuleOpts{Page: 1, Limit: 10})
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(rules), 2)
}

func TestCrawlExtractionRuleRepository_ListWithDomainFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyRSSHub, Status: model.ExtractionRuleCandidate, CreatedBy: model.RuleCreatedByLLM,
	}), "Create")

	_, total, err := repo.List(ListExtractionRuleOpts{Domain: "a.com", Page: 1, Limit: 10})
	assertNoError(t, err, "List domain filter")
	assertEqual(t, total, int64(1))

	_, total, err = repo.List(ListExtractionRuleOpts{Domain: "nonexistent.com", Page: 1, Limit: 10})
	assertNoError(t, err, "List domain filter nonexistent")
	assertEqual(t, total, int64(0))
}

func TestCrawlExtractionRuleRepository_CreateTrialAndListTrialsByRule(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	rule := &model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyRSSHub, Status: model.ExtractionRuleCandidate, CreatedBy: model.RuleCreatedByLLM,
	}
	assertNoError(t, repo.Create(rule), "Create rule")

	trial := &model.CrawlRuleTrial{
		RuleID:     rule.ID,
		SampleURLs: datatypes.JSON(`["https://a.com/1","https://a.com/2"]`),
		Attempt:    1,
		AfterStatus: "success",
		ContentLen: 5000,
		QualityScore: 0.85,
	}
	assertNoError(t, repo.CreateTrial(trial), "CreateTrial")

	trials, err := repo.ListTrialsByRule(rule.ID)
	assertNoError(t, err, "ListTrialsByRule")
	assertEqual(t, len(trials), 1)
	assertEqual(t, trials[0].QualityScore, 0.85)
}

func TestCrawlExtractionRuleRepository_FindActiveByDomain(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyRSSHub, Status: model.ExtractionRuleActive, CreatedBy: model.RuleCreatedByHuman,
	}), "Create active")
	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyTrafilatura, Status: model.ExtractionRuleCandidate, CreatedBy: model.RuleCreatedByLLM,
	}), "Create candidate")

	rule, err := repo.FindActiveByDomain("a.com")
	assertNoError(t, err, "FindActiveByDomain")
	assertEqual(t, rule.Status, model.ExtractionRuleActive)
	assertEqual(t, rule.Strategy, model.StrategyRSSHub)

	_, err = repo.FindActiveByDomain("b.com")
	assertError(t, err, "FindActiveByDomain nonexistent")
}

func TestCrawlExtractionRuleRepository_FindCandidateByDomain(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyRSSHub, Status: model.ExtractionRuleCandidate, CreatedBy: model.RuleCreatedByLLM,
	}), "Create candidate")
	assertNoError(t, repo.Create(&model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyTrafilatura, Status: model.ExtractionRuleActive, CreatedBy: model.RuleCreatedByHuman,
	}), "Create active")

	rule, err := repo.FindCandidateByDomain("a.com")
	assertNoError(t, err, "FindCandidateByDomain")
	assertEqual(t, rule.Status, model.ExtractionRuleCandidate)
}

func TestCrawlExtractionRuleRepository_CollectFailureSamples(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)
	crawlRepo := NewCrawlJobRepository(db)

	assertNoError(t, crawlRepo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobDead, SourceDomain: "a.com", ErrorType: "timeout", ExtractorUsed: "rsshub"}), "Create dead")
	assertNoError(t, crawlRepo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/2", Status: model.CrawlJobSuccess, SourceDomain: "a.com", ExtractorUsed: "trafilatura"}), "Create success")

	since := time.Now().Add(-1 * time.Hour)
	samples, err := repo.CollectFailureSamples("a.com", 10, since)
	assertNoError(t, err, "CollectFailureSamples")
	assertEqual(t, len(samples), 1)
	assertEqual(t, samples[0].ErrorType, "timeout")
}

func TestCrawlExtractionRuleRepository_CountCandidateTrials(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlExtractionRuleRepository(db)

	rule := &model.CrawlExtractionRule{
		Domain: "a.com", Strategy: model.StrategyRSSHub, Status: model.ExtractionRuleCandidate, CreatedBy: model.RuleCreatedByLLM,
	}
	assertNoError(t, repo.Create(rule), "Create rule")

	assertNoError(t, repo.CreateTrial(&model.CrawlRuleTrial{RuleID: rule.ID, Attempt: 1, AfterStatus: "success", QualityScore: 0.8}), "CreateTrial 1")
	assertNoError(t, repo.CreateTrial(&model.CrawlRuleTrial{RuleID: rule.ID, Attempt: 2, AfterStatus: "failed", QualityScore: 0.2}), "CreateTrial 2")

	count, err := repo.CountCandidateTrials(rule.ID)
	assertNoError(t, err, "CountCandidateTrials")
	assertEqual(t, count, int64(2))
}
