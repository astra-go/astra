package log_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/astra-go/astra/log"
)

// ─── New / Config ─────────────────────────────────────────────────────────────

func TestNew_TextFormat_WritesToOutput(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Format: "text", Output: &buf})
	l.Info("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected 'hello' in output, got: %s", buf.String())
	}
}

func TestNew_JSONFormat_WritesToOutput(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Format: "json", Output: &buf})
	l.Info("json-msg")
	out := buf.String()
	if !strings.Contains(out, `"msg"`) {
		t.Errorf("expected JSON output, got: %s", out)
	}
}

func TestNew_MultiOutput_WritesToBoth(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	l := log.New(log.Config{
		Level:   log.LevelDebug,
		Outputs: []io.Writer{&buf1, &buf2},
	})
	l.Info("multi")
	if !strings.Contains(buf1.String(), "multi") {
		t.Error("buf1 should contain 'multi'")
	}
	if !strings.Contains(buf2.String(), "multi") {
		t.Error("buf2 should contain 'multi'")
	}
}

func TestNew_NilOutput_FallsBackToStdout(t *testing.T) {
	// Should not panic when Output is nil and Outputs is empty.
	l := log.New(log.Config{Level: log.LevelInfo})
	if l == nil {
		t.Error("expected non-nil logger")
	}
}

func TestNew_AddSource_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, AddSource: true, Output: &buf})
	l.Info("with-source")
	if !strings.Contains(buf.String(), "with-source") {
		t.Errorf("expected 'with-source' in output, got: %s", buf.String())
	}
}

// ─── Default / SetDefault / GetDefault ───────────────────────────────────────

func TestDefault_ReturnsNonNil(t *testing.T) {
	if log.Default() == nil {
		t.Error("Default() should not return nil")
	}
}

func TestSetDefault_GetDefault_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	newLogger := log.New(log.Config{Level: log.LevelDebug, Output: &buf})

	original := log.GetDefault()
	defer log.SetDefault(original) // restore after test

	log.SetDefault(newLogger)
	if log.GetDefault() != newLogger {
		t.Error("GetDefault should return the logger set by SetDefault")
	}
}

// ─── Package-level convenience functions ─────────────────────────────────────

func TestPackageLevel_Debug_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	original := log.GetDefault()
	defer log.SetDefault(original)
	log.SetDefault(l)

	log.Debug("dbg", "k", "v")
	if !strings.Contains(buf.String(), "dbg") {
		t.Errorf("expected 'dbg' in output, got: %s", buf.String())
	}
}

func TestPackageLevel_Info_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	original := log.GetDefault()
	defer log.SetDefault(original)
	log.SetDefault(l)

	log.Info("inf")
	if !strings.Contains(buf.String(), "inf") {
		t.Errorf("expected 'inf' in output, got: %s", buf.String())
	}
}

func TestPackageLevel_Warn_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	original := log.GetDefault()
	defer log.SetDefault(original)
	log.SetDefault(l)

	log.Warn("wrn")
	if !strings.Contains(buf.String(), "wrn") {
		t.Errorf("expected 'wrn' in output, got: %s", buf.String())
	}
}

func TestPackageLevel_Error_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	original := log.GetDefault()
	defer log.SetDefault(original)
	log.SetDefault(l)

	log.Error("err")
	if !strings.Contains(buf.String(), "err") {
		t.Errorf("expected 'err' in output, got: %s", buf.String())
	}
}

// ─── Logger.WithFields ────────────────────────────────────────────────────────

func TestWithFields_AddsKeyValue(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	child := l.WithFields("user", "alice")
	child.Info("action")
	out := buf.String()
	if !strings.Contains(out, "alice") {
		t.Errorf("expected 'alice' in output, got: %s", out)
	}
}

// ─── Context helpers ──────────────────────────────────────────────────────────

func TestWithRequestID_GetRequestID_RoundTrip(t *testing.T) {
	ctx := log.WithRequestID(context.Background(), "req-123")
	if got := log.GetRequestID(ctx); got != "req-123" {
		t.Errorf("GetRequestID = %q, want %q", got, "req-123")
	}
}

func TestGetRequestID_Missing_ReturnsEmpty(t *testing.T) {
	if got := log.GetRequestID(context.Background()); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestWithTraceID_AppearsInLog(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	ctx := log.WithTraceID(context.Background(), "trace-abc")
	child := l.WithContext(ctx)
	child.Info("traced")
	out := buf.String()
	if !strings.Contains(out, "trace-abc") {
		t.Errorf("expected trace_id in output, got: %s", out)
	}
}

func TestWithContext_RequestID_AppearsInLog(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	ctx := log.WithRequestID(context.Background(), "rid-999")
	child := l.WithContext(ctx)
	child.Info("req")
	out := buf.String()
	if !strings.Contains(out, "rid-999") {
		t.Errorf("expected request_id in output, got: %s", out)
	}
}

func TestWithContext_EmptyContext_ReturnsSameLogger(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	child := l.WithContext(context.Background())
	// Should return the same logger (no attrs added).
	if child == nil {
		t.Error("WithContext should not return nil")
	}
}

// ─── SetSpanExtractor ─────────────────────────────────────────────────────────

func TestSetSpanExtractor_ExtractsTraceAndSpan(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})

	log.SetSpanExtractor(func(_ context.Context) (string, string) {
		return "trace-xyz", "span-456"
	})
	defer log.SetSpanExtractor(nil) // clear after test

	child := l.WithContext(context.Background())
	child.Info("otel")
	out := buf.String()
	if !strings.Contains(out, "trace-xyz") {
		t.Errorf("expected trace_id in output, got: %s", out)
	}
	if !strings.Contains(out, "span-456") {
		t.Errorf("expected span_id in output, got: %s", out)
	}
}

func TestSetSpanExtractor_Nil_ClearsExtractor(t *testing.T) {
	log.SetSpanExtractor(func(_ context.Context) (string, string) {
		return "t", "s"
	})
	log.SetSpanExtractor(nil)

	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	ctx := log.WithTraceID(context.Background(), "manual-trace")
	child := l.WithContext(ctx)
	child.Info("after-clear")
	out := buf.String()
	// After clearing, manual trace_id should be used instead.
	if !strings.Contains(out, "manual-trace") {
		t.Errorf("expected manual trace_id after clearing extractor, got: %s", out)
	}
}

func TestSetSpanExtractor_EmptyTraceID_FallsBackToManual(t *testing.T) {
	log.SetSpanExtractor(func(_ context.Context) (string, string) {
		return "", "" // no active span
	})
	defer log.SetSpanExtractor(nil)

	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	ctx := log.WithTraceID(context.Background(), "fallback-trace")
	child := l.WithContext(ctx)
	child.Info("fallback")
	out := buf.String()
	if !strings.Contains(out, "fallback-trace") {
		t.Errorf("expected fallback trace_id, got: %s", out)
	}
}

// ─── Logger.Panic ─────────────────────────────────────────────────────────────

func TestLogger_Panic_PanicsWithMsg(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic")
		}
		if r != "panic-msg" {
			t.Errorf("expected panic value 'panic-msg', got %v", r)
		}
	}()
	l.Panic("panic-msg")
}

func TestPackageLevel_Panic_PanicsWithMsg(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(log.Config{Level: log.LevelDebug, Output: &buf})
	original := log.GetDefault()
	defer log.SetDefault(original)
	log.SetDefault(l)

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic")
		}
	}()
	log.Panic("pkg-panic")
}
