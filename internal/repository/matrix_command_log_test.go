package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixCommandLogRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandLogRepository(db)

	log := &model.MatrixCommandLog{
		EventID:         "$event1:matrix.org",
		RoomID:          "!room1:matrix.org",
		Sender:          "@user:matrix.org",
		CommandName:     "help",
		HandlerType:     "builtin_help",
		ExecutionStatus: "success",
		ExecutionTimeMs: 50,
	}
	assertNoError(t, repo.Create(log), "Create")
	assertEqual(t, log.ID > 0, true)
}

func TestMatrixCommandLogRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandLogRepository(db)

	assertNoError(t, repo.Create(&model.MatrixCommandLog{
		EventID: "$e1:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org",
		CommandName: "help", ExecutionStatus: "success",
	}), "Create 1")
	assertNoError(t, repo.Create(&model.MatrixCommandLog{
		EventID: "$e2:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org",
		CommandName: "status", ExecutionStatus: "failed",
	}), "Create 2")

	logs, total, err := repo.List(1, 10, "", "")
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(logs), 2)
}

func TestMatrixCommandLogRepository_ListWithFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandLogRepository(db)

	assertNoError(t, repo.Create(&model.MatrixCommandLog{
		EventID: "$e1:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org",
		CommandName: "help", ExecutionStatus: "success",
	}), "Create 1")
	assertNoError(t, repo.Create(&model.MatrixCommandLog{
		EventID: "$e2:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org",
		CommandName: "status", ExecutionStatus: "failed",
	}), "Create 2")

	_, total, err := repo.List(1, 10, "", "success")
	assertNoError(t, err, "List status=success")
	assertEqual(t, total, int64(1))
}

func TestMatrixCommandLogRepository_Complete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandLogRepository(db)

	log := &model.MatrixCommandLog{
		EventID:         "$event1:matrix.org",
		RoomID:          "!room1:matrix.org",
		Sender:          "@user:matrix.org",
		CommandName:     "help",
		HandlerType:     "builtin_help",
		ExecutionStatus: "pending",
	}
	assertNoError(t, repo.Create(log), "Create")

	assertNoError(t, repo.Complete("$event1:matrix.org", "success", "", "$resp:matrix.org", 50), "Complete")

	var got model.MatrixCommandLog
	assertNoError(t, db.Where("event_id = ?", "$event1:matrix.org").First(&got).Error, "Find")
	assertEqual(t, got.ExecutionStatus, "success")
	assertEqual(t, got.ExecutionTimeMs, 50)
	assertEqual(t, got.ResponseEventID, "$resp:matrix.org")
}

func TestMatrixCommandLogRepository_CountByStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixCommandLogRepository(db)

	assertNoError(t, repo.Create(&model.MatrixCommandLog{
		EventID: "$e1:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org",
		CommandName: "help", ExecutionStatus: "success",
	}), "Create 1")
	assertNoError(t, repo.Create(&model.MatrixCommandLog{
		EventID: "$e2:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org",
		CommandName: "status", ExecutionStatus: "failed",
	}), "Create 2")
	assertNoError(t, repo.Create(&model.MatrixCommandLog{
		EventID: "$e3:m.org", RoomID: "!r1:m.org", Sender: "@u:m.org",
		CommandName: "help", ExecutionStatus: "success",
	}), "Create 3")

	since := time.Now().Add(-1 * time.Hour)
	counts, err := repo.CountByStatus(since)
	assertNoError(t, err, "CountByStatus")
	assertEqual(t, counts["success"], int64(2))
	assertEqual(t, counts["failed"], int64(1))
}
