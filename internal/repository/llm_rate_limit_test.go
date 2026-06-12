package repository

import (
	"testing"
	"time"
)

func TestLLMRateLimitRepository_GetOrCreate_New(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMRateLimitRepository(db)

	rl, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate new")
	assertEqual(t, rl.ChannelID, uint(1))
	assertEqual(t, rl.Model, "gpt-4o")
	assertEqual(t, rl.ConfiguredRPM, 500)
	assertEqual(t, rl.LearnedRPMSafe, 250)
}

func TestLLMRateLimitRepository_GetOrCreate_Existing(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMRateLimitRepository(db)

	rl1, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate first")

	rl2, err := repo.GetOrCreate(1, "gpt-4o", 600)
	assertNoError(t, err, "GetOrCreate second")
	assertEqual(t, rl2.ID, rl1.ID)
	assertEqual(t, rl2.ConfiguredRPM, 500)
}

func TestLLMRateLimitRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMRateLimitRepository(db)

	rl, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate")

	rl.LearnedRPMSafe = 300
	rl.LearnedRPDSafe = 10000
	assertNoError(t, repo.Update(rl), "Update")

	got, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate after update")
	assertEqual(t, got.LearnedRPMSafe, 300)
	assertEqual(t, got.LearnedRPDSafe, 10000)
}

func TestLLMRateLimitRepository_ListByChannel(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMRateLimitRepository(db)

	_, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate 1")
	_, err = repo.GetOrCreate(1, "gpt-4o-mini", 1000)
	assertNoError(t, err, "GetOrCreate 2")
	_, err = repo.GetOrCreate(2, "claude-3", 800)
	assertNoError(t, err, "GetOrCreate 3")

	rls, err := repo.ListByChannel(1)
	assertNoError(t, err, "ListByChannel")
	assertEqual(t, len(rls), 2)
}

func TestLLMRateLimitRepository_ListAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMRateLimitRepository(db)

	_, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate 1")
	_, err = repo.GetOrCreate(2, "claude-3", 800)
	assertNoError(t, err, "GetOrCreate 2")

	rls, err := repo.ListAll()
	assertNoError(t, err, "ListAll")
	assertEqual(t, len(rls), 2)
}

func TestLLMRateLimitRepository_SetLocked(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMRateLimitRepository(db)

	rl, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate")
	assertEqual(t, rl.Locked, false)

	assertNoError(t, repo.SetLocked(rl.ID, true), "SetLocked true")

	rls, err := repo.ListByChannel(1)
	assertNoError(t, err, "ListByChannel")
	assertEqual(t, rls[0].Locked, true)
}

func TestLLMRateLimitRepository_UpdateWithTimestamps(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMRateLimitRepository(db)

	rl, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate")

	now := time.Now()
	rl.Last429At = &now
	rl.LastAdjustAt = &now
	rl.ConfidenceScore = 0.85
	assertNoError(t, repo.Update(rl), "Update")

	got, err := repo.GetOrCreate(1, "gpt-4o", 500)
	assertNoError(t, err, "GetOrCreate after update")
	assertEqual(t, got.ConfidenceScore, 0.85)
}
