package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixChannelRepository_CreateAndGetByName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixChannelRepository(db)

	ch := &model.MatrixChannel{
		ChannelName: "alerts",
		RoomID:      "!alerts:matrix.org",
		IsActive:    true,
		Priority:    100,
		Config:      strPtr("{}"),
	}
	assertNoError(t, repo.Create(ch), "Create")
	assertEqual(t, ch.ID > 0, true)

	got, err := repo.GetByName("alerts")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.ChannelName, "alerts")
	assertEqual(t, got.Priority, 100)
}

func TestMatrixChannelRepository_CreateConfigNil(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixChannelRepository(db)

	ch := &model.MatrixChannel{
		ChannelName: "daily",
		RoomID:      "!daily:matrix.org",
		IsActive:    true,
		Config:      nil,
	}
	assertNoError(t, repo.Create(ch), "Create nil config")

	got, err := repo.GetByName("daily")
	assertNoError(t, err, "GetByName")
	assertEqual(t, got.Config != nil, true)
	assertEqual(t, *got.Config, "{}")
}

func TestMatrixChannelRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixChannelRepository(db)

	assertNoError(t, repo.Create(&model.MatrixChannel{ChannelName: "alerts", RoomID: "!r1:m.org", IsActive: true, Priority: 100, Config: strPtr("{}")}), "Create 1")
	assertNoError(t, repo.Create(&model.MatrixChannel{ChannelName: "daily", RoomID: "!r2:m.org", IsActive: false, Priority: 50, Config: strPtr("{}")}), "Create 2")

	channels, err := repo.List(false)
	assertNoError(t, err, "List all")
	assertEqual(t, len(channels), 2)

	channels, err = repo.List(true)
	assertNoError(t, err, "List active")
	for _, ch := range channels {
		assertEqual(t, ch.IsActive, true)
	}
}
