// Package middleware provides common HTTP middleware for Astra.
package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/astra-go/astra"
)

// SpanExtractor extracts distributed tracing IDs from an HTTP request.
// Implement this interface to bridge Logger with your tracing backend.
// Use OTelSpanExtractor for OpenTelemetry.
type SpanExtractor interface {
	TraceID(r *http.Request) string
	SpanID(r *http.Request) string
}

// LoggerConfig configures the Logger middleware.
type LoggerConfig struct {
	// Logger is the slog.Logger to use. Defaults to slog.Default().
	Logger *slog.Logger
	// SkipPaths are paths that should not be logged.
	SkipPaths []string
	// Format is the log format: "json" or "text" (default "text").
	Format string
	// SensitiveParams lists query-parameter names whose values are replaced
	// with "REDACTED" in access logs. Set to an empty slice to disable.
	// Default: defaultLogSensitiveParams (tokens, passwords, secrets, keys …).
	SensitiveParams []string
	// SpanExtractor injects trace_id and span_id into every log record.
	// When nil, trace context is not logged.
	// Use OTelSpanExtractor for OpenTelemetry.
	SpanExtractor SpanExtractor
}

// DefaultLoggerConfig is the default logger configuration.
var DefaultLoggerConfig = LoggerConfig{
	Format:          "text",
	SensitiveParams: DefaultSensitiveParams,
}

// Logger returns a middleware that logs each HTTP request.
func Logger() astra.HandlerFunc {
	return LoggerWithConfig(DefaultLoggerConfig)
}

// LoggerWithConfig returns a Logger middleware with the given configuration.
func LoggerWithConfig(cfg LoggerConfig) astra.HandlerFunc {
	if cfg.Logger == nil {
		var handler slog.Handler
		if cfg.Format == "json" {
			handler = slog.NewJSONHandler(os.Stdout, nil)
		} else {
			handler = slog.NewTextHandler(os.Stdout, nil)
		}
		cfg.Logger = slog.New(handler)
	}

	skip := make(map[string]bool, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skip[p] = true
	}

	sensitiveSet := buildSensitiveSet(cfg.SensitiveParams)

	return func(c *astra.Ctx) error {
		req := c.Request()
		if skip[req.URL.Path] {
			return nil
		}

		start := time.Now()
		rawPath := req.URL.Path
		rawQuery := req.URL.RawQuery

		// Process request.
		c.Next()

		latency := time.Since(start)

		status := c.Writer().Status()
		clientIP := c.ClientIP()
		size := c.Writer().Size()
		method := req.Method // direct field — no dispatch regardless

		// Build the logged path: append a sanitized query string if present.
		logPath := rawPath
		if rawQuery != "" {
			logPath = rawPath + "?" + sanitizeRawQuery(rawQuery, sensitiveSet)
		}

		// Color-code the status for text format only; JSON consumers must not
		// receive ANSI escape sequences embedded in field values.
		var statusStr string
		if cfg.Format == "json" {
			statusStr = fmt.Sprintf("%d", status)
		} else {
			statusStr = colorStatus(status)
		}

		args := []any{
			slog.String("status", statusStr),
			slog.String("method", method),
			slog.String("path", logPath),
			slog.String("ip", clientIP),
			slog.Duration("latency", latency),
			slog.Int("size", size),
		}
		if cfg.SpanExtractor != nil {
				if tid := cfg.SpanExtractor.TraceID(req); tid != "" {
					args = append(args, slog.String("trace_id", tid))
				}
				if sid := cfg.SpanExtractor.SpanID(req); sid != "" {
					args = append(args, slog.String("span_id", sid))
				}
			}
		cfg.Logger.Info("request", args...)

		return nil
	}
}

func colorStatus(code int) string {
	switch {
	case code >= 500:
		return fmt.Sprintf("\033[31m%d\033[0m", code) // red
	case code >= 400:
		return fmt.Sprintf("\033[33m%d\033[0m", code) // yellow
	case code >= 300:
		return fmt.Sprintf("\033[36m%d\033[0m", code) // cyan
	default:
		return fmt.Sprintf("\033[32m%d\033[0m", code) // green
	}
}

// WithLoggerSkipPaths sets paths excluded from access logging.
func WithLoggerSkipPaths(paths ...string) func(*LoggerConfig) {
	return func(c *LoggerConfig) { c.SkipPaths = paths }
}

// WithLoggerFormat sets the log format ("json" or "text").
func WithLoggerFormat(format string) func(*LoggerConfig) {
	return func(c *LoggerConfig) { c.Format = format }
}

// WithLoggerSensitiveParams replaces the default list of redacted
// query-parameter names in access logs.
func WithLoggerSensitiveParams(params ...string) func(*LoggerConfig) {
	return func(c *LoggerConfig) { c.SensitiveParams = params }
}

// WithLoggerSpanExtractor sets the SpanExtractor used to inject trace_id and
// span_id into log records. Use OTelSpanExtractor
// for OpenTelemetry integration.
func WithLoggerSpanExtractor(e SpanExtractor) func(*LoggerConfig) {
	return func(c *LoggerConfig) { c.SpanExtractor = e }
}

