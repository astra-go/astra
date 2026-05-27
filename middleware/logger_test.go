package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/astra-go/astra/testutil"
)

// bufHandler is a minimal slog.Handler that writes structured key=value to a buffer.
type bufHandler struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

func newBufLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), buf
}

func TestLogger_PassesRequestThrough(t *testing.T) {
	app := testutil.NewTestApp()
	logger, _ := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Logger: logger,
	}))
	app.GET("/ping", func(c *astra.Ctx) error { return c.String(http.StatusOK, "pong") })
	s := testutil.NewServer(t, app)

	s.GET("/ping").AssertStatus(http.StatusOK).AssertBodyContains("pong")
}

func TestLogger_SkipsConfiguredPaths(t *testing.T) {
	app := testutil.NewTestApp()
	logger, buf := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Logger:    logger,
		SkipPaths: []string{"/health"},
	}))
	app.GET("/health", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	app.GET("/info", func(c *astra.Ctx) error { return c.String(http.StatusOK, "info") })
	s := testutil.NewServer(t, app)

	s.GET("/health")
	healthLog := buf.String()

	s.GET("/info")
	infoLog := buf.String()

	if strings.Contains(healthLog, "/health") {
		t.Error("Logger should not log /health (in SkipPaths)")
	}
	if !strings.Contains(infoLog, "/info") {
		t.Error("Logger should log /info (not in SkipPaths)")
	}
}

func TestLogger_SanitizesQueryParams(t *testing.T) {
	app := testutil.NewTestApp()
	logger, buf := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Logger:          logger,
		SensitiveParams: []string{"token"},
	}))
	app.GET("/search", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/search?q=hello&token=supersecret")

	logged := buf.String()
	if strings.Contains(logged, "supersecret") {
		t.Errorf("sensitive param 'token' value should be redacted in log, got: %s", logged)
	}
	if !strings.Contains(logged, "REDACTED") {
		t.Errorf("expected REDACTED in log, got: %s", logged)
	}
}

func TestLogger_LogsStatusAndMethod(t *testing.T) {
	app := testutil.NewTestApp()
	logger, buf := newBufLogger()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{Logger: logger}))
	app.GET("/data", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/data")

	logged := buf.String()
	if !strings.Contains(logged, "GET") {
		t.Errorf("expected method GET in log, got: %s", logged)
	}
	if !strings.Contains(logged, "/data") {
		t.Errorf("expected path /data in log, got: %s", logged)
	}
}
