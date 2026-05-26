// Package smtp provides an SMTP-backed email.Sender implementation.
//
// It supports both STARTTLS (port 587, default) and implicit TLS (port 465).
// Authentication uses PLAIN SASL over the encrypted connection.
//
// # Usage
//
//	import emailsmtp "github.com/astra-go/astra/notify/email/smtp"
//
//	sender := emailsmtp.New(emailsmtp.Config{
//	    Host:     "smtp.gmail.com",
//	    Port:     587,
//	    Username: "you@gmail.com",
//	    Password: os.Getenv("GMAIL_APP_PASSWORD"),
//	    From:     "you@gmail.com",
//	})
//
//	err := sender.Send(ctx, &email.Message{
//	    To:       []string{"alice@example.com"},
//	    Subject:  "Hello from Astra",
//	    TextBody: "Hello!",
//	    HTMLBody: "<b>Hello!</b>",
//	})
package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/astra-go/astra/notify/email"
)

// Config configures the SMTP sender.
type Config struct {
	// Host is the SMTP server hostname. Required.
	Host string

	// Port is the SMTP server port.
	// Default: 587 (STARTTLS). Use 465 for implicit TLS.
	Port int

	// Username is the SMTP authentication username.
	Username string

	// Password is the SMTP authentication password.
	Password string

	// From is the default sender address used when Message.From is empty.
	From string

	// ImplicitTLS enables implicit TLS (SMTPS on port 465).
	// When false (default), STARTTLS is used.
	ImplicitTLS bool

	// TLSConfig overrides the default TLS configuration.
	// Useful for custom CA certificates or mutual TLS.
	TLSConfig *tls.Config

	// DialTimeout is the connection timeout. Default: 10 seconds.
	DialTimeout time.Duration
}

// Sender is an SMTP-backed email.Sender.
type Sender struct {
	cfg Config
}

// New creates an SMTP-backed Sender.
func New(cfg Config) *Sender {
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	return &Sender{cfg: cfg}
}

// Send delivers msg via SMTP. A new connection is opened per call.
func (s *Sender) Send(ctx context.Context, msg *email.Message) error {
	from := msg.From
	if from == "" {
		from = s.cfg.From
	}
	if from == "" {
		return fmt.Errorf("smtp: sender address not set (set Message.From or Config.From)")
	}
	if len(msg.To) == 0 {
		return fmt.Errorf("smtp: at least one recipient (To) is required")
	}
	if msg.Subject == "" {
		return fmt.Errorf("smtp: subject is required")
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	tlsCfg := s.cfg.TLSConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: s.cfg.Host} //nolint:gosec
	}

	var c *smtp.Client
	var err error

	if s.cfg.ImplicitTLS {
		// Dial with TLS from the start (SMTPS, port 465).
		dialer := &net.Dialer{Timeout: s.cfg.DialTimeout}
		conn, err2 := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err2 != nil {
			return fmt.Errorf("smtp: dial TLS: %w", err2)
		}
		c, err = smtp.NewClient(conn, s.cfg.Host)
	} else {
		// Plain dial + STARTTLS upgrade.
		c, err = smtp.Dial(addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: connect: %w", err)
	}
	defer c.Close()

	// Honour context cancellation.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-done:
		}
	}()

	if !s.cfg.ImplicitTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("smtp: STARTTLS: %w", err)
			}
		}
	}

	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}

	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp: MAIL FROM: %w", err)
	}

	allTo := append(append([]string(nil), msg.To...), msg.CC...)
	allTo = append(allTo, msg.BCC...)
	for _, to := range allTo {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("smtp: RCPT TO %q: %w", to, err)
		}
	}

	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp: DATA: %w", err)
	}

	body, err := buildBody(from, msg)
	if err != nil {
		wc.Close()
		return err
	}
	if _, err := wc.Write(body); err != nil {
		wc.Close()
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp: close DATA: %w", err)
	}

	return c.Quit()
}

// ─── MIME message builder ─────────────────────────────────────────────────────

func buildBody(from string, msg *email.Message) ([]byte, error) {
	var buf bytes.Buffer

	// Headers
	buf.WriteString("From: " + mime.QEncoding.Encode("utf-8", from) + " <" + from + ">\r\n")
	buf.WriteString("To: " + strings.Join(msg.To, ", ") + "\r\n")
	if len(msg.CC) > 0 {
		buf.WriteString("Cc: " + strings.Join(msg.CC, ", ") + "\r\n")
	}
	if msg.ReplyTo != "" {
		buf.WriteString("Reply-To: " + msg.ReplyTo + "\r\n")
	}
	buf.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")

	hasText := msg.TextBody != ""
	hasHTML := msg.HTMLBody != ""
	hasAttach := len(msg.Attachments) > 0

	switch {
	case hasAttach:
		// multipart/mixed wraps content + attachments
		mw := multipart.NewWriter(&buf)
		buf.WriteString("Content-Type: multipart/mixed; boundary=" + mw.Boundary() + "\r\n\r\n")

		// body part (text / html / alternative)
		bodyBytes, err := buildAlternative(msg)
		if err != nil {
			return nil, err
		}
		part, _ := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type": {"multipart/alternative"},
		})
		part.Write(bodyBytes)

		for _, att := range msg.Attachments {
			ct := att.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			h := textproto.MIMEHeader{
				"Content-Type":              {ct + "; name=\"" + att.Filename + "\""},
				"Content-Disposition":       {"attachment; filename=\"" + att.Filename + "\""},
				"Content-Transfer-Encoding": {"base64"},
			}
			aw, _ := mw.CreatePart(h)
			enc := base64.NewEncoder(base64.StdEncoding, aw)
			enc.Write(att.Data)
			enc.Close()
		}
		mw.Close()

	case hasText && hasHTML:
		mw := multipart.NewWriter(&buf)
		buf.WriteString("Content-Type: multipart/alternative; boundary=" + mw.Boundary() + "\r\n\r\n")

		pw, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
		qw := quotedprintable.NewWriter(pw)
		qw.Write([]byte(msg.TextBody))
		qw.Close()

		hw, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
		qwh := quotedprintable.NewWriter(hw)
		qwh.Write([]byte(msg.HTMLBody))
		qwh.Close()
		mw.Close()

	case hasHTML:
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		qw := quotedprintable.NewWriter(&buf)
		qw.Write([]byte(msg.HTMLBody))
		qw.Close()

	default:
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		qw := quotedprintable.NewWriter(&buf)
		qw.Write([]byte(msg.TextBody))
		qw.Close()
	}

	return buf.Bytes(), nil
}

func buildAlternative(msg *email.Message) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if msg.TextBody != "" {
		pw, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
		qw := quotedprintable.NewWriter(pw)
		qw.Write([]byte(msg.TextBody))
		qw.Close()
	}
	if msg.HTMLBody != "" {
		hw, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
		qw := quotedprintable.NewWriter(hw)
		qw.Write([]byte(msg.HTMLBody))
		qw.Close()
	}
	mw.Close()
	return buf.Bytes(), nil
}

// Compile-time assertion.
var _ email.Sender = (*Sender)(nil)
