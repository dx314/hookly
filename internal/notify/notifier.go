// Package notify provides notification services for webhook delivery events.
package notify

import (
	"context"
	"time"
)

// WebhookInfo contains information about a webhook for notifications.
type WebhookInfo struct {
	ID             string
	EndpointID     string
	EndpointName   string
	DestinationURL string
	Attempts       int
	Error          string
	ReceivedAt     time.Time
}

// Notifier sends notifications for webhook events.
type Notifier interface {
	// NotifyDeliveryFailure sends a notification when a webhook fails permanently (4xx).
	NotifyDeliveryFailure(ctx context.Context, info WebhookInfo) error

	// NotifyDeadLetter sends a notification when a webhook becomes a dead letter.
	NotifyDeadLetter(ctx context.Context, info WebhookInfo) error
}

// NopNotifier is a no-op notifier that does nothing.
// Used when notifications are not configured.
type NopNotifier struct{}

// NotifyDeliveryFailure does nothing.
func (NopNotifier) NotifyDeliveryFailure(context.Context, WebhookInfo) error {
	return nil
}

// NotifyDeadLetter does nothing.
func (NopNotifier) NotifyDeadLetter(context.Context, WebhookInfo) error {
	return nil
}
