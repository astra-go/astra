// Basic example: demonstrates core Astra features.
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	sec "github.com/astra-go/astra/middleware/security"
)

func main() {
	app := astra.New(
		astra.WithShutdownTimeout(10),
	)

	// ─── Global Middleware ────────────────────────────────────────────────────
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORS(),
		middleware.Timeout(30*time.Second),
	)

	// ─── Health ───────────────────────────────────────────────────────────────
	app.GET("/ping", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "pong")
	})

	app.GET("/health", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// ─── Path & Query Parameters ──────────────────────────────────────────────
	app.GET("/hello/:name", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{
			"message": fmt.Sprintf("Hello, %s!", c.Param("name")),
		})
	})

	app.GET("/search", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{
			"q":    c.DefaultQuery("q", "golang"),
			"page": c.DefaultQuery("page", "1"),
		})
	})

	// ─── JSON Body Binding ────────────────────────────────────────────────────
	app.POST("/echo", func(c *astra.Ctx) error {
		var body map[string]any
		if err := c.BindJSON(&body); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, body)
	})

	// ─── SSE (Server-Sent Events) ─────────────────────────────────────────────
	app.GET("/events", func(c *astra.Ctx) error {
		for i := range 5 {
			if err := c.SSEvent("tick", fmt.Sprintf(`{"n":%d}`, i)); err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
		}
		return nil
	})

	// ─── JWT-protected routes ─────────────────────────────────────────────────
	api := app.Group("/api/v1")
	api.Use(sec.JWT("change-me-in-prod"))

	api.GET("/me", func(c *astra.Ctx) error {
		claims := sec.GetClaims(c)
		sub := ""
		if claims != nil {
			sub, _ = claims.GetSubject()
		}
		return c.JSON(http.StatusOK, astra.Map{"sub": sub})
	})

	// ─── Rate-limited group ───────────────────────────────────────────────────
	limited := app.Group("/limited")
	limited.Use(sec.RateLimit(10, 5))

	limited.GET("/resource", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"data": "rate-limited"})
	})

	// ─── Lifecycle ────────────────────────────────────────────────────────────
	app.OnStart(func(_ context.Context) error {
		fmt.Println("server starting…")
		return nil
	})
	app.OnStop(func(_ context.Context) error {
		fmt.Println("server stopping…")
		return nil
	})

	fmt.Println("listening on :8080")
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
