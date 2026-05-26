// Package astra_test — micro-benchmarks for the core hot paths:
// router lookup, middleware chain execution, context response writing,
// and the full App.ServeHTTP round-trip.
//
// Run all core benchmarks:
//
//	go test -bench=. -benchmem -count=3 -benchtime=2s .
//
// Run a single group, e.g. router only:
//
//	go test -bench=BenchmarkRouter -benchmem -count=5 .
package astra_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/astra-go/astra"
)

// benchSink is a package-level sink used to prevent dead-code elimination of
// benchmark results.  Assign output values here at the end of each loop body.
var benchSink any

// ─── Router benchmarks ────────────────────────────────────────────────────────

// BenchmarkRouter_Static measures radix-tree lookup for a single plain static route.
func BenchmarkRouter_Static(b *testing.B) {
	app := astra.New()
	app.GET("/ping", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_Static_REST measures lookup in a tree where each top-level
// resource starts with a unique first byte — the typical REST API layout.
// This exercises the O(1) first-byte dispatch fast path introduced to replace
// the O(n) linear scan.  Hit target: the last-registered sibling (/webhooks).
func BenchmarkRouter_Static_REST(b *testing.B) {
	app := astra.New()
	for _, res := range []string{
		"users", "orders", "products", "auth", "settings",
		"metrics", "health", "webhooks", "billing", "notifications",
		"subscriptions", "payments", "invoices", "reports", "exports",
		"imports", "tags", "labels", "comments", "attachments",
		"media", "files", "queue", "jobs", "events",
	} {
		app.GET("/"+res, nopHandler)
	}
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}


// Exercises the linear static-child scan at depth 2 (worst case: last sibling).
func BenchmarkRouter_Static_100(b *testing.B) {
	app := astra.New()
	for i := 0; i < 100; i++ {
		path := "/route/" + strconv.Itoa(i)
		app.GET(path, nopHandler)
	}
	req := httptest.NewRequest(http.MethodGet, "/route/99", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_Param measures route matching with a single :param segment.
func BenchmarkRouter_Param(b *testing.B) {
	app := astra.New()
	app.GET("/users/:id", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/users/12345", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_Param_Deep measures a three-segment parameterized route.
// Quantifies the cost of extracting multiple path parameters per request.
func BenchmarkRouter_Param_Deep(b *testing.B) {
	app := astra.New()
	app.GET("/orgs/:org/repos/:repo/issues/:num", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/orgs/acme/repos/api/issues/42", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_Regex measures regex-constrained parameter matching.
// [0-9]+ is in wellKnownMatchers: the fast byte-scan path is used instead
// of the regexp engine, so this should approach BenchmarkRouter_Param cost.
func BenchmarkRouter_Regex(b *testing.B) {
	app := astra.New()
	app.GET("/items/{id:[0-9]+}", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_Regex_FastPath_Parallel is the parallel counterpart of
// BenchmarkRouter_Regex.  It exercises the hot path under concurrent load:
// [0-9]+ uses the fast byte-scan matcher (no regexp engine, no pool contention).
func BenchmarkRouter_Regex_FastPath_Parallel(b *testing.B) {
	app := astra.New()
	app.GET("/items/{id:[0-9]+}", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			benchSink = w.Code
		}
	})
}

// BenchmarkRouter_Regex_Custom_Parallel exercises a pattern NOT in
// wellKnownMatchers, so it falls back to the regexp engine.  Compare with
// BenchmarkRouter_Regex_FastPath_Parallel to quantify the fast-path gain.
func BenchmarkRouter_Regex_Custom_Parallel(b *testing.B) {
	app := astra.New()
	// A non-trivial pattern with a quantifier range — not in wellKnownMatchers.
	app.GET("/items/{id:[0-9]{1,10}}", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			benchSink = w.Code
		}
	})
}

// BenchmarkRouter_Wildcard measures the catch-all (*path) route match.
func BenchmarkRouter_Wildcard(b *testing.B) {
	app := astra.New()
	app.GET("/static/*filepath", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/static/css/app.min.css", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_NotFound measures the miss path — traverses the full tree
// without finding a match and returns 404.
func BenchmarkRouter_NotFound(b *testing.B) {
	app := astra.New()
	for i := 0; i < 20; i++ {
		app.GET("/route/"+strconv.Itoa(i), nopHandler)
	}
	req := httptest.NewRequest(http.MethodGet, "/no-such-route/here", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// ─── Middleware chain benchmarks ──────────────────────────────────────────────

// benchMiddlewareChain builds an app with n pass-through middleware and returns
// it along with a ready-to-reuse request.  The handler returns NoContent(200).
func benchMiddlewareChain(b *testing.B, n int) (*astra.App, *http.Request) {
	b.Helper()
	app := astra.New()
	for i := 0; i < n; i++ {
		app.Use(func(c *astra.Ctx) error {
			c.Next()
			return nil
		})
	}
	app.GET("/chain", nopHandler)
	return app, httptest.NewRequest(http.MethodGet, "/chain", nil)
}

func BenchmarkMiddlewareChain_0(b *testing.B) {
	app, req := benchMiddlewareChain(b, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

func BenchmarkMiddlewareChain_1(b *testing.B) {
	app, req := benchMiddlewareChain(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

func BenchmarkMiddlewareChain_3(b *testing.B) {
	app, req := benchMiddlewareChain(b, 3)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

func BenchmarkMiddlewareChain_5(b *testing.B) {
	app, req := benchMiddlewareChain(b, 5)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

func BenchmarkMiddlewareChain_10(b *testing.B) {
	app, req := benchMiddlewareChain(b, 10)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkMiddlewareChain_Abort measures a chain that aborts after the first
// middleware.  This is the fast path for auth rejections.
func BenchmarkMiddlewareChain_Abort(b *testing.B) {
	app := astra.New()
	app.Use(func(c *astra.Ctx) error {
		c.AbortWithStatus(http.StatusUnauthorized)
		return nil
	})
	// Five more middleware that must NOT execute:
	for i := 0; i < 5; i++ {
		app.Use(func(c *astra.Ctx) error {
			c.Next()
			return nil
		})
	}
	app.GET("/secret", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/secret", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// ─── Context response benchmarks ─────────────────────────────────────────────

// smallPayload / mediumPayload / largePayload are static JSON objects that
// represent realistic API response sizes.
type smallPayload struct {
	OK bool `json:"ok"`
}

type mediumPayload struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	Role    string   `json:"role"`
	Active  bool     `json:"active"`
	Score   float64  `json:"score"`
	Tags    []string `json:"tags"`
	Created string   `json:"created"`
}

// BenchmarkContext_JSON_Small measures JSON encoding + response writing for a
// ~20-byte object.  This is the most common response in health-check endpoints.
func BenchmarkContext_JSON_Small(b *testing.B) {
	app := astra.New()
	payload := smallPayload{OK: true}
	app.GET("/small", func(c *astra.Ctx) error {
		return c.JSON(200, payload)
	})
	req := httptest.NewRequest(http.MethodGet, "/small", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Body.Len()
	}
}

// BenchmarkContext_JSON_Medium measures JSON encoding for a realistic user record.
func BenchmarkContext_JSON_Medium(b *testing.B) {
	app := astra.New()
	payload := mediumPayload{
		ID: 42, Name: "Alice", Email: "alice@example.com",
		Role: "admin", Active: true, Score: 98.6,
		Tags: []string{"go", "backend", "api"}, Created: "2024-01-15T10:00:00Z",
	}
	app.GET("/medium", func(c *astra.Ctx) error {
		return c.JSON(200, payload)
	})
	req := httptest.NewRequest(http.MethodGet, "/medium", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Body.Len()
	}
}

// BenchmarkContext_JSON_Large measures JSON encoding for a list of 100 records
// (~12 KB).  Exercises buffer pool and serializer for bulk endpoints.
func BenchmarkContext_JSON_Large(b *testing.B) {
	type item struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	items := make([]item, 100)
	for i := range items {
		items[i] = item{ID: i, Title: "title-" + strconv.Itoa(i), Body: strings.Repeat("x", 80)}
	}
	app := astra.New()
	app.GET("/large", func(c *astra.Ctx) error {
		return c.JSON(200, items)
	})
	req := httptest.NewRequest(http.MethodGet, "/large", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Body.Len()
	}
}

// BenchmarkContext_JSONStream_Large is the JSONStream counterpart of
// BenchmarkContext_JSON_Large.  JSONStream encodes directly into the
// ResponseWriter, eliminating the intermediate pooled buffer copy.
// Expected gain: ~13 KB/op less memory, fewer allocs; trade-off is no
// Content-Length header (chunked transfer on HTTP/1.1).
func BenchmarkContext_JSONStream_Large(b *testing.B) {
	type item struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	items := make([]item, 100)
	for i := range items {
		items[i] = item{ID: i, Title: "title-" + strconv.Itoa(i), Body: strings.Repeat("x", 80)}
	}
	app := astra.New()
	app.GET("/stream", func(c *astra.Ctx) error {
		return c.JSONStream(200, items)
	})
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Body.Len()
	}
}

// BenchmarkContext_String measures plain-text response writing.
func BenchmarkContext_String(b *testing.B) {
	app := astra.New()
	app.GET("/str", func(c *astra.Ctx) error {
		return c.String(200, "hello world")
	})
	req := httptest.NewRequest(http.MethodGet, "/str", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Body.Len()
	}
}

// BenchmarkContext_QueryParams measures URL query-string parsing.
// Uses five parameters — a realistic REST API search/filter payload.
func BenchmarkContext_QueryParams(b *testing.B) {
	app := astra.New()
	app.GET("/search", func(c *astra.Ctx) error {
		_ = c.Query("q")
		_ = c.Query("page")
		_ = c.Query("limit")
		_ = c.Query("sort")
		_ = c.Query("order")
		return c.NoContent(200)
	})
	req := httptest.NewRequest(http.MethodGet,
		"/search?q=benchmark&page=1&limit=20&sort=created_at&order=desc", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// ─── Full ServeHTTP round-trip benchmarks (parallel) ─────────────────────────

// BenchmarkServeHTTP_Parallel_Static measures concurrent end-to-end throughput
// on a single static route using httptest.ResponseRecorder.
//
// The 208 B / 4 allocs/op are from httptest.NewRecorder() inside the hot loop,
// not from the App's sync.Pool (which is zero-alloc after warm-up).  The
// apparent latency regression at cpu=8 on asymmetric CPUs (e.g. Apple M4
// 4P+6E) reflects efficiency-core scheduling overhead, not Pool contention.
func BenchmarkServeHTTP_Parallel_Static(b *testing.B) {
	app := astra.New()
	app.GET("/ping", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			benchSink = w.Code
		}
	})
}

// BenchmarkServeHTTP_Parallel_JSON measures parallel JSON response throughput.
func BenchmarkServeHTTP_Parallel_JSON(b *testing.B) {
	app := astra.New()
	payload := mediumPayload{
		ID: 1, Name: "Bob", Email: "bob@example.com", Role: "user", Active: true,
	}
	app.GET("/user", func(c *astra.Ctx) error {
		return c.JSON(200, payload)
	})
	req := httptest.NewRequest(http.MethodGet, "/user", nil)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
			benchSink = w.Body.Len()
		}
	})
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// nopHandler is a zero-work handler used to isolate router and middleware
// overhead from response-writing overhead.
func nopHandler(c *astra.Ctx) error {
	return c.NoContent(http.StatusOK)
}

// ── Deep-param pool-sizing benchmarks ────────────────────────────────────────
//
// These benchmarks quantify the alloc cliff at maxRouteParams (8) and verify
// that sealPool() eliminates it for deeper routes.
//
// Run:
//
//	go test -bench=BenchmarkRouter_DeepParam -benchmem -count=6 -benchtime=1s .

// BenchmarkRouter_DeepParam_8_NoSeal is the ≤8 baseline: inline paramsArr is
// sufficient, sealPool is a no-op, zero overflow allocs.
func BenchmarkRouter_DeepParam_8_NoSeal(b *testing.B) {
	app := astra.New()
	app.GET("/a/:p1/b/:p2/c/:p3/d/:p4/e/:p5/f/:p6/g/:p7/h/:p8", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/a/1/b/2/c/3/d/4/e/5/f/6/g/7/h/8", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_DeepParam_9_NoSeal measures the alloc cliff: the 9th param
// overflows paramsArr, causing a 512 B heap allocation on every request.
func BenchmarkRouter_DeepParam_9_NoSeal(b *testing.B) {
	app := astra.New()
	app.GET("/a/:p1/b/:p2/c/:p3/d/:p4/e/:p5/f/:p6/g/:p7/h/:p8/i/:p9", nopHandler)
	req := httptest.NewRequest(http.MethodGet, "/a/1/b/2/c/3/d/4/e/5/f/6/g/7/h/8/i/9", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_DeepParam_9_Sealed verifies that sealPool() pre-sizes the
// params slice so the 9th param no longer triggers a heap allocation.
func BenchmarkRouter_DeepParam_9_Sealed(b *testing.B) {
	app := astra.New()
	app.GET("/a/:p1/b/:p2/c/:p3/d/:p4/e/:p5/f/:p6/g/:p7/h/:p8/i/:p9", nopHandler)
	app.SealPool() // simulates what Run() does before the first request
	req := httptest.NewRequest(http.MethodGet, "/a/1/b/2/c/3/d/4/e/5/f/6/g/7/h/8/i/9", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkRouter_DeepParam_12_Sealed exercises a 12-param route after sealing.
// Allocs should match the 8-param baseline (no overflow).
func BenchmarkRouter_DeepParam_12_Sealed(b *testing.B) {
	app := astra.New()
	app.GET("/a/:p1/b/:p2/c/:p3/d/:p4/e/:p5/f/:p6/g/:p7/h/:p8/i/:p9/j/:p10/k/:p11/l/:p12", nopHandler)
	app.SealPool()
	req := httptest.NewRequest(http.MethodGet, "/a/1/b/2/c/3/d/4/e/5/f/6/g/7/h/8/i/9/j/10/k/11/l/12", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// ─── 405 MethodNotAllowed benchmarks ─────────────────────────────────────────
//
// These benchmarks measure the cost of a 405 MethodNotAllowed response, which
// requires traversing all registered method trees to build the Allow header
// (RFC 9110 §15.5.6).  The slow path is acceptable because 405 requests are
// rare and erroneous; the primary goal is correctness, not throughput.

// BenchmarkMethodNotAllowed_5Methods registers GET/POST/PUT/DELETE/PATCH and
// sends a HEAD request (no HEAD tree), triggering full traversal of all 5 trees.
func BenchmarkMethodNotAllowed_5Methods(b *testing.B) {
	app := astra.New()
	app.GET("/api/users/:id", nopHandler)
	app.POST("/api/users/:id", nopHandler)
	app.PUT("/api/users/:id", nopHandler)
	app.DELETE("/api/users/:id", nopHandler)
	app.PATCH("/api/users/:id", nopHandler)
	req := httptest.NewRequest(http.MethodHead, "/api/users/42", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}

// BenchmarkMethodNotAllowed_1Method is the minimal case: only GET is registered;
// a DELETE request traverses 1 tree to build "Allow: GET".
func BenchmarkMethodNotAllowed_1Method(b *testing.B) {
	app := astra.New()
	app.GET("/item/:id", nopHandler)
	req := httptest.NewRequest(http.MethodDelete, "/item/1", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.ServeHTTP(w, req)
		benchSink = w.Code
	}
}
