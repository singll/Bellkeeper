package repository

import (
	"context"
	"testing"

	"github.com/singll/bellkeeper/internal/model"
)

func TestMatrixNotificationRepository_CreateAndGetByNotificationID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixNotificationRepository(db)
	ctx := context.Background()

	n := &model.MatrixNotification{
		NotificationID: "notif-1",
		ChannelName:    "alerts",
		MessageType:    "text",
		MessageContent: "Alert triggered",
		Status:         "pending",
		Severity:       "error",
	}
	assertNoError(t, repo.Create(ctx, n), "Create")
	assertEqual(t, n.ID > 0, true)

	got, err := repo.GetByNotificationID(ctx, "notif-1")
	assertNoError(t, err, "GetByNotificationID")
	assertEqual(t, got.ChannelName, "alerts")
	assertEqual(t, got.MessageContent, "Alert triggered")

	got, err = repo.GetByNotificationID(ctx, "nonexistent")
	assertNoError(t, err, "GetByNotificationID nonexistent")
	assertEqual(t, got, nil)
}

func TestMatrixNotificationRepository_MarkSent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixNotificationRepository(db)
	ctx := context.Background()

	n := &model.MatrixNotification{
		NotificationID: "notif-1",
		ChannelName:    "alerts",
		MessageContent: "Test",
		Status:         "pending",
	}
	assertNoError(t, repo.Create(ctx, n), "Create")

	assertNoError(t, repo.MarkSent(ctx, "notif-1", "$sent:matrix.org"), "MarkSent")

	got, err := repo.GetByNotificationID(ctx, "notif-1")
	assertNoError(t, err, "GetByNotificationID")
	assertEqual(t, got.Status, "sent")
	assertEqual(t, got.SentEventID, "$sent:matrix.org")
}

func TestMatrixNotificationRepository_GetFailed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixNotificationRepository(db)
	ctx := context.Background()

	assertNoError(t, repo.Create(ctx, &model.MatrixNotification{
		NotificationID: "n1", ChannelName: "alerts", MessageContent: "m1", Status: "failed", RetryCount: 1,
	}), "Create 1")
	assertNoError(t, repo.Create(ctx, &model.MatrixNotification{
		NotificationID: "n2", ChannelName: "alerts", MessageContent: "m2", Status: "failed", RetryCount: 5,
	}), "Create 2")
	assertNoError(t, repo.Create(ctx, &model.MatrixNotification{
		NotificationID: "n3", ChannelName: "alerts", MessageContent: "m3", Status: "pending",
	}), "Create 3")

	notifications, err := repo.GetFailed(ctx, 3)
	assertNoError(t, err, "GetFailed")
	assertEqual(t, len(notifications), 1)
	assertEqual(t, notifications[0].NotificationID, "n1")
}

func TestMatrixNotificationRepository_GetByChannel(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixNotificationRepository(db)
	ctx := context.Background()

	assertNoError(t, repo.Create(ctx, &model.MatrixNotification{
		NotificationID: "n1", ChannelName: "alerts", MessageContent: "m1", Status: "sent",
	}), "Create 1")
	assertNoError(t, repo.Create(ctx, &model.MatrixNotification{
		NotificationID: "n2", ChannelName: "daily", MessageContent: "m2", Status: "sent",
	}), "Create 2")

	notifications, err := repo.GetByChannel(ctx, "alerts", 10)
	assertNoError(t, err, "GetByChannel")
	assertEqual(t, len(notifications), 1)
}

func TestMatrixNotificationRepository_GetRecent(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixNotificationRepository(db)
	ctx := context.Background()

	assertNoError(t, repo.Create(ctx, &model.MatrixNotification{
		NotificationID: "n1", ChannelName: "alerts", MessageContent: "m1", Status: "sent",
	}), "Create")

	notifications, err := repo.GetRecent(ctx, 10)
	assertNoError(t, err, "GetRecent")
	assertEqual(t, len(notifications), 1)
}

func TestMatrixNotificationRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMatrixNotificationRepository(db)
	ctx := context.Background()

	n := &model.MatrixNotification{
		NotificationID: "notif-1",
		ChannelName:    "alerts",
		MessageContent: "Test",
		Status:         "pending",
		RetryCount:     0,
	}
	assertNoError(t, repo.Create(ctx, n), "Create")

	assertNoError(t, repo.UpdateStatus(ctx, "notif-1", "retrying", "connection error"), "UpdateStatus")

	got, err := repo.GetByNotificationID(ctx, "notif-1")
	assertNoError(t, err, "GetByNotificationID")
	assertEqual(t, got.Status, "retrying")
	assertEqual(t, got.RetryCount, 1)

	assertNoError(t, repo.UpdateStatus(ctx, "notif-1", "retrying", "still failing"), "UpdateStatus 2")
	got2, err := repo.GetByNotificationID(ctx, "notif-1")
	assertNoError(t, err, "GetByNotificationID 2")
	assertEqual(t, got2.RetryCount, 2)
}
