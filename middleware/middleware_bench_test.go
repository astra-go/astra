// Package middleware_test — micro-benchmarks for individual Astra middleware.
//
// Each benchmark wraps a single middleware into a minimal App and drives it
// via httptest.ResponseRecorder.  This isolates the per-middleware overhead
// from routing and response-writing costs measured in the root bench_test.go.
//
// Run all middleware benchmarks:
//
//	go test -bench=. -benchmem -count=3 ./middleware/
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/golang-jwt/jwt/v5"
)

// mwBenchSink prevents dead-code elimination.
var mwBenchSink any

// nopApp creates an App with an optional list of middleware and a single GET /
// handler that returns NoContent(200).
func nopApp(mws ...astra.MiddlewareFunc) *astra.App {
	app := astra.New()
	app.Use(mws...)
	app.GET("/", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })
	return app
}

// ─── CORS ─────────────────────────────────────────────────────────────────────

// BenchmarkCORS_Passthrough measures the CORS middleware cost for a regular
// same-origin GET — the common case where no CORS headers need to be added.
func BenchmarkCORS_Passthrough(b *testing.B) {
	app := nopApp(middleware.CORS())
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// BenchmarkCORS_CrossOrigin measures the cost when the request carries an
// Origin header — the middleware must compare against AllowOrigins, set
// Access-Control-Allow-Origin, and potentially add Vary.
func BenchmarkCORS_CrossOrigin(b *testing.B) {
	app := nopApp(middleware.CORS())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// BenchmarkCORS_Preflight measures the OPTIONS preflight path which constructs
// and returns the full allow-list response without calling the actual handler.
func BenchmarkCORS_Preflight(b *testing.B) {
	app := nopApp(middleware.CORS())
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// ─── Recovery ─────────────────────────────────────────────────────────────────

// BenchmarkRecovery_NoPanic measures the Recovery middleware overhead on the
// happy path (no panic occurs).  This is the hot path for every production
// request and should add near-zero overhead.
func BenchmarkRecovery_NoPanic(b *testing.B) {
	app := nopApp(middleware.Recovery())
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// BenchmarkRecovery_Panic measures the full panic → recover → 500 response path.
// This is deliberately the cold path (rare in production) but important to
// ensure the recovery itself does not introduce unbounded latency.
func BenchmarkRecovery_Panic(b *testing.B) {
	app := astra.New()
	app.Use(middleware.RecoveryWithConfig(middleware.RecoveryConfig{
		PrintStack: false, // suppress stack trace output during benchmarks
	}))
	app.GET("/", func(c *astra.Ctx) error {
		panic("benchmark panic")
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// ─── JWT ──────────────────────────────────────────────────────────────────────

const benchJWTSecret = "super-secret-bench-key-32-bytes!!"

// signBenchToken creates a signed HS256 token valid for 1 hour.
// Called once in benchmark setup — the goal is to benchmark verification, not signing.
func signBenchToken(b *testing.B) string {
	b.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "bench-user",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	signed, err := token.SignedString([]byte(benchJWTSecret))
	if err != nil {
		b.Fatal("sign token:", err)
	}
	return "Bearer " + signed
}

// BenchmarkJWT_ValidToken measures HMAC-SHA256 token parsing and claims
// extraction on a valid, non-expired token — the production hot path.
func BenchmarkJWT_ValidToken(b *testing.B) {
	authHeader := signBenchToken(b)

	app := nopApp(sec.JWT(benchJWTSecret))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", authHeader)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// BenchmarkJWT_MissingToken measures the early-exit path when no Authorization
// header is present.  The middleware should short-circuit with 401 before any
// crypto work is done.
func BenchmarkJWT_MissingToken(b *testing.B) {
	app := nopApp(sec.JWT(benchJWTSecret))
	req := httptest.NewRequest(http.MethodGet, "/", nil) // no Authorization header

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// BenchmarkJWT_InvalidSignature measures the cost of parsing a token whose
// HMAC signature does not match — exercises the full parse + verify path.
func BenchmarkJWT_InvalidSignature(b *testing.B) {
	// Sign with a different key so verification fails.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "attacker",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	badToken, _ := token.SignedString([]byte("wrong-key"))

	app := nopApp(sec.JWT(benchJWTSecret))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+badToken)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// BenchmarkJWT_CacheHit measures the JWT middleware with a warm LRU cache.
// The same token is presented on every iteration so after the first request
// all subsequent lookups are pure map reads — no crypto, no JSON decoding.
// Expected: ≈5 allocs/op (httptest overhead only), down from ~64 allocs/op.
func BenchmarkJWT_CacheHit(b *testing.B) {
	authHeader := signBenchToken(b)
	app := nopApp(sec.JWTWithConfig(sec.JWTConfig{
		Secret: sec.NewSecretString(benchJWTSecret),
		CacheSize: 512,
	}))
	warmReq := httptest.NewRequest(http.MethodGet, "/", nil)
	warmReq.Header.Set("Authorization", authHeader)
	app.ServeHTTP(httptest.NewRecorder(), warmReq) // pre-warm

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", authHeader)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}

// BenchmarkJWT_CacheMiss measures the JWT middleware with caching enabled but
// always missing — the token is pre-parsed once but CacheSize is 0 on the inner
// app so every request goes through the full crypto + JSON path with a pooled
// parser and pooled MapClaims. Compare against BenchmarkJWT_ValidToken (no pool)
// to isolate the parser-pool and MapClaims-pool benefit.
func BenchmarkJWT_PooledParser(b *testing.B) {
	authHeader := signBenchToken(b)
	// CacheSize intentionally 0: only pooled parser + pooled MapClaims, no cache.
	app := nopApp(sec.JWTWithConfig(sec.JWTConfig{
		Secret: sec.NewSecretString(benchJWTSecret),
		CacheSize: 0,
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", authHeader)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		mwBenchSink = w.Code
	}
}
