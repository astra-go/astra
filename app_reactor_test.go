package astra_test

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/astra-go/astra"
)

// TestRunReactorH2C verifies that App.RunReactorH2C accepts HTTP/1.1
// connections on a port that also supports h2c (plain-text HTTP/2).
func TestRunReactorH2C(t *testing.T) {
	app := astra.New()
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "h2c-ok")
	})

	// Use a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	errCh := make(chan error, 1)
	go func() {
		if err := app.RunReactorH2C(addr); err != nil {
			errCh <- err
		}
	}()

	// Give the server a moment to start.
	time.Sleep(200 * time.Millisecond)

	// ── HTTP/1.1 client ──────────────────────────────────────────────────
	t.Run("http1", func(t *testing.T) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("dial http1: %v", err)
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatalf("read http1 response: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("http1 status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})
}
