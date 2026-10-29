package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixRoomRepository_CreateAndGetByRoomID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixRoomRepository(db)

	room := &model.MatrixRoom{
		RoomID:   "!room1:matrix.org",
		RoomName: "Test Room",
		RoomType: "command",
		IsActive: true,
	}
	assertNoError(t, repo.Create(room), "Create")
	assertEqual(t, room.ID > 0, true)

	got, err := repo.GetByRoomID("!room1:matrix.org")
	assertNoError(t, err, "GetByRoomID")
	assertEqual(t, got.RoomName, "Test Room")
	assertEqual(t, got.RoomType, "command")
}

func TestMatrixRoomRepository_CreateConfigNil(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixRoomRepository(db)

	room := &model.MatrixRoom{
		RoomID:   "!room2:matrix.org",
		RoomName: "No Config",
		RoomType: "notification",
		IsActive: true,
		Config:   nil,
	}
	assertNoError(t, repo.Create(room), "Create nil config")

	got, err := repo.GetByRoomID("!room2:matrix.org")
	assertNoError(t, err, "GetByRoomID")
	assertEqual(t, got.Config != nil, true)
	assertEqual(t, *got.Config, "{}")
}

func TestMatrixRoomRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixRoomRepository(db)

	s := "{}"
	assertNoError(t, repo.Create(&model.MatrixRoom{RoomID: "!r1:m.org", RoomType: "command", IsActive: true, Config: &s}), "Create 1")
	assertNoError(t, repo.Create(&model.MatrixRoom{RoomID: "!r2:m.org", RoomType: "notification", IsActive: false, Config: &s}), "Create 2")

	rooms, err := repo.List(false)
	assertNoError(t, err, "List all")
	assertEqual(t, len(rooms), 2)

	rooms, err = repo.List(true)
	assertNoError(t, err, "List active")
	for _, r := range rooms {
		assertEqual(t, r.IsActive, true)
	}
}

func TestMatrixRoomRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixRoomRepository(db)

	room := &model.MatrixRoom{RoomID: "!r1:m.org", RoomType: "command", IsActive: true, Config: strPtr("{}")}
	assertNoError(t, repo.Create(room), "Create")

	room.RoomName = "Updated"
	assertNoError(t, repo.Update(room), "Update")

	got, err := repo.GetByRoomID("!r1:m.org")
	assertNoError(t, err, "GetByRoomID")
	assertEqual(t, got.RoomName, "Updated")
}

func TestMatrixRoomRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixRoomRepository(db)

	room := &model.MatrixRoom{RoomID: "!r1:m.org", RoomType: "command", IsActive: true, Config: strPtr("{}")}
	assertNoError(t, repo.Create(room), "Create")
	assertNoError(t, repo.Delete("!r1:m.org"), "Delete")

	_, err := repo.GetByRoomID("!r1:m.org")
	assertError(t, err, "GetByRoomID after delete")
}

func TestMatrixRoomRepository_Upsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixRoomRepository(db)
	ctx := context.Background()

	room1 := &model.MatrixRoom{
		RoomID:   "!r1:m.org",
		RoomName: "First",
		RoomType: "command",
		IsActive: true,
		Config:   strPtr("{}"),
	}
	assertNoError(t, repo.Upsert(ctx, room1), "Upsert insert")

	room2 := &model.MatrixRoom{
		RoomID:   "!r1:m.org",
		RoomName: "Second",
		RoomType: "notification",
		IsActive: false,
		Config:   strPtr("{}"),
	}
	assertNoError(t, repo.Upsert(ctx, room2), "Upsert update")

	got, err := repo.GetByRoomID("!r1:m.org")
	assertNoError(t, err, "GetByRoomID")
	assertEqual(t, got.RoomName, "Second")
	assertEqual(t, got.RoomType, "notification")
}

func strPtr(s string) *string {
	return &s
}

func TestMatrixRoomRepository_DiscoverPreservesSettings(t *testing.T) {
	repo := NewMatrixRoomRepository(setupTestDB(t))
	ctx := context.Background()
	const roomID = "!discovered:matrix.org"
	assertNoError(t, repo.Discover(ctx, roomID), "Discover new room")
	room, err := repo.GetByRoomID(roomID)
	assertNoError(t, err, "Get new room")
	assertEqual(t, room.RoomType, model.RoomTypeCommand)
	assertEqual(t, room.IsActive, true)
	room.RoomName, room.RoomType, room.IsActive = "Private chat", model.RoomTypeChat, false
	room.Config = strPtr(`{"owner":"@owner:matrix.org"}`)
	assertNoError(t, repo.Update(room), "Configure room")
	assertNoError(t, repo.Discover(ctx, roomID), "Rediscover existing room")
	got, err := repo.GetByRoomID(roomID)
	assertNoError(t, err, "Get preserved room")
	assertEqual(t, got.RoomName, room.RoomName)
	assertEqual(t, got.RoomType, room.RoomType)
	assertEqual(t, got.IsActive, false)
	var config map[string]string
	assertNoError(t, json.Unmarshal([]byte(*got.Config), &config), "Decode preserved config")
	assertEqual(t, config["owner"], "@owner:matrix.org")
}
