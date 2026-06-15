package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestCrawlFailureRepository_UpsertFromJobUsesPassedError(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlFailureRepository(db)

	// job carries a STALE error from a prior attempt; the call passes the real one.
	job := &model.CrawlJob{
		SourceID: 1, URL: "https://f.com/1", SourceDomain: "f.com",
		ErrorType: "STALE", ErrorMessage: "stale message",
	}
	assertNoError(t, repo.UpsertFromJob(job, "forbidden", "403 Forbidden"), "UpsertFromJob")

	got, err := repo.FindByURL("https://f.com/1")
	assertNoError(t, err, "FindByURL")
	if got == nil {
		t.Fatal("expected a failure record, got nil")
	}
	assertEqual(t, got.LastErrorType, "forbidden")
	assertEqual(t, got.LastErrorMessage, "403 Forbidden")
	assertEqual(t, got.FailureCount, 1)

	// Second failure bumps the counter and updates the error.
	assertNoError(t, repo.UpsertFromJob(job, "timeout", "deadline exceeded"), "UpsertFromJob 2")
	got, err = repo.FindByURL("https://f.com/1")
	assertNoError(t, err, "FindByURL 2")
	assertEqual(t, got.FailureCount, 2)
	assertEqual(t, got.LastErrorType, "timeout")
}

func TestCrawlFailureRepository_ResolveByURL(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlFailureRepository(db)

	job := &model.CrawlJob{SourceID: 1, URL: "https://f.com/2", SourceDomain: "f.com"}
	assertNoError(t, repo.UpsertFromJob(job, "forbidden", "403"), "UpsertFromJob")

	// After a later success, the URL drops off the failure to-do list.
	assertNoError(t, repo.ResolveByURL("https://f.com/2"), "ResolveByURL")

	got, err := repo.FindByURL("https://f.com/2")
	assertNoError(t, err, "FindByURL")
	assertEqual(t, got == nil, true)

	// Resolving a URL with no record is a no-op.
	assertNoError(t, repo.ResolveByURL("https://f.com/never"), "ResolveByURL missing")
}
