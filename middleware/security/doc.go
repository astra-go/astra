// Package security provides HTTP security middleware for the Astra framework.
//
// Package layout (logical groups by file prefix):
//
//	auth & JWT
//	  jwt.go              — core JWT verification middleware
//	  jwt_generate.go     — JWT generation helpers
//	  jwt_revoke.go       — JWT revocation list (in-memory + Redis)
//	  jwt_cache.go        — JWT signature cache (signing-key → public-key)
//	  jwt_cache_redis.go  — Redis-backed JWT cache
//	  jwt_cache_multilevel.go — L1 (in-process) + L2 (Redis) cache
//
//	rate limiting
//	  ratelimit.go        — core token-bucket / sliding-window limiter
//	  ratelimit_redis.go  — Redis-backed distributed rate limiter
//	  ratelimit_advanced.go — multi-dimension rate limits
//
//	multi-tenancy
//	  tenant.go           — tenant ID extraction from token / header
//	  tenant_quota.go     — per-tenant resource quota enforcement
//	  tenant_quota_json.go — JSON-file-backed quota store
//	  tenant_metrics.go   — per-tenant metrics labels
//
//	network security
//	  ipfilter.go         — allowlist / blocklist by CIDR or IP
//	  ipblacklist.go      — dynamic IP blacklist with expiry
//	  canary.go           — canary / percentage traffic splitting
//
//	cryptography & signing
//	  signature.go        — HMAC / RSA request signature verification
//	  apikey.go           — API key authentication
//	  accountlock.go      — account lockout after failed attempts
//
//	miscellaneous
//	  longpoll.go         — long-polling response holder
//	  pprof.go            — pprof profiling endpoints
//	  options.go          — shared middleware options
//
// Relationship with auth package:
//
//	The auth/ subpackage is a standalone authentication system (token issuance,
//	token introspection, account management). middleware/security/jwt.go is the
//	HTTP middleware that verifies incoming JWTs. Use auth for login/logout flows;
//	use middleware/security/jwt.go to protect routes.
package security
