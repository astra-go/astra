//go:build wasip1 || js

// WASM example — runs Astra inside WasmEdge or any wasip1-compatible runtime.
//
// Build:
//
//	GOOS=wasip1 GOARCH=wasm go build -o astra.wasm .
//
// Run with WasmEdge (requires WasmEdge ≥ 0.13 with WASI socket support):
//
//	wasmedge --net-enable astra.wasm
//
// Then test:
//
//	curl http://localhost:8080/
//	curl http://localhost:8080/hello/world
//	curl -X POST http://localhost:8080/echo \
//	     -H "Content-Type: application/json" \
//	     -d '{"message":"hi"}'
package main

import (
	"context"
	"net/http"

	"github.com/astra-go/astra"
)

func main() {
	app := astra.New()

	app.GET("/", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{
			"runtime": "wasip1",
			"message": "hello from astra wasm",
		})
	})

	app.GET("/hello/:name", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{
			"hello": c.Param("name"),
		})
	})

	app.POST("/echo", func(c *astra.Ctx) error {
		var body map[string]any
		if err := c.BindJSON(&body); err != nil {
			return astra.ErrBadRequest
		}
		return c.JSON(http.StatusOK, body)
	})

	// ServeWASI blocks until ctx is cancelled or the server fails.
	// Pass context.Background() for a long-running edge function;
	// use a cancellable context to trigger graceful shutdown programmatically.
	if err := app.ServeWASI(context.Background(), ":8080"); err != nil {
		panic(err)
	}
}
