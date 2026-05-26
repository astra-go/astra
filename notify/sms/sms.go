// Package sms defines a provider-agnostic SMS sending abstraction.
//
// All backends implement the Sender interface, so application code is
// decoupled from the underlying provider (Aliyun / Tencent Cloud / Twilio …).
//
// # Quick start
//
//	import (
//	    "github.com/astra-go/astra/notify/sms"
//	    smsaliyun "github.com/astra-go/astra/notify/sms/aliyun"
//	)
//
//	sender := smsaliyun.New(smsaliyun.Config{
//	    AccessKeyID:     os.Getenv("ALIYUN_AK_ID"),
//	    AccessKeySecret: os.Getenv("ALIYUN_AK_SECRET"),
//	    SignName:        "MyApp",
//	    TemplateCode:    "SMS_123456",
//	})
//
//	err := sender.Send(ctx, &sms.Message{
//	    To:     "+8613800138000",
//	    Params: map[string]string{"code": "123456"},
//	})
package sms

import "context"

// Message represents an SMS to be sent.
type Message struct {
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

// Sender sends SMS messages.
// All implementations must be safe for concurrent use.
type Sender interface {
	// Send delivers msg to the recipient.
	Send(ctx context.Context, msg *Message) error
}
