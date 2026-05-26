package astra

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// RunQUIC starts an HTTP/3 (QUIC) server on addr and simultaneously starts a
// TLS server that advertises HTTP/3 via the Alt-Svc response header. Clients
// that support HTTP/3 will upgrade automatically on their next request.
//
// addr, certFile, and keyFile have the same semantics as RunTLS.
//
// Both servers share the same handler (the App) and the same graceful shutdown
// lifecycle. On SIGINT / SIGTERM, the HTTP/3 server closes gracefully and the
// TLS server is shut down.
//
//	app.RunQUIC(":443", "cert.pem", "key.pem")
func (a *App) RunQUIC(addr, certFile, keyFile string) error {
	// Determine the port for the Alt-Svc header.
	port := portFromAddr(addr)

	// HTTP/3 server
	h3srv := &http3.Server{
		Addr:    addr,
		Handler: a,
	}

	// TLS (HTTP/1.1 + HTTP/2) server — advertises Alt-Svc so browsers upgrade.
	altSvcValue := fmt.Sprintf(`h3="%s"; ma=86400`, addr)
	tlsSrv := newDefaultServer(addr, altSvcMiddleware(a, altSvcValue))

	return a.runQUICWithGracefulShutdown(h3srv, tlsSrv, certFile, keyFile, port)
}

// runQUICWithGracefulShutdown runs both the HTTP/3 and TLS servers, handles OS
// signals, and shuts both down gracefully.
func (a *App) runQUICWithGracefulShutdown(
	h3srv *http3.Server,
	tlsSrv *http.Server,
	certFile, keyFile string,
	_ string,
) error {
	if err := a.lifecycle.RunStartHooks(context.Background()); err != nil {
		return err
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 2)

	// Start HTTP/3
	go func() {
		slog.Info("astra: HTTP/3 server starting", "addr", h3srv.Addr)
		if err := h3srv.ListenAndServeTLS(certFile, keyFile); err != nil {
			errCh <- fmt.Errorf("http3: %w", err)
		}
	}()

	// Start TLS (Alt-Svc upgrade path)
	go func() {
		slog.Info("astra: TLS server starting (Alt-Svc)", "addr", tlsSrv.Addr)
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

	timeout := time.Duration(a.options.ShutdownTimeout) * time.Second
	shutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run stop hooks.
	a.lifecycle.RunStopHooks(shutCtx)

	_ = h3srv.Shutdown(shutCtx)
	_ = tlsSrv.Shutdown(shutCtx)
	return nil
}

// portFromAddr returns the port string from an "host:port" address.
func portFromAddr(addr string) string {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr
	}
	return addr[idx+1:]
}

// altSvcMiddleware wraps a handler and injects the Alt-Svc header into every response.
func altSvcMiddleware(next http.Handler, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", value)
		next.ServeHTTP(w, r)
	})
}
