//go:build sms
// +build sms

package notify

// This file provides the SMS types and interfaces, enabled with build tag "sms".

import "context"

// SmsMessage represents an SMS to be sent.
type SmsMessage struct {
	// To is the recipient phone number in E.164 format (e.g. "+8613800138000").
	To string

	// Params is a map of template variable substitutions.
	// Keys and value semantics are provider-specific.
	Params map[string]string

	// TemplateCode overrides the default template configured in the Sender.
	// Leave empty to use the Sender's default.
	TemplateCode string

	// SignName overrides the default SMS sign name configured in the Sender.
	// Leave empty to use the Sender's default.
	SignName string
}

// SmsSender sends SMS messages.
// All implementations must be safe for concurrent use.
type SmsSender interface {
	// Send delivers msg to the recipient.
	Send(ctx context.Context, msg *SmsMessage) error
}
