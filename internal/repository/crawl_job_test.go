package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
)

func TestCrawlJobRepository_EnqueueAndGetByURL(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{
		SourceID:     1,
		URL:          "https://example.com/article",
		Priority:     5,
		Status:       model.CrawlJobPending,
		SourceDomain: "example.com",
	}
	assertNoError(t, repo.Enqueue(job), "Enqueue")
	assertEqual(t, job.ID > 0, true)

	got, err := repo.GetByURL("https://example.com/article")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.URL, "https://example.com/article")

	got, err = repo.GetByURL("https://nonexistent.com")
	assertNoError(t, err, "GetByURL nonexistent")
	assertEqual(t, got, nil)
}

func TestCrawlJobRepository_EnqueueDedup(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job1 := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobPending}
	assertNoError(t, repo.Enqueue(job1), "Enqueue 1")

	job2 := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobPending}
	err := repo.Enqueue(job2)
	assertError(t, err, "Enqueue duplicate")
}

func TestCrawlJobRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobRunning}
	assertNoError(t, repo.Enqueue(job), "Enqueue")

	assertNoError(t, repo.UpdateStatus(job.ID, model.CrawlJobSuccess, map[string]interface{}{
		"quality_score": 0.95,
	}), "UpdateStatus")

	got, err := repo.GetByURL("https://example.com/a")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.Status, model.CrawlJobSuccess)
}

func TestCrawlJobRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobPending, SourceDomain: "a.com"}), "Create 1")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://b.com/2", Status: model.CrawlJobSuccess, SourceDomain: "b.com"}), "Create 2")

	jobs, total, err := repo.List(ListCrawlJobOpts{Page: 1, Limit: 10})
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(jobs), 2)
}

func TestCrawlJobRepository_Stats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobPending}), "Create pending")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/2", Status: model.CrawlJobSuccess}), "Create success")

	stats, err := repo.Stats()
	assertNoError(t, err, "Stats")
	assertEqual(t, stats.Pending, int64(1))
	assertEqual(t, stats.Success, int64(1))
	assertEqual(t, stats.Total, int64(2))
}

func TestCrawlJobRepository_MarkBlocked(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobRunning}
	assertNoError(t, repo.Enqueue(job), "Enqueue")

	assertNoError(t, repo.MarkBlocked(job.ID, "paywall detected"), "MarkBlocked")

	got, err := repo.GetByURL("https://example.com/a")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.Status, model.CrawlJobBlocked)
	assertEqual(t, got.BlockReason, "paywall detected")
}

func TestCrawlJobRepository_MarkDead(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobRunning}
	assertNoError(t, repo.Enqueue(job), "Enqueue")

	assertNoError(t, repo.MarkDead(job.ID, "fatal", "permanent error"), "MarkDead")

	got, err := repo.GetByURL("https://example.com/a")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.Status, model.CrawlJobDead)
	assertEqual(t, got.ErrorType, "fatal")
}

func TestCrawlJobRepository_RecoverOrphanedJobs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobRunning}
	assertNoError(t, repo.Enqueue(job), "Enqueue")

	assertNoError(t, repo.RecoverOrphanedJobs(), "RecoverOrphanedJobs")

	got, err := repo.GetByURL("https://example.com/a")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.Status, model.CrawlJobPending)
}

func TestCrawlJobRepository_CountRunningByDomain(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobRunning, SourceDomain: "a.com"}), "Create running")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/2", Status: model.CrawlJobPending, SourceDomain: "a.com"}), "Create pending")

	count, err := repo.CountRunningByDomain("a.com")
	assertNoError(t, err, "CountRunningByDomain")
	assertEqual(t, count, int64(1))
}

func TestCrawlJobRepository_CountByDomainAndStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	since := time.Now().Add(-24 * time.Hour)
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobSuccess, SourceDomain: "a.com"}), "Create")

	count, err := repo.CountByDomainAndStatus("a.com", model.CrawlJobSuccess, since)
	assertNoError(t, err, "CountByDomainAndStatus")
	assertEqual(t, count, int64(1))
}

func TestCrawlJobRepository_GetBlockedSince(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobBlocked, SourceDomain: "a.com"}), "Create")

	since := time.Now().Add(-1 * time.Hour)
	jobs, err := repo.GetBlockedSince(since)
	assertNoError(t, err, "GetBlockedSince")
	assertEqual(t, len(jobs), 1)
}

func TestCrawlJobRepository_GetDeadSince(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobDead, SourceDomain: "a.com"}), "Create")

	since := time.Now().Add(-1 * time.Hour)
	jobs, err := repo.GetDeadSince(since)
	assertNoError(t, err, "GetDeadSince")
	assertEqual(t, len(jobs), 1)
}

func TestCrawlJobRepository_RecoverStaleRunningJobs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	past := time.Now().Add(-2 * time.Hour)
	job := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobRunning, StartedAt: &past}
	assertNoError(t, repo.Enqueue(job), "Enqueue")
	assertNoError(t, db.Model(&model.CrawlJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status": model.CrawlJobRunning, "started_at": past,
	}).Error, "Force update started_at")

	affected, err := repo.RecoverStaleRunningJobs(1 * time.Hour)
	assertNoError(t, err, "RecoverStaleRunningJobs")
	assertEqual(t, affected, int64(1))
}

func TestCrawlJobRepository_CountRunningDomainRank(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	now := time.Now()
	job := &model.CrawlJob{
		SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobRunning,
		SourceDomain: "a.com", StartedAt: &now,
	}
	assertNoError(t, repo.Enqueue(job), "Enqueue")
	db.Model(&model.CrawlJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status": model.CrawlJobRunning, "started_at": now,
	})

	rank, err := repo.CountRunningDomainRank("a.com", job.ID, &now)
	assertNoError(t, err, "CountRunningDomainRank")
	assertEqual(t, rank >= int64(1), true)
}

func TestCrawlJobRepository_Dequeue(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobPending, SourceDomain: "a.com"}), "Enqueue")

	job, err := repo.Dequeue("")
	assertNoError(t, err, "Dequeue")
	assertEqual(t, job != nil, true)
	assertEqual(t, job.Status, model.CrawlJobRunning)
	assertEqual(t, job.StartedAt != nil, true)

	job2, err := repo.Dequeue("")
	assertNoError(t, err, "Dequeue empty")
	assertEqual(t, job2, nil)
}

func TestCrawlJobRepository_MarkRetry(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobRunning, SourceDomain: "example.com"}
	assertNoError(t, repo.Enqueue(job), "Enqueue")

	nextRetry := time.Now().Add(30 * time.Second)
	assertNoError(t, repo.MarkRetry(job.ID, nextRetry, "timeout", "temporary error"), "MarkRetry")

	got, err := repo.GetByURL("https://example.com/a")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.Status, model.CrawlJobRetrying)
	assertEqual(t, got.RetryCount, 1)
	assertEqual(t, got.ErrorType, "timeout")
}

func TestCrawlJobRepository_DelayJob(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{SourceID: 1, URL: "https://example.com/a", Status: model.CrawlJobRunning, SourceDomain: "example.com"}
	assertNoError(t, repo.Enqueue(job), "Enqueue")

	nextRetry := time.Now().Add(10 * time.Second)
	assertNoError(t, repo.DelayJob(job.ID, nextRetry, "throttle", "rate limited"), "DelayJob")

	got, err := repo.GetByURL("https://example.com/a")
	assertNoError(t, err, "GetByURL")
	assertEqual(t, got.Status, model.CrawlJobRetrying)
	assertEqual(t, got.NextRetryAt != nil, true)
	assertEqual(t, got.ErrorType, "throttle")
}

func TestCrawlJobRepository_Audit(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobSuccess, SourceDomain: "a.com", ExtractorUsed: "trafilatura"}), "Create success")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/2", Status: model.CrawlJobFailed, SourceDomain: "a.com", ErrorType: "timeout", ExtractorUsed: "rsshub"}), "Create failed")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/3", Status: model.CrawlJobBlocked, SourceDomain: "a.com", BlockReason: "paywall", ExtractorUsed: "trafilatura"}), "Create blocked")

	since := time.Now().Add(-1 * time.Hour)
	stats, err := repo.Audit(since, 10)
	assertNoError(t, err, "Audit")
	assertEqual(t, stats.Total, int64(3))
	assertEqual(t, len(stats.ByStatus) > 0, true)
	assertEqual(t, len(stats.TopDomains) > 0, true)
	assertEqual(t, len(stats.TopErrorTypes) > 0, true)
	assertEqual(t, len(stats.Extractors) > 0, true)
}

func TestCrawlJobRepository_EnqueueWithMetadata(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	job := &model.CrawlJob{
		SourceID: 1, URL: "https://example.com/a",
		Status: model.CrawlJobPending, SourceDomain: "example.com",
		Metadata: datatypes.JSON(`{"depth":1}`),
	}
	assertNoError(t, repo.Enqueue(job), "Enqueue")
	assertEqual(t, job.ID > 0, true)
}

func TestCrawlJobRepository_CountPendingByDomain(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	// 2 pending + 1 retrying + 1 success for q.com; success must not count.
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://q.com/1", Status: model.CrawlJobPending, SourceDomain: "q.com"}), "p1")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://q.com/2", Status: model.CrawlJobPending, SourceDomain: "q.com"}), "p2")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://q.com/3", Status: model.CrawlJobRetrying, SourceDomain: "q.com"}), "r1")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://q.com/4", Status: model.CrawlJobSuccess, SourceDomain: "q.com"}), "s1")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://other.com/1", Status: model.CrawlJobPending, SourceDomain: "other.com"}), "o1")

	count, err := repo.CountPendingByDomain("q.com")
	assertNoError(t, err, "CountPendingByDomain")
	assertEqual(t, count, int64(3))

	none, err := repo.CountPendingByDomain("missing.com")
	assertNoError(t, err, "CountPendingByDomain missing")
	assertEqual(t, none, int64(0))
}

func TestCrawlJobRepository_DequeueFair_ClaimsAndRuns(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)

	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://a.com/1", Status: model.CrawlJobPending, SourceDomain: "a.com"}), "Enqueue")

	job, err := repo.DequeueFair("")
	assertNoError(t, err, "DequeueFair")
	if job == nil {
		t.Fatalf("expected a job, got nil")
	}
	assertEqual(t, job.Status, model.CrawlJobRunning)
	assertEqual(t, job.URL, "https://a.com/1")

	// Once claimed, the same job is not handed out again.
	job2, err := repo.DequeueFair("")
	assertNoError(t, err, "DequeueFair empty")
	assertEqual(t, job2 == nil, true)
}

func TestCrawlJobRepository_DequeueFair_SkipsCoolingDomain(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)
	profiles := NewCrawlDomainProfileRepository(db)

	// cool.com is cooling (next_allowed_at in the future); ready.com is not.
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://cool.com/1", Status: model.CrawlJobPending, SourceDomain: "cool.com"}), "Enqueue cool")
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://ready.com/1", Status: model.CrawlJobPending, SourceDomain: "ready.com"}), "Enqueue ready")
	assertNoError(t, profiles.EnterCooling("cool.com", 1*time.Minute, 1*time.Hour, true), "EnterCooling")

	// First claim must be ready.com — cool.com is filtered out at the SQL level.
	job, err := repo.DequeueFair("")
	assertNoError(t, err, "DequeueFair")
	if job == nil {
		t.Fatalf("expected ready.com job, got nil")
	}
	assertEqual(t, job.SourceDomain, "ready.com")

	// No more claimable jobs: cool.com stays pending until its cooling expires.
	job2, err := repo.DequeueFair("")
	assertNoError(t, err, "DequeueFair after ready claimed")
	assertEqual(t, job2 == nil, true)

	cool, err := repo.GetByURL("https://cool.com/1")
	assertNoError(t, err, "GetByURL cool")
	assertEqual(t, cool.Status, model.CrawlJobPending)
}

func TestCrawlJobRepository_DequeueFair_FairRotation(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlJobRepository(db)
	profiles := NewCrawlDomainProfileRepository(db)

	// big.com floods the queue (3 older jobs); small.com has a single newer job.
	for i := 1; i <= 3; i++ {
		assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: fmt.Sprintf("https://big.com/%d", i), Status: model.CrawlJobPending, SourceDomain: "big.com"}), "Enqueue big")
	}
	assertNoError(t, repo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://small.com/1", Status: model.CrawlJobPending, SourceDomain: "small.com"}), "Enqueue small")

	// First claim goes to big.com (oldest head).
	first, err := repo.DequeueFair("")
	assertNoError(t, err, "DequeueFair 1")
	if first == nil {
		t.Fatalf("expected a job, got nil")
	}
	assertEqual(t, first.SourceDomain, "big.com")

	// Simulate the politeness/cooling delay applied after processing big.com.
	assertNoError(t, profiles.EnterCooling("big.com", 1*time.Minute, 1*time.Hour, true), "EnterCooling big")

	// Next claim must reach small.com — it is not starved behind big.com's backlog.
	second, err := repo.DequeueFair("")
	assertNoError(t, err, "DequeueFair 2")
	if second == nil {
		t.Fatalf("expected small.com job, got nil")
	}
	assertEqual(t, second.SourceDomain, "small.com")
}
