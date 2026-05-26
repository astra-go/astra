// Audit middleware has moved to a dedicated sub-module.
//
// Migration:
//
//	// Before
//	import "github.com/astra-go/astra/middleware"
//	app.Use(middleware.Audit(middleware.AuditConfig{...}))
//
//	// After
//	import obs "github.com/astra-go/astra/middleware/observability"
//	app.Use(obs.Audit(obs.AuditConfig{...}))
//
// The observability sub-module is at:
//
//	go get github.com/astra-go/astra/middleware/observability
package middleware
