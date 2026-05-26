// Package email defines a broker-agnostic email sending abstraction.
//
// All backends implement the Sender interface so application code is decoupled
// from the underlying transport (SMTP, SendGrid, SES, Mailgun, …).
//
// # Quick start
//
//	import (
//	    "github.com/astra-go/astra/notify/email"
//	    emailsmtp "github.com/astra-go/astra/notify/email/smtp"
//	)
//
//	sender := emailsmtp.New(emailsmtp.Config{
//	    Host:     "smtp.example.com",
//	    Port:     587,
//	    Username: "user@example.com",
//	    Password: os.Getenv("SMTP_PASSWORD"),
//	    From:     "no-reply@example.com",
//	})
//
//	err := sender.Send(ctx, &email.Message{
//	    To:       []string{"alice@example.com"},
//	    Subject:  "Welcome!",
//	    HTMLBody: "<h1>Hello Alice</h1>",
//	    TextBody: "Hello Alice",
//	})
package email

import "context"

// Message represents an email message.
type Message struct {
	// From overrides the sender address. If empty, the backend's default From is used.
	From string

	// To is the list of primary recipients. Required.
	To []string

	// CC is the list of carbon-copy recipients.
	CC []string

	// BCC is the list of blind carbon-copy recipients.
	BCC []string

	// ReplyTo sets the Reply-To header.
	ReplyTo string

	// Subject is the email subject line. Required.
	Subject string

	// TextBody is the plain-text version of the message body.
	// At least one of TextBody or HTMLBody must be provided.
	TextBody string

	// HTMLBody is the HTML version of the message body.
	// When both TextBody and HTMLBody are set, the message is sent as
	// multipart/alternative so the recipient's client chooses the best format.
	HTMLBody string

	// Attachments is a list of files to attach to the message.
	Attachments []Attachment
}

// Attachment represents a file to attach to an email.
type Attachment struct {
	// Filename is the name shown in the email client.
	Filename string

	// ContentType is the MIME type (e.g. "application/pdf"). If empty,
	// "application/octet-stream" is used.
	ContentType string

	// Data is the raw file content.
	Data []byte
}

// Sender sends email messages.
// All implementations must be safe for concurrent use.
type Sender interface {
	// Send delivers msg to all recipients.
	Send(ctx context.Context, msg *Message) error
}
