//go:build email
// +build email

package notify

// This file provides the SMTP email sender, enabled with build tag "email".

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
	"os"
	"strconv"
	"strings"
	"time"
)

// SmtpConfig configures the SMTP sender.
type SmtpConfig struct {
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

	// EnvPrefix enables reading sensitive fields from environment variables.
	// When set (e.g. "SMTP"), the following env vars are checked:
	//   SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM
	// Environment variables take precedence over struct fields for Host,
	// Username, and Password. This avoids storing credentials in config files.
	EnvPrefix string

	// TLSConfig overrides the default TLS configuration.
	TLSConfig *tls.Config

	// DialTimeout is the connection timeout. Default: 10 seconds.
	DialTimeout time.Duration
}

// SmtpSender is an SMTP-backed EmailSender.
type SmtpSender struct {
	cfg SmtpConfig
}

// NewSmtpSender creates an SMTP-backed EmailSender.
// When SmtpConfig.EnvPrefix is set, sensitive fields are read from environment
// variables first (e.g. SMTP_PASSWORD), falling back to struct values.
func NewSmtpSender(cfg SmtpConfig) *SmtpSender {
	cfg.resolveFromEnv()
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	return &SmtpSender{cfg: cfg}
}

// resolveFromEnv reads Host, Username, Password, and Port from environment
// variables when EnvPrefix is set. Environment values take precedence over
// struct fields for security-sensitive data.
func (c *SmtpConfig) resolveFromEnv() {
	if c.EnvPrefix == "" {
		return
	}
	if v := os.Getenv(c.EnvPrefix + "_HOST"); v != "" {
		c.Host = v
	}
	if v := os.Getenv(c.EnvPrefix + "_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Port = n
		}
	}
	if v := os.Getenv(c.EnvPrefix + "_USERNAME"); v != "" {
		c.Username = v
	}
	if v := os.Getenv(c.EnvPrefix + "_PASSWORD"); v != "" {
		c.Password = v
	}
	if v := os.Getenv(c.EnvPrefix + "_FROM"); v != "" {
		c.From = v
	}
}

// Send delivers msg via SMTP. A new connection is opened per call.
func (s *SmtpSender) Send(ctx context.Context, msg *EmailMessage) error {
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
		dialer := &net.Dialer{Timeout: s.cfg.DialTimeout}
		conn, err2 := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err2 != nil {
			return fmt.Errorf("smtp: dial TLS: %w", err2)
		}
		c, err = smtp.NewClient(conn, s.cfg.Host)
	} else {
		c, err = smtp.Dial(addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: connect: %w", err)
	}
	defer c.Close()

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

	body, err := smtpBuildBody(from, msg)
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

func smtpBuildBody(from string, msg *EmailMessage) ([]byte, error) {
	var buf bytes.Buffer

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
		mw := multipart.NewWriter(&buf)
		buf.WriteString("Content-Type: multipart/mixed; boundary=" + mw.Boundary() + "\r\n\r\n")

		bodyBytes, err := smtpBuildAlternative(msg)
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

func smtpBuildAlternative(msg *EmailMessage) ([]byte, error) {
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
		qwh := quotedprintable.NewWriter(hw)
		qwh.Write([]byte(msg.HTMLBody))
		qwh.Close()
	}
	mw.Close()
	return buf.Bytes(), nil
}

// Verify SmtpSender implements EmailSender at compile time.
var _ EmailSender = (*SmtpSender)(nil)
