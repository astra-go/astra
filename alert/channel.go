// Package alert — notification channel implementations.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// defaultHTTPClient is used for webhook notifications with reasonable timeouts.
var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	},
}

// Channel is the interface implemented by all notification backends.
type Channel interface {
	// Name returns the unique identifier used to reference this channel in Rule.Channels.
	Name() string

	// Send delivers an alert notification. ctx is the evaluation context and
	// may carry a deadline. A non-nil error is logged but does not stop other
	// channels from being notified.
	Send(ctx context.Context, a *Alert) error
}

// ─── WebhookChannel ───────────────────────────────────────────────────────────

// WebhookChannel delivers alerts as HTTP POST requests with a JSON body.
//
// # Payload format
//
//	{
//	  "rule":     "high-error-rate",
//	  "expr":     "error_rate >= 0.05",
//	  "metrics":  {"error_rate": 0.07, "cpu_usage": 42.1},
//	  "labels":   {"severity": "critical"},
//	  "fired_at": "2024-01-15T10:30:00Z",
//	  "resolved": false
//	}
type WebhookChannel struct {
	// ChannelName is the identifier used in Rule.Channels.
	ChannelName string

	// URL is the endpoint that receives POST requests.
	URL string

	// Timeout for each POST request. Default: 5s.
	Timeout time.Duration

	// Headers are additional HTTP headers sent with each request.
	Headers map[string]string
}

// Name implements Channel.
func (w *WebhookChannel) Name() string { return w.ChannelName }

// Send implements Channel.
func (w *WebhookChannel) Send(ctx context.Context, a *Alert) error {
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	payload := map[string]any{
		"rule":     a.Rule.Name,
		"expr":     a.Rule.Expr,
		"metrics":  a.Metrics,
		"labels":   a.Rule.Labels,
		"fired_at": a.FiredAt.UTC().Format(time.RFC3339),
		"resolved": !a.ResolvedAt.IsZero(),
	}
	if !a.ResolvedAt.IsZero() {
		payload["resolved_at"] = a.ResolvedAt.UTC().Format(time.RFC3339)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("alert/webhook: marshal payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("alert/webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("alert/webhook: POST %s: %w", w.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("alert/webhook: POST %s returned %d", w.URL, resp.StatusCode)
	}
	return nil
}

// ─── LogChannel ───────────────────────────────────────────────────────────────

// LogChannel writes alert notifications to a slog.Logger.
// Useful for development, testing, and as a fallback channel.
type LogChannel struct {
	// ChannelName is the identifier used in Rule.Channels.
	ChannelName string

	// Logger to write to. Defaults to slog.Default() when nil.
	Logger *slog.Logger
}

// Name implements Channel.
func (l *LogChannel) Name() string { return l.ChannelName }

// Send implements Channel.
func (l *LogChannel) Send(_ context.Context, a *Alert) error {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}

	attrs := []any{
		slog.String("rule", a.Rule.Name),
		slog.String("expr", a.Rule.Expr),
		slog.Time("fired_at", a.FiredAt),
	}
	for k, v := range a.Rule.Labels {
		attrs = append(attrs, slog.String(k, v))
	}
	for k, v := range a.Metrics {
		attrs = append(attrs, slog.Float64(k, v))
	}

	if !a.ResolvedAt.IsZero() {
		attrs = append(attrs, slog.Time("resolved_at", a.ResolvedAt))
		logger.Info("alert: RESOLVED", attrs...)
	} else {
		logger.Warn("alert: FIRING", attrs...)
	}
	return nil
}
