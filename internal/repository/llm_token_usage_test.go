package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMTokenUsageRepository_GetOrCreate_New(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenUsageRepository(db)

	date := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	usage, err := repo.GetOrCreate(1, date)
	assertNoError(t, err, "GetOrCreate new")
	assertEqual(t, usage.TokenID, uint(1))
	assertEqual(t, usage.Requests, 0)
}

func TestLLMTokenUsageRepository_GetOrCreate_Existing(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenUsageRepository(db)

	date := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	u1, err := repo.GetOrCreate(1, date)
	assertNoError(t, err, "GetOrCreate first")

	u2, err := repo.GetOrCreate(1, date)
	assertNoError(t, err, "GetOrCreate second")
	assertEqual(t, u2.ID, u1.ID)
}

func TestLLMTokenUsageRepository_ListByToken(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenUsageRepository(db)

	d1 := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)

	assertNoError(t, db.Create(&model.LLMTokenUsageDaily{TokenID: 1, Date: d1, Requests: 10}).Error, "Create d1")
	assertNoError(t, db.Create(&model.LLMTokenUsageDaily{TokenID: 1, Date: d2, Requests: 20}).Error, "Create d2")
	assertNoError(t, db.Create(&model.LLMTokenUsageDaily{TokenID: 1, Date: d3, Requests: 30}).Error, "Create d3")
	assertNoError(t, db.Create(&model.LLMTokenUsageDaily{TokenID: 2, Date: d1, Requests: 5}).Error, "Create other")

	usages, err := repo.ListByToken(1, d1, d2)
	assertNoError(t, err, "ListByToken")
	assertEqual(t, len(usages), 2)
}

func TestLLMTokenUsageRepository_ListByDateRange(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenUsageRepository(db)

	d1 := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)

	assertNoError(t, db.Create(&model.LLMTokenUsageDaily{TokenID: 1, Date: d1, Requests: 10}).Error, "Create 1")
	assertNoError(t, db.Create(&model.LLMTokenUsageDaily{TokenID: 2, Date: d2, Requests: 20}).Error, "Create 2")
	assertNoError(t, db.Create(&model.LLMTokenUsageDaily{TokenID: 1, Date: d3, Requests: 30}).Error, "Create 3")

	usages, err := repo.ListByDateRange(d1, d2)
	assertNoError(t, err, "ListByDateRange")
	assertEqual(t, len(usages), 2)
}

func TestLLMTokenUsageRepository_AddUsage(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenUsageRepository(db)

	date := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)

	assertNoError(t, repo.AddUsage(1, date, 5, 100, 50, 10, 1, 500, 0), "AddUsage first")

	usages, err := repo.ListByToken(1, date, date)
	assertNoError(t, err, "ListByToken after first")
	assertEqual(t, len(usages), 1)
	assertEqual(t, usages[0].Requests, 5)

	assertNoError(t, repo.AddUsage(1, date, 3, 200, 100, 0, 2, 300, 1), "AddUsage second")

	usages2, err := repo.ListByToken(1, date, date)
	assertNoError(t, err, "ListByToken after second")
	assertEqual(t, len(usages2), 1)
	assertEqual(t, usages2[0].Requests, 8)
	assertEqual(t, usages2[0].PromptTokens, 300)
	assertEqual(t, usages2[0].CompletionTokens, 150)
	assertEqual(t, usages2[0].CostCents, 3)
	assertEqual(t, usages2[0].CostMicroCents, int64(800))
	assertEqual(t, usages2[0].ErrorCount, 1)
}

func TestLLMTokenUsageRepository_Aggregate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenUsageRepository(db)

	d1 := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	assertNoError(t, repo.AddUsage(1, d1, 10, 100, 50, 5, 1, 1500, 1), "AddUsage d1")
	assertNoError(t, repo.AddUsage(1, d2, 20, 200, 100, 10, 2, 2500, 2), "AddUsage d2")

	results, err := repo.Aggregate("token", d1, d2)
	assertNoError(t, err, "Aggregate")
	assertEqual(t, len(results), 1)

	row := results[0]
	assertEqual(t, row["token_id"] != nil, true)
	assertEqual(t, row["requests"] != nil, true)
}
