// TechEmpower-style benchmark server for Astra.
//
// Implements the two stateless TFB test types that require no database:
//
//	GET /json       — JSON Serialization (TFB type 1)
//	GET /plaintext  — Plaintext (TFB type 6)
//
// These endpoints follow the TFB specification:
//   https://github.com/TechEmpower/FrameworkBenchmarks/wiki/Project-Information-Framework-Tests-Overview
//
// Run:
//
//	go run ./examples/techempower/
//
// Quick local throughput check (requires wrk):
//
//	wrk -t4 -c256 -d10s http://localhost:8080/json
//	wrk -t4 -c256 -d10s http://localhost:8080/plaintext
package main

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/astra-go/astra"
)

// helloMessage matches the TFB JSON Serialization response schema exactly.
type helloMessage struct {
	Message string `json:"message"`
}

var tfbHello = helloMessage{Message: "Hello, World!"}

func main() {
	app := astra.New()

	// TFB type 1: JSON Serialization
	// Response: {"message":"Hello, World!"} with Content-Type: application/json
	app.GET("/json", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, tfbHello)
	})

	// TFB type 6: Plaintext
	// Response: "Hello, World!" with Content-Type: text/plain
	app.GET("/plaintext", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "Hello, World!")
	})

	fmt.Printf("TechEmpower benchmark server · GOMAXPROCS=%d\n", runtime.GOMAXPROCS(0))
	fmt.Println("  GET /json       — JSON Serialization (TFB type 1)")
	fmt.Println("  GET /plaintext  — Plaintext (TFB type 6)")
	fmt.Println("listening on :8080")

	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
