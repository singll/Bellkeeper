package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestLLMChannelBalanceSnapshotRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelBalanceSnapshotRepository(db)

	snap := &model.LLMChannelBalanceSnapshot{
		ChannelID:    1,
		ChannelName:  "openai-main",
		BalanceUSD:   50.0,
		Currency:     "USD",
		TotalGranted: 100.0,
		TotalUsed:    50.0,
		BalanceRaw:   `{"balance":50}`,
		LatencyMs:    200,
		FetchedAt:    time.Now(),
	}
	assertNoError(t, repo.Create(snap), "Create")
	assertEqual(t, snap.ID > 0, true)
}

func TestLLMChannelBalanceSnapshotRepository_ListByChannelName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelBalanceSnapshotRepository(db)

	now := time.Now()
	assertNoError(t, repo.Create(&model.LLMChannelBalanceSnapshot{
		ChannelID:   1, ChannelName: "openai-main", BalanceUSD: 50, FetchedAt: now.Add(-1 * time.Hour),
	}), "Create 1")
	assertNoError(t, repo.Create(&model.LLMChannelBalanceSnapshot{
		ChannelID:   1, ChannelName: "openai-main", BalanceUSD: 45, FetchedAt: now,
	}), "Create 2")
	assertNoError(t, repo.Create(&model.LLMChannelBalanceSnapshot{
		ChannelID:   2, ChannelName: "deepseek", BalanceUSD: 30, FetchedAt: now,
	}), "Create 3")

	snaps, err := repo.ListByChannelName("openai-main", 10)
	assertNoError(t, err, "ListByChannelName")
	assertEqual(t, len(snaps), 2)
	assertEqual(t, snaps[0].BalanceUSD, 45.0)
}

func TestLLMChannelBalanceSnapshotRepository_ListByChannelNameEmpty(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLLMChannelBalanceSnapshotRepository(db)

	snaps, err := repo.ListByChannelName("nonexistent", 10)
	assertNoError(t, err, "ListByChannelName nonexistent")
	assertEqual(t, len(snaps), 0)
}
