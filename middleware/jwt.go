// JWT middleware has moved to a dedicated sub-module to eliminate the
// golang-jwt dependency from the core middleware package.
//
// Migration:
//
//	// Before
//	import "github.com/astra-go/astra/middleware"
//	app.Use(middleware.JWT("my-secret"))
//	app.Use(middleware.JWTWithConfig(middleware.JWTConfig{...}))
//	claims := middleware.GetClaims(c)
//
//	// After
//	import sec "github.com/astra-go/astra/middleware/security"
//	app.Use(sec.JWT("my-secret"))
//	app.Use(sec.JWTWithConfig(sec.JWTConfig{...}))
//	claims := sec.GetClaims(c)
//
// The security sub-module is at:
//
//	go get github.com/astra-go/astra/middleware/security
package middleware
