// Quickstart example — everything a real service needs, nothing it doesn't.
//
// Covers: middleware, routing groups, request binding + validation,
// error handling, and graceful shutdown.
// DI / Modules / Plugins are intentionally absent — see examples/basic for those.
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
)

// ─── Request / Response types ─────────────────────────────────────────────────

type CreateItemReq struct {
	Name  string `json:"name"  validate:"required,max=64"`
	Price int    `json:"price" validate:"gte=0"`
}

type Item struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// ─── In-memory store (swap for a real DB later) ───────────────────────────────

var (
	items  []Item
	nextID = 1
)

// ─── Handlers ────────────────────────────────────────────────────────────────

func listItems(c *astra.Ctx) error {
	return c.JSON(http.StatusOK, items)
}

func createItem(c *astra.Ctx) error {
	var req CreateItemReq
	if err := c.ShouldBind(&req); err != nil {
		return err // framework renders 422 + field errors automatically
	}
	item := Item{ID: nextID, Name: req.Name, Price: req.Price}
	nextID++
	items = append(items, item)
	return c.JSON(http.StatusCreated, item)
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	app := astra.New()

	// Global middleware — same pattern as Gin/Echo
	app.Use(
		middleware.Recovery(),
		middleware.Logger(),
		middleware.RequestID(),
	)

	// Public routes
	app.GET("/ping", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "pong")
	})

	// Versioned API group
	v1 := app.Group("/api/v1")
	v1.GET("/items", listItems)
	v1.POST("/items", createItem)

	// Protected group — add JWT middleware to a sub-group
	admin := v1.Group("/admin")
	admin.Use(middleware.JWT("change-me-secret"))
	admin.GET("/stats", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{"total_items": len(items)})
	})

	// Optional: lifecycle hooks for resource init/cleanup
	app.OnStart(func(_ context.Context) error {
		fmt.Println("server ready")
		return nil
	})
	app.OnStop(func(_ context.Context) error {
		fmt.Println("server stopped")
		return nil
	})

	app.Run(":8080")
}
