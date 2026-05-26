// Metrics middleware has moved to a dedicated sub-module to eliminate the
// Prometheus and OpenTelemetry dependencies from the core middleware package.
//
// Migration:
//
//	// Before
//	import "github.com/astra-go/astra/middleware"
//	app.Use(middleware.Metrics())
//	app.GET("/metrics", middleware.MetricsHandler())
//
//	// After
//	import obs "github.com/astra-go/astra/middleware/observability"
//	app.Use(obs.Metrics())
//	app.GET("/metrics", obs.MetricsHandler())
//
// The observability sub-module is at:
//
//	go get github.com/astra-go/astra/middleware/observability
package middleware
