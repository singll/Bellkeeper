package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

func TestActivityLogRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	log := &model.ActivityLog{
		Module:     "rss",
		Action:     "fetch",
		Status:     "success",
		Summary:    "Fetched 10 feeds",
		RefID:      "feed-1",
		DurationMs: 500,
	}
	assertNoError(t, repo.Create(log), "Create")
	assertEqual(t, log.ID > 0, true)
}

func TestActivityLogRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success"}), "Create 1")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "llm", Action: "proxy", Status: "failure"}), "Create 2")

	logs, total, err := repo.List(ActivityLogQuery{Limit: 10})
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(logs), 2)
}

func TestActivityLogRepository_ListWithModuleFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success"}), "Create 1")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "llm", Action: "proxy", Status: "failure"}), "Create 2")

	logs, total, err := repo.List(ActivityLogQuery{Module: "rss", Limit: 10})
	assertNoError(t, err, "List module=rss")
	assertEqual(t, total, int64(1))
	assertEqual(t, logs[0].Module, "rss")
}

func TestActivityLogRepository_ListWithStatusFilter(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success"}), "Create 1")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "llm", Action: "proxy", Status: "failure"}), "Create 2")

	logs, total, err := repo.List(ActivityLogQuery{Status: "failure", Limit: 10})
	assertNoError(t, err, "List status=failure")
	assertEqual(t, total, int64(1))
	assertEqual(t, logs[0].Module, "llm")
}

func TestActivityLogRepository_GetDistinctModules(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success"}), "Create 1")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "llm", Action: "proxy", Status: "success"}), "Create 2")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "parse", Status: "success"}), "Create 3")

	modules, err := repo.GetDistinctModules()
	assertNoError(t, err, "GetDistinctModules")
	assertEqual(t, len(modules), 2)
}

func TestActivityLogRepository_GetModuleStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success"}), "Create 1")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "parse", Status: "failure"}), "Create 2")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "llm", Action: "proxy", Status: "success"}), "Create 3")

	stats, err := repo.GetModuleStats(time.Time{})
	assertNoError(t, err, "GetModuleStats")
	assertEqual(t, len(stats), 2)
}

func TestActivityLogRepository_GetActionStats(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success"}), "Create 1")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "failure"}), "Create 2")

	stats, err := repo.GetActionStats("rss", time.Time{})
	assertNoError(t, err, "GetActionStats")
	assertEqual(t, len(stats), 2)
}

func TestActivityLogRepository_GetRecentFailures(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "failure", Summary: "err1"}), "Create 1")
	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success", Summary: "ok"}), "Create 2")

	logs, err := repo.GetRecentFailures("rss", time.Time{}, 10)
	assertNoError(t, err, "GetRecentFailures")
	assertEqual(t, len(logs), 1)
	assertEqual(t, logs[0].Status, "failure")
}

func TestActivityLogRepository_CleanOldLogs(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityLogRepository(db)

	assertNoError(t, repo.Create(&model.ActivityLog{Module: "rss", Action: "fetch", Status: "success"}), "Create")

	affected, err := repo.CleanOldLogs(0)
	assertNoError(t, err, "CleanOldLogs")
	assertEqual(t, affected, int64(1))
}
