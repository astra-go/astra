// Package push defines a provider-agnostic mobile push notification abstraction.
//
// All backends implement the Sender interface so application code is decoupled
// from the underlying provider (FCM / APNs / Huawei / Mi Push …).
//
// # Quick start
//
//	import (
//	    "github.com/astra-go/astra/notify/push"
//	    pushfcm "github.com/astra-go/astra/notify/push/fcm"
//	)
//
//	sender := pushfcm.New(pushfcm.Config{
//	    ServerKey: os.Getenv("FCM_SERVER_KEY"),
//	})
//
//	err := sender.Send(ctx, &push.Message{
//	    Token: deviceToken,
//	    Title: "New Order",
//	    Body:  "Your order #1234 has been shipped.",
//	    Data:  map[string]string{"order_id": "1234"},
//	})
package push

import "context"

// Message represents a push notification to be sent.
type Message struct {
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

// SendResult is the outcome of a single notification delivery.
type SendResult struct {
	// MessageID is the provider-assigned message identifier on success.
	MessageID string

	// Error is non-nil when delivery to this token/topic failed.
	Error error
}

// Sender sends push notifications.
// All implementations must be safe for concurrent use.
type Sender interface {
	// Send delivers msg to a single device token or topic.
	Send(ctx context.Context, msg *Message) (*SendResult, error)

	// SendBatch delivers multiple messages in a single round-trip (best-effort).
	// Returns one SendResult per message in the same order.
	SendBatch(ctx context.Context, msgs []*Message) ([]*SendResult, error)
}
