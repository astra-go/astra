// Package benchmarks — full-stack integration benchmarks for the Astra framework.
//
// Unlike the per-package micro-benchmarks (bench_test.go, middleware/middleware_bench_test.go,
// netengine/engine_bench_test.go), this suite exercises the COMPLETE request
// pipeline via httptest.ResponseRecorder:
//
//	HTTP request → App.ServeHTTP → router → global middleware → route middleware
//	→ handler (JSON, string, or no-content) → response writer
//
// These numbers reflect the wall-clock cost a production app pays per request
// and are the canonical figures to compare against Gin, Echo, Chi, etc.
//
// Run the full integration suite:
//
//	go test -bench=. -benchmem -count=5 -benchtime=3s ./benchmarks/
//
// Compare with a baseline (e.g. after an optimisation PR):
//
//	go test -bench=. -benchmem -count=5 ./benchmarks/ | tee new.txt
//	benchstat baseline.txt new.txt
package benchmarks_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/golang-jwt/jwt/v5"
)

// suiteSink prevents dead-code elimination.
var suiteSink any

// ─── Shared fixtures ──────────────────────────────────────────────────────────

// apiUser is a representative API response payload used across multiple benchmarks.
type apiUser struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Role      string   `json:"role"`
	Active    bool     `json:"active"`
	Score     float64  `json:"score"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

var fixtureUser = apiUser{
	ID: 1, Name: "Alice", Email: "alice@example.com",
	Role: "admin", Active: true, Score: 99.5,
	Tags: []string{"go", "backend", "platform"}, CreatedAt: "2024-01-01T00:00:00Z",
}

// ─── Baseline ────────────────────────────────────────────────────────────────

// BenchmarkIntegration_Baseline is the absolute floor: no middleware, static
// route, NoContent response.  Overhead above this is framework routing cost.
func BenchmarkIntegration_Baseline(b *testing.B) {
	app := astra.New()
	app.GET("/ping", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

// ─── Route types ─────────────────────────────────────────────────────────────

// BenchmarkIntegration_StaticRoute_JSON — most common API pattern: static path
// + JSON response body.  Used as the primary number for framework comparisons.
func BenchmarkIntegration_StaticRoute_JSON(b *testing.B) {
	app := astra.New()
	app.GET("/api/health", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, fixtureUser)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

// BenchmarkIntegration_ParamRoute_JSON — parameterised path + JSON response.
// Models a typical GET /users/:id endpoint.
func BenchmarkIntegration_ParamRoute_JSON(b *testing.B) {
	app := astra.New()
	app.GET("/users/:id", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, fixtureUser)
	})
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

// BenchmarkIntegration_POST_BindJSON_Response — POST with JSON request body
// parsing and JSON response.  Models a mutation endpoint (create/update).
func BenchmarkIntegration_POST_BindJSON_Response(b *testing.B) {
	type createReq struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	body, _ := json.Marshal(createReq{Name: "Bob", Email: "bob@example.com", Role: "user"})
	bodyStr := string(body)

	app := astra.New()
	app.POST("/users", func(c *astra.Ctx) error {
		var req createReq
		if err := c.ShouldBindJSON(&req); err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, fixtureUser)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/users",
			strings.NewReader(bodyStr))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

// BenchmarkIntegration_NotFound measures the cost of an unmatched route.
// This path hits: router lookup miss → notFoundChain (pre-allocated) →
// defaultNotFoundHandler → defaultErrorHandler fast path → pre-built body write.
// Target: ≈ baseline allocs (4).
func BenchmarkIntegration_NotFound(b *testing.B) {
	app := astra.New()
	app.GET("/ping", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

// ─── Middleware stacks ────────────────────────────────────────────────────────

// BenchmarkIntegration_Middleware3_JSON measures a realistic lightweight stack:
// RequestID + Recovery + CORS wrapping a JSON endpoint.
// These three middleware appear in almost every production Astra deployment.
func BenchmarkIntegration_Middleware3_JSON(b *testing.B) {
	app := astra.New()
	app.Use(middleware.RequestID())
	app.Use(middleware.Recovery())
	app.Use(middleware.CORS())
	app.GET("/users/:id", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, fixtureUser)
	})
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

// BenchmarkIntegration_Middleware5_JWT_JSON exercises the typical secure API
// stack: RequestID + Recovery + CORS + JWT + (nop) + JSON response.
// JWT is the most expensive middleware; this benchmark reveals its real weight
// in a full chain.
func BenchmarkIntegration_Middleware5_JWT_JSON(b *testing.B) {
	const secret = "integration-bench-secret-32bytes"
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "bench-user",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString([]byte(secret))

	app := astra.New()
	app.Use(middleware.RequestID())
	app.Use(middleware.Recovery())
	app.Use(middleware.CORS())
	app.Use(middleware.JWT(secret))
	app.Use(func(c *astra.Ctx) error { // 5th: lightweight audit stub
		c.Set("audit_ts", time.Now().UnixNano())
		c.Next()
		return nil
	})
	app.GET("/users/:id", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, fixtureUser)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

// ─── Grouped routes ───────────────────────────────────────────────────────────

// BenchmarkIntegration_GroupedAPI simulates a multi-group REST API
// (/v1/users, /v1/orders, /v2/users) with per-group middleware and measures
// the overhead of group inheritance in the handler chain.
func BenchmarkIntegration_GroupedAPI(b *testing.B) {
	app := astra.New()
	app.Use(middleware.Recovery())

	v1 := app.Group("/v1", middleware.CORS())
	v1.GET("/users/:id", func(c *astra.Ctx) error { return c.JSON(200, fixtureUser) })
	v1.GET("/orders/:id", func(c *astra.Ctx) error { return c.JSON(200, fixtureUser) })

	v2 := app.Group("/v2", middleware.CORS())
	v2.GET("/users/:id", func(c *astra.Ctx) error { return c.JSON(200, fixtureUser) })

	req := httptest.NewRequest(http.MethodGet, "/v1/users/42", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

// ─── Parallel throughput ──────────────────────────────────────────────────────

// BenchmarkIntegration_Parallel_Static measures concurrent end-to-end
// throughput on a zero-middleware static route.
//
// NOTE: allocs/op here (208 B / 4 allocs) come entirely from
// httptest.NewRecorder() in the hot loop, not from the App's sync.Pool.
// The Pool itself is allocation-free (Ctx is recycled) and scales linearly
// up to GOMAXPROCS with no contention plateau.  To measure Pool behaviour
// in isolation, see BenchmarkPool_SharedRecorder in the root package.
func BenchmarkIntegration_Parallel_Static(b *testing.B) {
	app := astra.New()
	app.GET("/ping", func(c *astra.Ctx) error { return c.NoContent(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			suiteSink = w.Code
		}
	})
}

// BenchmarkIntegration_Parallel_Static_WarmPool is the pool-isolated variant:
// a single httptest.ResponseRecorder is shared across all goroutines (valid
// because the handler writes no body).  allocs/op drop to 0, revealing the
// true Pool Get→reset→Handle→Put latency.
func BenchmarkIntegration_Parallel_Static_WarmPool(b *testing.B) {
	app := astra.New()
	app.GET("/ping", func(c *astra.Ctx) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			app.ServeHTTP(w, req)
		}
	})
}

// BenchmarkIntegration_Parallel_JSON_3MW measures parallel JSON throughput
// with three middleware — the minimum recommended production stack.
func BenchmarkIntegration_Parallel_JSON_3MW(b *testing.B) {
	app := astra.New()
	app.Use(middleware.RequestID())
	app.Use(middleware.Recovery())
	app.Use(middleware.CORS())
	app.GET("/users/:id", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, fixtureUser)
	})
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			suiteSink = w.Body.Len()
		}
	})
}

// ─── Large payload ────────────────────────────────────────────────────────────

// BenchmarkIntegration_LargeList_JSON benchmarks encoding a slice of 200 user
// records (~30 KB).  This exercises the jsonBufPool buffer sizing and the
// Content-Length calculation for bulk/list endpoints.
func BenchmarkIntegration_LargeList_JSON(b *testing.B) {
	users := make([]apiUser, 200)
	for i := range users {
		users[i] = fixtureUser
		users[i].ID = i
	}
	app := astra.New()
	app.GET("/users", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, users)
	})
	req := httptest.NewRequest(http.MethodGet, "/users", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

// BenchmarkIntegration_LargeList_JSONStream is the JSONStream counterpart of
// BenchmarkIntegration_LargeList_JSON.  JSONStream encodes directly into the
// ResponseWriter, eliminating the intermediate pooled buffer copy.
func BenchmarkIntegration_LargeList_JSONStream(b *testing.B) {
	users := make([]apiUser, 200)
	for i := range users {
		users[i] = fixtureUser
		users[i].ID = i
	}
	app := astra.New()
	app.GET("/users/stream", func(c *astra.Ctx) error {
		return c.JSONStream(http.StatusOK, users)
	})
	req := httptest.NewRequest(http.MethodGet, "/users/stream", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}
