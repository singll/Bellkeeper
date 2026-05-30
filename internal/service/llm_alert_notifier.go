package service

import (
	"context"
	"fmt"
)

// matrixAlertNotifier adapts the Matrix NotificationService to the AlertNotifier
// interface so the LLM proxy's AlertAggregator can deliver aggregated alerts to a
// Matrix channel. It returns an error when delivery fails so the aggregator does
// not mark the alert as sent (which would suppress it for the dedup window).
type matrixAlertNotifier struct {
	svc     *NotificationService
	channel string
}

// NewMatrixAlertNotifier wraps a NotificationService as an AlertNotifier targeting
// the given channel (e.g. "alerts").
func NewMatrixAlertNotifier(svc *NotificationService, channel string) AlertNotifier {
	return &matrixAlertNotifier{svc: svc, channel: channel}
}

// Notify delivers a single aggregated alert message. A nil/unsuccessful response is
// treated as a failure so the caller can retry on the next flush.
func (m *matrixAlertNotifier) Notify(message string) error {
	resp, err := m.svc.Send(context.Background(), &NotificationRequest{
		Channel: m.channel,
		Message: message,
	})
	if err != nil {
		return err
	}
	if resp == nil || !resp.Success {
		detail := "delivery failed"
		if resp != nil && resp.Message != "" {
			detail = resp.Message
		}
		return fmt.Errorf("alert notify: %s", detail)
	}
	return nil
}
