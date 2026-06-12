package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMTokenRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	tok := &model.LLMToken{
		Name:                 "test-token",
		KeyHash:              model.HashKey("sk-test123"),
		KeyPrefix:            "sk-test",
		CallerID:             "caller-1",
		AllowedModels:        `["gpt-4o"]`,
		AllowedGroups:        `[]`,
		QuotaRequestsDaily:   1000,
		QuotaTokensDaily:     500000,
		QuotaCostMonthlyCents: 10000,
		Enabled:              true,
	}
	assertNoError(t, repo.Create(tok), "Create")
	assertEqual(t, tok.ID > 0, true)

	got, err := repo.Get(tok.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.Name, "test-token")
	assertEqual(t, got.CallerID, "caller-1")
}

func TestLLMTokenRepository_GetByKeyHash(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	hash := model.HashKey("sk-mykey")
	tok := &model.LLMToken{
		Name:      "tok1",
		KeyHash:   hash,
		KeyPrefix: "sk-myk",
		CallerID:  "caller-1",
		Enabled:   true,
	}
	assertNoError(t, repo.Create(tok), "Create")

	got, err := repo.GetByKeyHash(hash)
	assertNoError(t, err, "GetByKeyHash")
	assertEqual(t, got.Name, "tok1")

	_, err = repo.GetByKeyHash("nonexistent")
	assertError(t, err, "GetByKeyHash nonexistent")
}

func TestLLMTokenRepository_GetByCallerID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	tok := &model.LLMToken{
		Name:      "tok1",
		KeyHash:   model.HashKey("sk-1"),
		KeyPrefix: "sk-",
		CallerID:  "myapp",
		Enabled:   true,
	}
	assertNoError(t, repo.Create(tok), "Create")

	got, err := repo.GetByCallerID("myapp")
	assertNoError(t, err, "GetByCallerID")
	assertEqual(t, got.Name, "tok1")

	_, err = repo.GetByCallerID("nonexistent")
	assertError(t, err, "GetByCallerID nonexistent")
}

func TestLLMTokenRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	assertNoError(t, repo.Create(&model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "c1", Enabled: true}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMToken{Name: "t2", KeyHash: "h2", CallerID: "c2", Enabled: true}), "Create 2")

	tokens, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(tokens), 2)
}

func TestLLMTokenRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	tok := &model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "c1", Enabled: true, QuotaRequestsDaily: 100}
	assertNoError(t, repo.Create(tok), "Create")

	tok.QuotaRequestsDaily = 200
	assertNoError(t, repo.Update(tok), "Update")

	got, err := repo.Get(tok.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.QuotaRequestsDaily, 200)
}

func TestLLMTokenRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	tok := &model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "c1", Enabled: true}
	assertNoError(t, repo.Create(tok), "Create")
	assertNoError(t, repo.Delete(tok.ID), "Delete")

	_, err := repo.Get(tok.ID)
	assertError(t, err, "Get after delete")
}

func TestLLMTokenRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	assertNoError(t, repo.Create(&model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "c1", Enabled: true}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMToken{Name: "t2", KeyHash: "h2", CallerID: "c2", Enabled: true}), "Create 2")

	count, err := repo.Count()
	assertNoError(t, err, "Count")
	assertEqual(t, count, int64(2))
}

func TestLLMTokenRepository_UpdateLastUsed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	tok := &model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "c1", Enabled: true}
	assertNoError(t, repo.Create(tok), "Create")

	assertNoError(t, repo.UpdateLastUsed(tok.ID), "UpdateLastUsed")

	got, err := repo.Get(tok.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.LastUsedAt != nil, true)
}

func TestLLMTokenRepository_CreateWithExpiry(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	expiresAt := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	tok := &model.LLMToken{
		Name:      "expiring",
		KeyHash:   "h-exp",
		CallerID:  "c-exp",
		Enabled:   true,
		ExpiresAt: &expiresAt,
	}
	assertNoError(t, repo.Create(tok), "Create")

	got, err := repo.Get(tok.ID)
	assertNoError(t, err, "Get")
	assertEqual(t, got.ExpiresAt != nil, true)
}

func TestLLMTokenRepository_CountRequestsToday(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)
	proxyRepo := NewLLMProxyRepository(db)

	tok := &model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "caller-test", Enabled: true}
	assertNoError(t, repo.Create(tok), "Create token")

	assertNoError(t, proxyRepo.CreateLog(&model.LLMProxyLog{ChannelName: "ch1", Model: "gpt-4o", CallerID: "caller-test"}), "CreateLog 1")
	assertNoError(t, proxyRepo.CreateLog(&model.LLMProxyLog{ChannelName: "ch1", Model: "gpt-4o", CallerID: "caller-test"}), "CreateLog 2")

	count, err := repo.CountRequestsToday(tok.ID)
	assertNoError(t, err, "CountRequestsToday")
	assertEqual(t, count, 2)
}

func TestLLMTokenRepository_TokensUsedToday(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)
	usageRepo := NewLLMTokenUsageRepository(db)

	tok := &model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "c1", Enabled: true}
	assertNoError(t, repo.Create(tok), "Create token")

	today := time.Now().Truncate(24 * time.Hour)
	assertNoError(t, usageRepo.AddUsage(tok.ID, today, 5, 100, 50, 0, 0, 0, 0), "AddUsage")

	used, err := repo.TokensUsedToday(tok.ID)
	assertNoError(t, err, "TokensUsedToday")
	assertEqual(t, used, 150)
}

func TestLLMTokenRepository_CostThisMonthCents(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)
	usageRepo := NewLLMTokenUsageRepository(db)

	tok := &model.LLMToken{Name: "t1", KeyHash: "h1", CallerID: "c1", Enabled: true}
	assertNoError(t, repo.Create(tok), "Create token")

	today := time.Now().Truncate(24 * time.Hour)
	assertNoError(t, usageRepo.AddUsage(tok.ID, today, 1, 0, 0, 0, 0, 3500, 0), "AddUsage")

	cost, err := repo.CostThisMonthCents(tok.ID)
	assertNoError(t, err, "CostThisMonthCents")
	assertEqual(t, cost, 4)
}

func TestLLMTokenRepository_IsModelGroupName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMTokenRepository(db)

	assertNoError(t, db.Create(&model.LLMModelGroup{Name: "gpt-4o-group", Strategy: "priority-health"}).Error, "Create group")

	isGroup, err := repo.IsModelGroupName("gpt-4o-group")
	assertNoError(t, err, "IsModelGroupName true")
	assertEqual(t, isGroup, true)

	isGroup2, err := repo.IsModelGroupName("nonexistent")
	assertNoError(t, err, "IsModelGroupName false")
	assertEqual(t, isGroup2, false)
}
