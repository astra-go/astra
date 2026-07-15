// Package auth provides identity and access-control primitives for Astra
// applications. It is a standalone Go module shipped independently from the
// core framework so that users who only need HTTP-layer protection (JWT,
// rate limiting, API keys) are not required to pull in Casbin or OAuth2
// dependencies.
//
// # Sub-packages
//
//   - rbac    — Casbin-based role-based access control middleware
//   - oauth2  — OAuth2 / OIDC authorization-code flow with PKCE
//
// # Relationship with middleware/security
//
// auth and middleware/security address different layers:
//
//	auth                  → identity flows (login, logout, token refresh,
//	                         OAuth2 authorization code, RBAC policy enforcement)
//	middleware/security    → HTTP-layer protection (JWT verification, rate
//	                         limiting, IP filtering, API key auth, tenant
//	                         isolation)
//
// A typical app uses both:
//
//	auth/rbac   for "can user X take action Y on resource Z?"
//	security/   for "is this request authenticated and within rate limits?"
//
// Import this module when your application needs fine-grained access control
// (Casbin policies) or OAuth2/OIDC client integration.
package auth
