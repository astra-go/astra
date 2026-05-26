//go:build wasip1 || js

package astra

import (
	"context"
	"net/http"
	"time"
)

// ServeWASI starts the HTTP server for WASM runtimes (wasip1 / WasmEdge).
//
// Unlike Run, ServeWASI does not listen for OS signals — the WASM host
// controls the process lifetime. Shutdown is triggered by cancelling ctx.
//
// For WasmEdge with WASI socket support, addr follows the same format as
// Run (e.g. ":8080"). The host must have granted the socket capability.
//
// Example:
//
//	app := astra.New()
//	app.GET("/", handler)
//	app.ServeWASI(context.Background(), ":8080")
func (a *App) ServeWASI(ctx context.Context, addr string) error {
	a.sealPool()

	if a.lifecycle != nil {
		if err := a.lifecycle.RunStartHooks(ctx); err != nil {
			return err
		}
	}

	srv := newDefaultServer(addr, a)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutCtx := context.Background()
	var cancel context.CancelFunc
	if a.options.ShutdownTimeout > 0 {
		shutCtx, cancel = context.WithTimeout(
			context.Background(),
			time.Duration(a.options.ShutdownTimeout)*time.Second,
		)
		defer cancel()
	}

	if a.lifecycle != nil {
		a.lifecycle.RunStopHooks(shutCtx)
	}
	return srv.Shutdown(shutCtx)
}

// Handler returns the App as an http.Handler.
// Use this when the WASM host provides its own request dispatch loop and
// calls ServeHTTP directly, rather than opening a TCP listener.
func (a *App) Handler() http.Handler { return a }
