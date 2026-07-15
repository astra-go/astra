package security

import (
	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

// Skipper is re-exported from the middleware package for backwards
// compatibility. Use middleware.Skipper in new code.
type Skipper = middleware.Skipper

// shouldSkip evaluates the skipper; returns false when skipper is nil.
func shouldSkip(skipper Skipper, c *astra.Ctx) bool {
	return skipper != nil && skipper(c)
}

// ErrorHandler is re-exported from the middleware package for backwards
// compatibility.  Use middleware.ErrorHandler in new code.
type ErrorHandler = middleware.ErrorHandler
