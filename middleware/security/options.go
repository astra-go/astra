package security

import (
	"github.com/astra-go/astra"
)

// Skipper determines whether a middleware should be skipped for the given request.
// Return true to skip, false to process normally.
type Skipper func(c *astra.Ctx) bool

// shouldSkip evaluates the skipper; returns false when skipper is nil.
func shouldSkip(skipper Skipper, c *astra.Ctx) bool {
	return skipper != nil && skipper(c)
}

// ErrorHandler is a handler invoked when a middleware rejects a request.
// Common uses: APIKey (401), Tenant (400), RateLimit exceeded (429).
type ErrorHandler astra.HandlerFunc
