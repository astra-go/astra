// Package quic adds HTTP/3 (QUIC) support to an Astra application.
//
// Import this sub-module when you need HTTP/3; it is intentionally kept out of
// the core module to avoid pulling quic-go's ~40 transitive dependencies into
// projects that only use HTTP/1.1 or HTTP/2.
//
//	import astraquic "github.com/astra-go/astra/quic"
//
//	app := astra.New()
//	// ... register routes ...
//	astraquic.RunQUIC(app, ":443", "cert.pem", "key.pem")
package quic

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	astra "github.com/astra-go/astra"
	"github.com/quic-go/quic-go/http3"
)

// RunQUIC starts an HTTP/3 (QUIC) server on addr and simultaneously starts a
// TLS server that advertises HTTP/3 via the Alt-Svc response header. Clients
// that support HTTP/3 will upgrade automatically on their next request.
//
// addr, certFile, and keyFile have the same semantics as app.RunTLS.
//
// Both servers share the same handler (app) and the same graceful shutdown
// lifecycle. On SIGINT / SIGTERM, both servers shut down gracefully.
//
//	astraquic.RunQUIC(app, ":443", "cert.pem", "key.pem")
func RunQUIC(app *astra.App, addr, certFile, keyFile string) error {
	altSvcValue := fmt.Sprintf(`h3="%s"; ma=86400`, addr)

	h3srv := &http3.Server{
		Addr:    addr,
		Handler: app,
	}
	tlsSrv := &http.Server{
		Addr:         addr,
		Handler:      altSvcHandler(app, altSvcValue),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return runWithGracefulShutdown(app, h3srv, tlsSrv, certFile, keyFile)
}

func runWithGracefulShutdown(
	app *astra.App,
	h3srv *http3.Server,
	tlsSrv *http.Server,
	certFile, keyFile string,
) error {
	if err := app.Start(context.Background()); err != nil {
		return err
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 2)

	go func() {
		slog.Info("astra/quic: HTTP/3 server starting", "addr", h3srv.Addr)
		if err := h3srv.ListenAndServeTLS(certFile, keyFile); err != nil {
			errCh <- fmt.Errorf("http3: %w", err)
		}
	}()

	go func() {
		slog.Info("astra/quic: TLS server starting (Alt-Svc)", "addr", tlsSrv.Addr)
		if err := tlsSrv.ListenAndServeTLS(certFile, keyFile); err != nil &&
			err != http.ErrServerClosed {
			errCh <- fmt.Errorf("tls: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		signal.Stop(quit)
		return err
	case <-quit:
	}
	signal.Stop(quit)

	timeout := time.Duration(app.ShutdownTimeout()) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_ = app.Stop(shutCtx)
	_ = h3srv.Shutdown(shutCtx)
	_ = tlsSrv.Shutdown(shutCtx)
	return nil
}

func altSvcHandler(next http.Handler, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", value)
		next.ServeHTTP(w, r)
	})
}
