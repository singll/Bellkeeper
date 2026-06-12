package repository

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixEventRepository_CreateAndExistsByEventID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixEventRepository(db)

	event := &model.MatrixEvent{
		EventID:   "$event1:matrix.org",
		RoomID:    "!room1:matrix.org",
		Sender:    "@user:matrix.org",
		EventType: "m.room.message",
		Content:   strPtr(`{"body":"hello"}`),
	}
	assertNoError(t, repo.Create(event), "Create")
	assertEqual(t, event.ID > 0, true)

	exists, err := repo.ExistsByEventID("$event1:matrix.org")
	assertNoError(t, err, "ExistsByEventID")
	assertEqual(t, exists, true)

	exists, err = repo.ExistsByEventID("$nonexistent:matrix.org")
	assertNoError(t, err, "ExistsByEventID nonexistent")
	assertEqual(t, exists, false)
}

func TestMatrixEventRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixEventRepository(db)

	event := &model.MatrixEvent{
		EventID:   "$event1:matrix.org",
		RoomID:    "!room1:matrix.org",
		Sender:    "@user:matrix.org",
		EventType: "m.room.message",
	}
	assertNoError(t, repo.Create(event), "Create")

	assertNoError(t, repo.UpdateStatus("$event1:matrix.org", "processed", ""), "UpdateStatus")

	events, _, err := repo.List(MatrixEventQuery{Status: "processed", Limit: 10})
	assertNoError(t, err, "List")
	assertEqual(t, len(events), 1)
}

func TestMatrixEventRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixEventRepository(db)

	assertNoError(t, repo.Create(&model.MatrixEvent{
		EventID: "$e1:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org", EventType: "m.room.message",
	}), "Create 1")
	assertNoError(t, repo.Create(&model.MatrixEvent{
		EventID: "$e2:m.org", RoomID: "!r2:m.org", Sender: "@u:m.org", EventType: "m.room.member",
	}), "Create 2")

	events, total, err := repo.List(MatrixEventQuery{Limit: 10})
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(events), 2)
}

func TestMatrixEventRepository_ListWithRoomFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixEventRepository(db)

	assertNoError(t, repo.Create(&model.MatrixEvent{
		EventID: "$e1:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org", EventType: "m.room.message",
	}), "Create")
	assertNoError(t, repo.Create(&model.MatrixEvent{
		EventID: "$e2:m.org", RoomID: "!r2:m.org", Sender: "@u:m.org", EventType: "m.room.member",
	}), "Create 2")

	events, total, err := repo.List(MatrixEventQuery{RoomID: "!r1:m.org", Limit: 10})
	assertNoError(t, err, "List room filter")
	assertEqual(t, total, int64(1))
	assertEqual(t, len(events), 1)
}
