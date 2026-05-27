// Package otel — slog handler that automatically injects OTel trace context.
package otel

import (
	"context"
	"log/slog"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// OTelSlogHandler wraps a base slog.Handler and automatically injects
// trace_id and span_id into every log record when an active sampled OTel
// span is present in the record's context.
//
// Usage — replace the global logger once, before handling any requests:
//
//	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
//	slog.SetDefault(slog.New(otel.OTelSlogHandler(base)))
//
// Any application code that calls slog.InfoContext(ctx, ...) will automatically
// include trace_id and span_id when ctx carries an active span.
// Calls without a context (slog.Info) are forwarded unchanged.
func OTelSlogHandler(base slog.Handler) slog.Handler {
	return &traceHandler{base: base}
}

type traceHandler struct {
	base slog.Handler
}

// Enabled delegates to the base handler so level filtering is unchanged.
func (h *traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

// Handle injects trace_id and span_id attributes when the context carries an
// active sampled span, then delegates to the base handler.
func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if sc := oteltrace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
			r.AddAttrs(
				slog.String("trace_id", sc.TraceID().String()),
				slog.String("span_id", sc.SpanID().String()),
			)
		}
	}
	return h.base.Handle(ctx, r)
}

// WithAttrs returns a new handler whose base has the given attributes pre-set.
func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{base: h.base.WithAttrs(attrs)}
}

// WithGroup returns a new handler with the given group name set on the base.
func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{base: h.base.WithGroup(name)}
}

// SpanExtractor is a log.SpanExtractor implementation that reads trace_id and
// span_id from the active OTel span in ctx. Register it once at startup:
//
//	import (
//	    astralog  "github.com/astra-go/astra/log"
//	    astraotel "github.com/astra-go/astra/otel"
//	)
//
//	astralog.SetSpanExtractor(astraotel.SpanExtractor)
func SpanExtractor(ctx context.Context) (traceID, spanID string) {
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return "", ""
	}
	return sc.TraceID().String(), sc.SpanID().String()
}
