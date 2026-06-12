package repository

import (
	"context"
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixSyncStateRepository_Get(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixSyncStateRepository(db)

	state := &model.MatrixSyncState{
		BotUserID:  "@bot:matrix.org",
		NextBatch:  "batch-token-1",
		FilterID:   "filter-1",
	}
	assertNoError(t, db.Create(state).Error, "Create")

	got, err := repo.Get("@bot:matrix.org")
	assertNoError(t, err, "Get")
	assertEqual(t, got.NextBatch, "batch-token-1")

	_, err = repo.Get("@nonexistent:matrix.org")
	assertError(t, err, "Get nonexistent")
}

func TestMatrixSyncStateRepository_Upsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixSyncStateRepository(db)
	ctx := context.Background()

	state1 := &model.MatrixSyncState{
		BotUserID: "@bot:matrix.org",
		NextBatch: "batch-1",
	}
	assertNoError(t, repo.Upsert(ctx, state1), "Upsert insert")

	state2 := &model.MatrixSyncState{
		BotUserID: "@bot:matrix.org",
		NextBatch: "batch-2",
	}
	assertNoError(t, repo.Upsert(ctx, state2), "Upsert update")

	got, err := repo.Get("@bot:matrix.org")
	assertNoError(t, err, "Get")
	assertEqual(t, got.NextBatch, "batch-2")
}
