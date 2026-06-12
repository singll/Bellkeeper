package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
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
