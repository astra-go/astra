// Tracing middleware has moved to a dedicated sub-module to eliminate the
// OpenTelemetry dependency from the core middleware package.
//
// Migration:
//
//	// Before
//	import "github.com/astra-go/astra/middleware"
//	app.Use(middleware.Tracing())
//
//	// After
//	import obs "github.com/astra-go/astra/middleware/observability"
//	app.Use(obs.Tracing())
//
// The observability sub-module is at:
//
//	go get github.com/astra-go/astra/middleware/observability
package middleware
