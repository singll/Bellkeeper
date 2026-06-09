package service

import (
	"context"
	"strings"
	"testing"
)

func TestNotificationSenderReturnsNotReadyWithoutClient(t *testing.T) {
	sender := NewNotificationSender(nil, nil)
	err := sender.Send(context.Background(), &NotificationQueueMessage{
		NotificationID: "n1",
		RoomID:         "!room:example.org",
		Message:        "hello",
		MessageType:    "text",
	})
	if err == nil || !strings.Contains(err.Error(), "matrix client not ready") {
		t.Fatalf("Send err = %v, want matrix client not ready", err)
	}
}
