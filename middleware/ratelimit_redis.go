// Ratelimit Redis middleware has moved to a dedicated sub-module.
//
// Migration:
//
//	// Before
//	import "github.com/astra-go/astra/middleware"
//
//	// After
//	import sec "github.com/astra-go/astra/middleware/security"
//
// The security sub-module is at:
//
//	go get github.com/astra-go/astra/middleware/security
package middleware
