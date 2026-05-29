// HTTP/3 example: demonstrates running Astra with QUIC support.
//
// Prerequisites:
//
//	# Generate a self-signed certificate for local testing
//	openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem \
//	    -days 365 -nodes -subj "/CN=localhost"
//
// Run:
//
//	go run main.go
//
// Test with curl (requires curl 7.88+ with HTTP/3 support):
//
//	curl --http3 https://localhost:443/ping -k
//	curl --http3 https://localhost:443/health -k
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	astraquic "github.com/astra-go/astra/quic"
)

func main() {
	app := astra.New(
		astra.WithShutdownTimeout(10),
	)

	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
	)

	app.GET("/ping", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "pong")
	})

	app.GET("/health", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, astra.Map{
			"status":    "ok",
			"transport": c.Request().Proto, // shows "HTTP/3.0" for QUIC clients
			"time":      time.Now().Format(time.RFC3339),
		})
	})

	app.OnStart(func(_ context.Context) error {
		fmt.Println("server starting on :443 (HTTP/3 + TLS)")
		return nil
	})
	app.OnStop(func(_ context.Context) error {
		fmt.Println("server stopping…")
		return nil
	})

	// RunQUICWithOptions starts both an HTTP/3 (QUIC/UDP) server and a
	// companion TLS server. The TLS server injects Alt-Svc headers so
	// HTTP/1.1 clients automatically upgrade to HTTP/3 on their next request.
	//
	// For production, replace cert.pem / key.pem with real certificates
	// (e.g. from Let's Encrypt via certbot or acme.sh).
	err := astraquic.RunQUICWithOptions(
		app,
		":443",       // QUIC (UDP) listen address
		"cert.pem",   // TLS certificate
		"key.pem",    // TLS private key
		// Uncomment to enable 0-RTT for repeat connections (read the security
		// note in options.go before enabling in production):
		// astraquic.WithAllow0RTT(true),
		astraquic.WithMaxIdleTimeout(60*time.Second),
		astraquic.WithMaxIncomingStreams(200),
	)
	if err != nil {
		panic(err)
	}
}
