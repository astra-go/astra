// Package otel — see app.go for the package overview.
//
// This file keeps the lightweight log-correlation helpers that read the active
// span out of a context. The heavier provider wiring lives in app.go.
package otel

import (
	"context"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// TraceIDFromContext returns the trace ID of the active span in ctx as a
// 32-character lowercase hex string, or "" if there is no active span.
//
// Inject this into log records to correlate logs with distributed traces:
//
//	slog.Info("payment processed",
//	    slog.String("trace_id", otel.TraceIDFromContext(ctx)),
//	    slog.String("span_id",  otel.SpanIDFromContext(ctx)),
//	)
func TraceIDFromContext(ctx context.Context) string {
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

// SpanIDFromContext returns the span ID of the active span in ctx as a
// 16-character lowercase hex string, or "" if there is no active span.
func SpanIDFromContext(ctx context.Context) string {
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	if sc.HasSpanID() {
		return sc.SpanID().String()
	}
	return ""
}

// SpanContext returns the full SpanContext for the active span in ctx.
// Prefer TraceIDFromContext / SpanIDFromContext for simple log injection.
func SpanContext(ctx context.Context) oteltrace.SpanContext {
	return oteltrace.SpanFromContext(ctx).SpanContext()
}
