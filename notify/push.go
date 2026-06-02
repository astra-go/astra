//go:build push
// +build push

package notify

// This file provides the push notification types and interfaces, enabled with build tag "push".

import "context"

// PushMessage represents a push notification to be sent.
type PushMessage struct {
	// Token is the device registration token (FCM) or device token (APNs).
	// Required for single-device delivery.
	Token string

	// Topic is an FCM topic or APNs topic (bundle ID).
	// When set, the message is delivered to all subscribers.
	// Mutually exclusive with Token.
	Topic string

	// Title is the notification title. Displayed in the system tray.
	Title string

	// Body is the notification body text.
	Body string

	// ImageURL is an optional URL for a notification image (FCM only).
	ImageURL string

	// Data is a map of arbitrary key-value pairs delivered to the app (silent/data push).
	Data map[string]string

	// Badge is the app icon badge number (APNs / supported FCM targets).
	Badge int

	// Sound is the notification sound file name. "default" uses the system sound.
	Sound string

	// Priority controls delivery priority. "high" wakes sleeping devices.
	// Default: "normal". Values: "normal", "high".
	Priority string

	// CollapseKey groups related notifications so only the latest is shown (FCM).
	CollapseKey string

	// TTL is the time-to-live in seconds. 0 means the provider's default.
	TTL int
}

// PushSendResult is the outcome of a single notification delivery.
type PushSendResult struct {
	// MessageID is the provider-assigned message identifier on success.
	MessageID string

	// Error is non-nil when delivery to this token/topic failed.
	Error error
}

// PushSender sends push notifications.
// All implementations must be safe for concurrent use.
type PushSender interface {
	// Send delivers msg to a single device token or topic.
	Send(ctx context.Context, msg *PushMessage) (*PushSendResult, error)

	// SendBatch delivers multiple messages in a single round-trip (best-effort).
	// Returns one PushSendResult per message in the same order.
	SendBatch(ctx context.Context, msgs []*PushMessage) ([]*PushSendResult, error)
}
