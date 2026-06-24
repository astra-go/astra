// Package middleware provides HTTP middleware for the Astra web framework.
//
// BodyLimit limits the maximum size of incoming request bodies. When a request
// body exceeds the configured limit, the handler reading the body receives a
// *http.MaxBytesError (Go ≥ 1.19), and the middleware returns an HTTP 413
// (Payload Too Large) response before the rest of the handler chain processes.
//
// # Usage
//
//	app.Use(middleware.BodyLimit(1 << 20)) // 1 MiB
//
// The limit is applied per-request. The middleware wraps the request body in
// http.MaxBytesReader so that handlers reading via c.Request().Body,
// c.Body(), or c.FormValues() are automatically limited.

package middleware

import (
	"net/http"

	"github.com/astra-go/astra"
)

// BodyLimit returns a middleware that limits the size of incoming request
// bodies to at most maxBytes bytes.
//
// When the handler (or the framework body parser) reads past maxBytes, the
// read returns *http.MaxBytesError, the response code is set to 413, and
// no further processing occurs.
//
// Use BodyLimitWithConfig for a configurable status code, message, and skipper.
func BodyLimit(maxBytes int64) astra.HandlerFunc {
	return BodyLimitWithConfig(BodyLimitConfig{MaxBytes: maxBytes})
}

// BodyLimitConfig configures the BodyLimit middleware.
type BodyLimitConfig struct {
	// MaxBytes is the maximum request body size in bytes (required).
	// When 0 or negative, BodyLimit falls back to 1 MiB.
	MaxBytes int64

	// StatusCode is the HTTP status code for oversized bodies.
	// Default: 413 Payload Too Large.
	StatusCode int

	// Message is the response body for oversized bodies.
	// Default: "request body too large".
	Message string

	// Skipper, when set, skips body-size limiting for matching requests.
	Skipper func(c *astra.Ctx) bool
}

// BodyLimitWithConfig returns a body-limiter middleware using the supplied
// configuration. Unlike the one-arg BodyLimit, this variant returns an
// immediate 413 response when the limit is exceeded.
func BodyLimitWithConfig(cfg BodyLimitConfig) astra.HandlerFunc {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 1 << 20 // 1 MiB
	}
	if cfg.StatusCode == 0 {
		cfg.StatusCode = http.StatusRequestEntityTooLarge
	}
	if cfg.Message == "" {
		cfg.Message = "request body too large"
	}

	return func(c *astra.Ctx) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			return c.Next()
		}

		req := c.Request()
		req.Body = http.MaxBytesReader(c.Writer(), req.Body, cfg.MaxBytes)
		c.SetRequest(req)

		c.Next()

		// Check if the limit was exceeded during processing.
		// net/http reads the MaxBytesReader eagerly after the handler returns,
		// setting Content-Length to the actual value and the Connection to close.
		// Our path: the handler already got a *http.MaxBytesError from a read,
		// which triggered an AppError (or HTTPError). If the writer hasn't been
		// written to yet we overwrite with our 413.
		if !c.Writer().Written() {
			c.Writer().WriteHeader(cfg.StatusCode)
			_, _ = c.Writer().Write([]byte(cfg.Message))
			c.Abort()
		}
		return nil
	}
}
