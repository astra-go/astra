package middleware

import (
	"log/slog"

	"github.com/astra-go/astra"
)

// Skipper determines whether a middleware should be skipped for the given request.
// Return true to skip, false to process normally.
type Skipper func(c *astra.Ctx) bool

// DefaultSkipper never skips — all requests are processed by the middleware.
func DefaultSkipper() Skipper {
	return func(c *astra.Ctx) bool { return false }
}

// SkipAll skips every request. Useful for disabling a middleware without
// removing it from the handler chain (e.g. feature-flag driven toggling).
func SkipAll() Skipper {
	return func(c *astra.Ctx) bool { return true }
}

// shouldSkip evaluates the skipper; returns false when skipper is nil.
func shouldSkip(skipper Skipper, c *astra.Ctx) bool {
	return skipper != nil && skipper(c)
}

// ErrorHandler is a handler invoked when a middleware rejects a request.
// Common uses: APIKey (401), Tenant (400), RateLimit exceeded (429).
//
// For middleware that needs the originating error (e.g. JWT), use a
// middleware-specific callback instead — the signature varies too much
// for a single type to cover well.
type ErrorHandler astra.HandlerFunc

// resolveLogger returns logger if non-nil, otherwise slog.Default().
func resolveLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
