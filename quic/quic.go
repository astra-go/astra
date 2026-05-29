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
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// RunQUIC starts an HTTP/3 (QUIC) server on addr and simultaneously starts a
// TLS server that advertises HTTP/3 via the Alt-Svc response header. Clients
// that support HTTP/3 will upgrade automatically on their next request.
//
// addr, certFile, and keyFile have the same semantics as app.RunTLS.
//
//	astraquic.RunQUIC(app, ":443", "cert.pem", "key.pem")
func RunQUIC(app *astra.App, addr, certFile, keyFile string) error {
	return RunQUICWithOptions(app, addr, certFile, keyFile)
}

// RunQUICWithOptions starts HTTP/3 and companion TLS servers with full
// configuration control. RunQUIC is a zero-option convenience wrapper around
// this function.
//
//	astraquic.RunQUICWithOptions(app, ":443", "cert.pem", "key.pem",
//	    astraquic.WithAllow0RTT(true),
//	    astraquic.WithMaxIdleTimeout(60*time.Second),
//	    astraquic.WithTLSAddr(":8443"),
//	)
func RunQUICWithOptions(app *astra.App, addr, certFile, keyFile string, opts ...QUICOption) error {
	o := defaultQUICOptions()
	for _, opt := range opts {
		opt(o)
	}

	tlsAddr := o.TLSAddr
	if tlsAddr == "" {
		tlsAddr = addr
	}

	tlsCfg := o.TLSConfig
	if tlsCfg == nil {
		tlsCfg = defaultTLSConfig()
	}

	altSvcValue := fmt.Sprintf(`h3="%s"; ma=%d`, tlsAddr, o.AltSvcMaxAge)

	h3srv := &http3.Server{
		Addr:      addr,
		Handler:   app,
		TLSConfig: tlsCfg,
		QUICConfig: &quic.Config{
			Allow0RTT:          o.Allow0RTT,
			MaxIdleTimeout:     o.MaxIdleTimeout,
			MaxIncomingStreams: o.MaxIncomingStreams,
		},
	}
	tlsSrv := &http.Server{
		Addr:         tlsAddr,
		Handler:      altSvcHandler(app, altSvcValue),
		TLSConfig:    tlsCfg.Clone(),
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

	var startErr error
	select {
	case startErr = <-errCh:
		signal.Stop(quit)
	case <-quit:
		signal.Stop(quit)
	}

	timeout := time.Duration(app.ShutdownTimeout()) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_ = app.Stop(shutCtx)
	_ = h3srv.Shutdown(shutCtx)
	_ = tlsSrv.Shutdown(shutCtx)

	return startErr
}

func altSvcHandler(next http.Handler, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", value)
		next.ServeHTTP(w, r)
	})
}
