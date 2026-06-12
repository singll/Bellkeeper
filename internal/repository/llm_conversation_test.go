package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestConversationBindingRepository_CreateAndGetByConversationID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewConversationBindingRepository(db)

	b := &model.LLMConversationBinding{
		ConversationID: "conv-1",
		ChannelID:      1,
		ChannelName:    "openai-main",
		Model:          "gpt-4o",
		TaskType:       "chat",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(30 * time.Minute),
		RequestCount:   1,
		TotalTokens:    100,
		TotalCostCents: 5,
	}
	assertNoError(t, repo.Create(b), "Create")

	got, err := repo.GetByConversationID("conv-1")
	assertNoError(t, err, "GetByConversationID")
	assertEqual(t, got.ChannelName, "openai-main")
	assertEqual(t, got.Model, "gpt-4o")
}

func TestConversationBindingRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewConversationBindingRepository(db)

	b1 := &model.LLMConversationBinding{
		ConversationID: "conv-1",
		ChannelID:      1,
		ChannelName:    "ch1",
		Model:          "m1",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour),
	}
	b2 := &model.LLMConversationBinding{
		ConversationID: "conv-2",
		ChannelID:      2,
		ChannelName:    "ch2",
		Model:          "m2",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour),
	}
	assertNoError(t, repo.Create(b1), "Create 1")
	assertNoError(t, repo.Create(b2), "Create 2")

	bindings, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(bindings), 2)
}

func TestConversationBindingRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewConversationBindingRepository(db)

	b := &model.LLMConversationBinding{
		ConversationID: "conv-1",
		ChannelID:      1,
		ChannelName:    "ch1",
		Model:          "m1",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		RequestCount:   1,
	}
	assertNoError(t, repo.Create(b), "Create")

	b.RequestCount = 5
	b.TotalTokens = 500
	assertNoError(t, repo.Update(b), "Update")

	got, err := repo.GetByConversationID("conv-1")
	assertNoError(t, err, "GetByConversationID")
	assertEqual(t, got.RequestCount, 5)
	assertEqual(t, got.TotalTokens, 500)
}

func TestConversationBindingRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewConversationBindingRepository(db)

	b := &model.LLMConversationBinding{
		ConversationID: "conv-1",
		ChannelID:      1,
		ChannelName:    "ch1",
		Model:          "m1",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour),
	}
	assertNoError(t, repo.Create(b), "Create")
	assertNoError(t, repo.Delete("conv-1"), "Delete")

	_, err := repo.GetByConversationID("conv-1")
	assertError(t, err, "GetByConversationID after delete")
}

func TestConversationBindingRepository_Upsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewConversationBindingRepository(db)

	b1 := &model.LLMConversationBinding{
		ConversationID: "conv-1",
		ChannelID:      1,
		ChannelName:    "ch1",
		Model:          "m1",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		RequestCount:   1,
	}
	assertNoError(t, repo.Upsert(b1), "Upsert insert")

	b2 := &model.LLMConversationBinding{
		ConversationID: "conv-1",
		ChannelID:      2,
		ChannelName:    "ch2",
		Model:          "m2",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		RequestCount:   3,
		TotalTokens:    300,
	}
	assertNoError(t, repo.Upsert(b2), "Upsert update")

	got, err := repo.GetByConversationID("conv-1")
	assertNoError(t, err, "GetByConversationID")
	assertEqual(t, got.ChannelName, "ch2")
	assertEqual(t, got.RequestCount, 3)
}

func TestConversationBindingRepository_CleanupExpired(t *testing.T) {
	db := setupTestDB(t)
	repo := NewConversationBindingRepository(db)

	b1 := &model.LLMConversationBinding{
		ConversationID: "conv-expired",
		ChannelID:      1,
		ChannelName:    "ch1",
		Model:          "m1",
		FirstSeenAt:    time.Now().Add(-2 * time.Hour),
		LastSeenAt:     time.Now().Add(-2 * time.Hour),
		ExpiresAt:      time.Now().Add(-1 * time.Hour),
	}
	b2 := &model.LLMConversationBinding{
		ConversationID: "conv-active",
		ChannelID:      2,
		ChannelName:    "ch2",
		Model:          "m2",
		FirstSeenAt:    time.Now(),
		LastSeenAt:     time.Now(),
		ExpiresAt:      time.Now().Add(1 * time.Hour),
	}
	assertNoError(t, repo.Create(b1), "Create expired")
	assertNoError(t, repo.Create(b2), "Create active")

	assertNoError(t, repo.CleanupExpired(), "CleanupExpired")

	bindings, err := repo.List()
	assertNoError(t, err, "List")
	assertEqual(t, len(bindings), 1)
	assertEqual(t, bindings[0].ConversationID, "conv-active")
}
