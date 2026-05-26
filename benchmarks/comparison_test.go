// Package benchmarks_test — cross-framework comparison benchmarks.
//
// Each BenchmarkVs_ function runs the identical HTTP scenario through Astra,
// Gin, Echo, and Fiber so that the numbers are directly comparable.
// All use httptest.ResponseRecorder (net/http) except Fiber which uses
// app.Test() — an in-process fasthttp adapter that adds ~0.5 µs of
// serialisation overhead, so Fiber numbers are slightly pessimistic.
//
// Run only the comparison suite:
//
//	go test -bench='^BenchmarkVs_' -benchmem -count=6 -benchtime=2s ./benchmarks/
//
// The benchmark-publish CI workflow calls this and feeds the output to
// scripts/bench-to-html.py which generates the public HTML report.
package benchmarks_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/gofiber/fiber/v2"
	fibercors "github.com/gofiber/fiber/v2/middleware/cors"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	echov4 "github.com/labstack/echo/v4"
)

func init() { gin.SetMode(gin.ReleaseMode) }

// ─── helpers ──────────────────────────────────────────────────────────────────

func ginCORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Next()
	}
}

func ginRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Request-ID", "bench")
		c.Next()
	}
}

func echoRequestID() echov4.MiddlewareFunc {
	return func(next echov4.HandlerFunc) echov4.HandlerFunc {
		return func(c echov4.Context) error {
			c.Response().Header().Set("X-Request-ID", "bench")
			return next(c)
		}
	}
}

func fiberRequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Request-ID", "bench")
		return c.Next()
	}
}

// ─── Scenario 1: Baseline — GET /ping → 204 ──────────────────────────────────

func BenchmarkVs_Astra_Baseline(b *testing.B) {
	app := astra.New()
	app.GET("/ping", func(c *astra.Ctx) error { return c.NoContent(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

func BenchmarkVs_Gin_Baseline(b *testing.B) {
	r := gin.New()
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

func BenchmarkVs_Echo_Baseline(b *testing.B) {
	e := echov4.New()
	e.GET("/ping", func(c echov4.Context) error { return c.NoContent(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

func BenchmarkVs_Fiber_Baseline(b *testing.B) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/ping", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		resp, _ := app.Test(req, -1) //nolint:bodyclose
		suiteSink = resp.StatusCode
	}
}

// ─── Scenario 2: Static route → JSON ─────────────────────────────────────────

func BenchmarkVs_Astra_StaticJSON(b *testing.B) {
	app := astra.New()
	app.GET("/api/health", func(c *astra.Ctx) error { return c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Gin_StaticJSON(b *testing.B) {
	r := gin.New()
	r.GET("/api/health", func(c *gin.Context) { c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Echo_StaticJSON(b *testing.B) {
	e := echov4.New()
	e.GET("/api/health", func(c echov4.Context) error { return c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Fiber_StaticJSON(b *testing.B) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/api/health", func(c *fiber.Ctx) error { return c.JSON(fixtureUser) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		resp, _ := app.Test(req, -1) //nolint:bodyclose
		suiteSink = resp.StatusCode
	}
}

// ─── Scenario 3: Param route → JSON ──────────────────────────────────────────

func BenchmarkVs_Astra_ParamJSON(b *testing.B) {
	app := astra.New()
	app.GET("/users/:id", func(c *astra.Ctx) error { return c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Gin_ParamJSON(b *testing.B) {
	r := gin.New()
	r.GET("/users/:id", func(c *gin.Context) { c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Echo_ParamJSON(b *testing.B) {
	e := echov4.New()
	e.GET("/users/:id", func(c echov4.Context) error { return c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Fiber_ParamJSON(b *testing.B) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/users/:id", func(c *fiber.Ctx) error { return c.JSON(fixtureUser) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		resp, _ := app.Test(req, -1) //nolint:bodyclose
		suiteSink = resp.StatusCode
	}
}

// ─── Scenario 4: POST bind JSON body → JSON ───────────────────────────────────

const vsPostBody = `{"name":"Bob","email":"bob@example.com","role":"user"}`

type vsCreateReq struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func BenchmarkVs_Astra_PostBind(b *testing.B) {
	app := astra.New()
	app.POST("/users", func(c *astra.Ctx) error {
		var req vsCreateReq
		if err := c.ShouldBindJSON(&req); err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, fixtureUser)
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(vsPostBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

func BenchmarkVs_Gin_PostBind(b *testing.B) {
	r := gin.New()
	r.POST("/users", func(c *gin.Context) {
		var req vsCreateReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.JSON(http.StatusCreated, fixtureUser)
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(vsPostBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

func BenchmarkVs_Echo_PostBind(b *testing.B) {
	e := echov4.New()
	e.POST("/users", func(c echov4.Context) error {
		var req vsCreateReq
		if err := c.Bind(&req); err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, fixtureUser)
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(vsPostBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		suiteSink = w.Code
	}
}

func BenchmarkVs_Fiber_PostBind(b *testing.B) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/users", func(c *fiber.Ctx) error {
		var req vsCreateReq
		if err := c.BodyParser(&req); err != nil {
			return err
		}
		return c.Status(http.StatusCreated).JSON(fixtureUser)
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(vsPostBody))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req, -1) //nolint:bodyclose
		suiteSink = resp.StatusCode
	}
}

// ─── Scenario 5: 3-middleware stack → JSON ────────────────────────────────────
//
// Stack: Recovery + CORS + RequestID  (all frameworks)
// Each framework uses its own idiomatic recovery + a CORS header setter +
// a request-id header setter so the chain depth is identical (3 handlers).

func BenchmarkVs_Astra_Mw3JSON(b *testing.B) {
	app := astra.New()
	app.Use(middleware.Recovery())
	app.Use(middleware.CORS())
	app.Use(middleware.RequestID())
	app.GET("/users/:id", func(c *astra.Ctx) error { return c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Gin_Mw3JSON(b *testing.B) {
	r := gin.New()
	r.Use(gin.Recovery(), ginCORS(), ginRequestID())
	r.GET("/users/:id", func(c *gin.Context) { c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Echo_Mw3JSON(b *testing.B) {
	e := echov4.New()
	e.Use(func(next echov4.HandlerFunc) echov4.HandlerFunc { // Recovery
		return func(c echov4.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = echov4.ErrInternalServerError
				}
			}()
			return next(c)
		}
	})
	e.Use(func(next echov4.HandlerFunc) echov4.HandlerFunc { // CORS
		return func(c echov4.Context) error {
			c.Response().Header().Set("Access-Control-Allow-Origin", "*")
			return next(c)
		}
	})
	e.Use(echoRequestID())
	e.GET("/users/:id", func(c echov4.Context) error { return c.JSON(http.StatusOK, fixtureUser) })
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		suiteSink = w.Body.Len()
	}
}

func BenchmarkVs_Fiber_Mw3JSON(b *testing.B) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(fiberrecover.New(), fibercors.New(), fiberRequestID())
	app.Get("/users/:id", func(c *fiber.Ctx) error { return c.JSON(fixtureUser) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
		resp, _ := app.Test(req, -1) //nolint:bodyclose
		suiteSink = resp.StatusCode
	}
}
