package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
)

func TestCrawlDomainProfileRepository_FindOrCreate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	p, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate new")
	assertEqual(t, p.Domain, "example.com")
	assertEqual(t, p.DefaultDelaySeconds, 30)
	assertEqual(t, p.MaxConcurrency, 2)

	p2, err := repo.FindOrCreate("example.com", 60, 5)
	assertNoError(t, err, "FindOrCreate existing")
	assertEqual(t, p2.ID, p.ID)
	assertEqual(t, p2.DefaultDelaySeconds, 30)
}

func TestCrawlDomainProfileRepository_FindOrCreateDefaults(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	p, err := repo.FindOrCreate("new.com", 0, 0)
	assertNoError(t, err, "FindOrCreate with zero defaults")
	assertEqual(t, p.DefaultDelaySeconds, 60)
	assertEqual(t, p.MaxConcurrency, 1)
}

func TestCrawlDomainProfileRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("a.com", 30, 1)
	assertNoError(t, err, "FindOrCreate a")
	_, err = repo.FindOrCreate("b.com", 60, 2)
	assertNoError(t, err, "FindOrCreate b")

	profiles, total, err := repo.List(1, 10)
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(profiles), 2)
}

func TestCrawlDomainProfileRepository_RecordStart(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate")

	nextAllowed := time.Now().Add(30 * time.Second)
	assertNoError(t, repo.RecordStart("example.com", nextAllowed), "RecordStart")

	var p model.CrawlDomainProfile
	assertNoError(t, db.Where("domain = ?", "example.com").First(&p).Error, "Find")
	assertEqual(t, p.LastStatus, "running")
	assertEqual(t, p.NextAllowedAt != nil, true)
}

func TestCrawlDomainProfileRepository_RecordOutcome(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate")

	nextAllowed := time.Now().Add(60 * time.Second)
	assertNoError(t, repo.RecordOutcome("example.com", "success", "ok", &nextAllowed), "RecordOutcome")

	var p model.CrawlDomainProfile
	assertNoError(t, db.Where("domain = ?", "example.com").First(&p).Error, "Find")
	assertEqual(t, p.LastStatus, "success")
	assertEqual(t, p.Notes, "ok")
}

func TestCrawlDomainProfileRepository_RefreshRates(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)
	crawlRepo := NewCrawlJobRepository(db)

	_, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate")

	assertNoError(t, crawlRepo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://example.com/1", Status: model.CrawlJobSuccess, SourceDomain: "example.com"}), "Create success")
	assertNoError(t, crawlRepo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://example.com/2", Status: model.CrawlJobBlocked, SourceDomain: "example.com"}), "Create blocked")

	since := time.Now().Add(-1 * time.Hour)
	assertNoError(t, repo.RefreshRates("example.com", since), "RefreshRates")

	var p model.CrawlDomainProfile
	assertNoError(t, db.Where("domain = ?", "example.com").First(&p).Error, "Find")
	assertEqual(t, p.SuccessRate24h > 0, true)
	assertEqual(t, p.BlockRate24h > 0, true)
}

func TestCrawlDomainProfileRepository_CoolingExponential(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	base := 1 * time.Minute
	max := 1 * time.Hour

	// First failure: failure_count=1, next_allowed_at ~ now+base.
	assertNoError(t, repo.EnterCooling("example.com", base, max), "EnterCooling 1")
	cooling, err := repo.IsCooling("example.com")
	assertNoError(t, err, "IsCooling")
	assertEqual(t, cooling, true)

	p, err := repo.FindOrCreate("example.com", 0, 0)
	assertNoError(t, err, "FindOrCreate")
	assertEqual(t, p.FailureCount, 1)
	if p.NextAllowedAt == nil {
		t.Fatalf("expected next_allowed_at set, got nil")
	}
	assertWithin(t, *p.NextAllowedAt, time.Now().Add(base), 15*time.Second)

	// Second failure: failure_count=2, duration doubles to base*2.
	assertNoError(t, repo.EnterCooling("example.com", base, max), "EnterCooling 2")
	p, _ = repo.FindOrCreate("example.com", 0, 0)
	assertEqual(t, p.FailureCount, 2)
	assertWithin(t, *p.NextAllowedAt, time.Now().Add(2*base), 15*time.Second)

	// Success clears cooling: failure_count back to 0, next_allowed_at NULL.
	assertNoError(t, repo.ClearCooling("example.com"), "ClearCooling")
	cooling, _ = repo.IsCooling("example.com")
	assertEqual(t, cooling, false)
	p, _ = repo.FindOrCreate("example.com", 0, 0)
	assertEqual(t, p.FailureCount, 0)
	assertEqual(t, p.NextAllowedAt == nil, true)
}

func TestCrawlDomainProfileRepository_CoolingCap(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	base := 1 * time.Minute
	max := 5 * time.Minute

	// After many failures the exponential duration must be capped at max.
	for i := 0; i < 10; i++ {
		assertNoError(t, repo.EnterCooling("capped.com", base, max), "EnterCooling")
	}
	p, _ := repo.FindOrCreate("capped.com", 0, 0)
	assertEqual(t, p.FailureCount, 10)
	assertWithin(t, *p.NextAllowedAt, time.Now().Add(max), 15*time.Second)
}

func TestCrawlDomainProfileRepository_UpdateOverrides(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("ov.com", 0, 0)
	assertNoError(t, err, "FindOrCreate")

	assertNoError(t, repo.UpdateOverrides("ov.com", datatypes.JSON(`{"user_agent":"Mozilla/5.0"}`), "needs UA"), "UpdateOverrides")

	p, _ := repo.FindOrCreate("ov.com", 0, 0)
	assertJSONEq(t, string(p.RequestOverrides), `{"user_agent":"Mozilla/5.0"}`)
	assertEqual(t, p.AnalysisResult, "needs UA")
}

func assertWithin(t *testing.T, got, want time.Time, tolerance time.Duration) {
	t.Helper()
	diff := got.Sub(want)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("time %v not within %s of %v (diff %s)", got, tolerance, want, diff)
	}
}
