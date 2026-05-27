// Package log provides structured logging for Astra, built on Go's log/slog.
//
// # Quick start
//
//	log.SetDefault(log.New(log.Config{
//	    Level:  log.LevelDebug,
//	    Format: "json",
//	}))
//	log.Info("server started", "addr", ":8080")
//
// # Context-aware logging
//
// By default, only request_id and manually-set trace_id are extracted from
// context. To enable automatic OTel trace/span injection, register a
// SpanExtractor via SetSpanExtractor — the astra/otel sub-module provides one:
//
//	import astraotel "github.com/astra-go/astra/otel"
//	log.SetSpanExtractor(astraotel.SpanExtractor)
//
// # Multi-output
//
//	f, _ := os.Create("app.log")
//	log.New(log.Config{Outputs: []io.Writer{os.Stdout, f}})
package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
)

// SpanExtractor extracts trace_id and span_id strings from a context.
// Return empty strings when no active span is present.
type SpanExtractor func(ctx context.Context) (traceID, spanID string)

// spanExtractor is the package-level extractor; nil means OTel is not wired in.
var spanExtractor atomic.Pointer[SpanExtractor]

// SetSpanExtractor registers a function that extracts OTel trace/span IDs from
// a context. Call this once at startup, before handling any requests.
// Pass nil to clear a previously registered extractor.
func SetSpanExtractor(fn SpanExtractor) {
	if fn == nil {
		spanExtractor.Store(nil)
	} else {
		spanExtractor.Store(&fn)
	}
}

// Level aliases slog.Level so callers do not need to import log/slog directly.
type Level = slog.Level

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Config configures a Logger.
type Config struct {
	// Level is the minimum log level. Default: LevelInfo.
	Level Level
	// Format is "json" or "text". Default: "text".
	Format string
	// Output is the single writer target. Ignored when Outputs is non-empty.
	Output io.Writer
	// Outputs enables writing to multiple targets simultaneously.
	// When set, Output is ignored. Use io.Discard to silence a target.
	Outputs []io.Writer
	// AddSource adds the caller's file and line number to every record.
	AddSource bool
}

// DefaultConfig is the default logger configuration.
var DefaultConfig = Config{
	Level:  LevelInfo,
	Format: "text",
	Output: os.Stdout,
}

// ─── Logger ───────────────────────────────────────────────────────────────────

// Logger wraps *slog.Logger with context-aware helpers and convenience levels.
type Logger struct {
	*slog.Logger
}

// New creates a Logger from cfg.
func New(cfg Config) *Logger {
	out := resolveOutput(cfg)
	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
	}
	var h slog.Handler
	if cfg.Format == "json" {
		h = slog.NewJSONHandler(out, opts)
	} else {
		h = slog.NewTextHandler(out, opts)
	}
	return &Logger{Logger: slog.New(h)}
}

// resolveOutput returns the effective io.Writer for cfg.
func resolveOutput(cfg Config) io.Writer {
	if len(cfg.Outputs) > 0 {
		return io.MultiWriter(cfg.Outputs...)
	}
	if cfg.Output != nil {
		return cfg.Output
	}
	return os.Stdout
}

// Default returns a Logger using DefaultConfig.
func Default() *Logger { return New(DefaultConfig) }

// WithContext returns a child logger enriched with observability fields from ctx.
//
// Fields added (when available):
//   - trace_id  — from the active OTel span (preferred) or context key
//   - span_id   — from the active OTel span
//   - request_id — from context key set by WithRequestID
func (l *Logger) WithContext(ctx context.Context) *Logger {
	attrs := extractContextAttrs(ctx)
	if len(attrs) == 0 {
		return l
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return &Logger{Logger: l.Logger.With(args...)}
}

// WithFields returns a child logger with additional key-value pairs.
func (l *Logger) WithFields(fields ...any) *Logger {
	return &Logger{Logger: l.Logger.With(fields...)}
}

// Fatal logs at ERROR level then calls os.Exit(1).
// Use sparingly — prefer returning errors whenever possible.
func (l *Logger) Fatal(msg string, args ...any) {
	l.Logger.Error(msg, args...)
	os.Exit(1)
}

// Panic logs at ERROR level then panics with msg.
func (l *Logger) Panic(msg string, args ...any) {
	l.Logger.Error(msg, args...)
	panic(msg)
}

// ─── Context extraction ───────────────────────────────────────────────────────

// extractContextAttrs returns slog.Attrs carrying request/trace identifiers.
//
// Priority:
//  1. SpanExtractor (trace_id + span_id) — set when OTel is wired via SetSpanExtractor.
//  2. Manually stored trace_id (via WithTraceID).
//  3. Manually stored request_id (via WithRequestID) — always included.
func extractContextAttrs(ctx context.Context) []slog.Attr {
	var attrs []slog.Attr

	// request_id is always included if present — it is independent of tracing.
	if rid, ok := ctx.Value(contextKeyRequestID{}).(string); ok && rid != "" {
		attrs = append(attrs, slog.String("request_id", rid))
	}

	// OTel span IDs via injected extractor (avoids hard dep on otel/trace).
	if extPtr := spanExtractor.Load(); extPtr != nil {
		if traceID, spanID := (*extPtr)(ctx); traceID != "" {
			attrs = append(attrs,
				slog.String("trace_id", traceID),
				slog.String("span_id", spanID),
			)
			return attrs
		}
	}

	// Fallback: manually stored trace_id (no OTel span active).
	if tid, ok := ctx.Value(contextKeyTraceID{}).(string); ok && tid != "" {
		attrs = append(attrs, slog.String("trace_id", tid))
	}
	return attrs
}

// ─── Context keys ─────────────────────────────────────────────────────────────

type contextKeyRequestID struct{}
type contextKeyTraceID struct{}

// WithRequestID stores id in ctx under the request-ID context key.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyRequestID{}, id)
}

// WithTraceID stores id in ctx under the trace-ID context key.
// Prefer using the OTel SDK; this helper exists for environments without OTel.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyTraceID{}, id)
}

// GetRequestID retrieves the request ID stored by WithRequestID.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(contextKeyRequestID{}).(string)
	return id
}

// ─── Thread-safe global logger ────────────────────────────────────────────────

// std is the package-level default logger.
// atomic.Pointer gives safe, lock-free reads and writes.
var std atomic.Pointer[Logger]

func init() {
	l := Default()
	std.Store(l)
}

// SetDefault replaces the package-level default logger and also updates
// slog's global default so that log/slog calls pick up the new handler.
func SetDefault(l *Logger) {
	std.Store(l)
	slog.SetDefault(l.Logger)
}

// GetDefault returns the current package-level default logger.
func GetDefault() *Logger { return std.Load() }

// Package-level convenience functions — delegate to the current default logger.

// Debug logs at DEBUG level.
func Debug(msg string, args ...any) { std.Load().Debug(msg, args...) }

// Info logs at INFO level.
func Info(msg string, args ...any) { std.Load().Info(msg, args...) }

// Warn logs at WARN level.
func Warn(msg string, args ...any) { std.Load().Warn(msg, args...) }

// Error logs at ERROR level.
func Error(msg string, args ...any) { std.Load().Error(msg, args...) }

// Fatal logs at ERROR level and exits the process.
func Fatal(msg string, args ...any) { std.Load().Fatal(msg, args...) }

// Panic logs at ERROR level and panics.
func Panic(msg string, args ...any) { std.Load().Panic(msg, args...) }
