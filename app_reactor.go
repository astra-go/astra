package astra

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/astra-go/astra/netengine"
)

// RunReactor starts the HTTP server using the Reactor-pattern engine from the
// netengine package.  It offers significantly lower goroutine overhead under
// high concurrency compared to RunServer / Run:
//
//   - Idle keep-alive connections park in an epoll/kqueue event loop and consume
//     NO goroutine.
//   - Active requests share a bounded worker-pool (default 4xGOMAXPROCS
//     goroutines), so goroutine count stays flat regardless of connection count.
//
// RunReactor participates in the same OnStart / OnStop lifecycle as Run and
// responds to SIGINT / SIGTERM for graceful shutdown.
//
// # Compatibility boundaries
//
// The Reactor engine bypasses net/http's connection management, which means
// certain standard library features are unavailable on the Reactor path:
//
//   - http.Hijacker: WebSocket upgrades and other connection hijacks are not
//     supported. Use RunServer (which wraps net/http) for WebSocket endpoints.
//   - http.Flusher / http.ResponseController: The response is fully buffered
//     before being written to the wire; incremental flushing for SSE or chunked
//     streaming is not available.
//   - http2.ConfigureServer: Requires an *http.Server which does not exist in
//     Reactor mode. For TLS+H2, use RunReactorTLS (which wires http2.Server
//     automatically) or call RunServer + http2.ConfigureServer for full control.
//   - Third-party net/http middleware that reads or modifies *http.Server
//     internals will not function here.
//
// Standard http.Handler middleware that only inspects or modifies requests and
// responses (headers, body, status code) works normally because the engine calls
// handler.ServeHTTP with a compliant http.ResponseWriter. Use RunReactorHandler
// to pass a wrapped handler when you need that layer.
//
// # Fallback to net/http
//
// On platforms without epoll or kqueue (e.g. Windows), RunReactor automatically
// falls back to the standard net/http server, which has full compatibility.
// To explicitly use net/http on any platform — for Hijacker, streaming, or
// http2.ConfigureServer — call RunServer instead:
//
//	srv := &http.Server{Addr: addr, Handler: app}
//	http2.ConfigureServer(srv, nil) // full H2 support
//	app.RunServer(srv)
func (a *App) RunReactor(addr string) error {
	return a.runReactor(addr, nil, a)
}

// RunReactorHandler is like RunReactor but uses h as the HTTP handler instead of
// the App itself. This allows wrapping the App with standard http.Handler
// middleware before it is handed to the Reactor engine:
//
//	app.RunReactorHandler(":8080", corsMiddleware(app))
//
// The same compatibility boundaries described on RunReactor apply.
func (a *App) RunReactorHandler(addr string, h http.Handler) error {
	return a.runReactor(addr, nil, h)
}

// RunReactorTLS is the TLS variant of RunReactor.
// HTTP/2 is negotiated automatically via ALPN using default http2.Server settings.
// For custom H2 settings or http2.ConfigureServer, use RunServer instead.
func (a *App) RunReactorTLS(addr, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("astra: load TLS cert: %w", err)
	}
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	return a.runReactor(addr, tlsCfg, a)
}

// RunReactorTLSHandler is like RunReactorTLS but uses h as the HTTP handler.
func (a *App) RunReactorTLSHandler(addr, certFile, keyFile string, h http.Handler) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("astra: load TLS cert: %w", err)
	}
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	return a.runReactor(addr, tlsCfg, h)
}

func (a *App) runReactor(addr string, tlsCfg *tls.Config, h http.Handler) error {
	// Run OnStart hooks.
	if err := a.lifecycle.RunStartHooks(context.Background()); err != nil {
		return err
	}

	engine, err := netengine.New(h, netengine.ReactorConfig{})
	if err != nil {
		// epoll/kqueue not available on this platform — fall back to net/http.
		srv := newDefaultServer(addr, h)
		return a.runWithGracefulShutdown(srv, srv.ListenAndServe)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("astra: reactor listen %s: %w", addr, err)
	}
	if tlsCfg != nil {
		// Require at least TLS 1.2. Without this, the minimum version depends on
		// the Go runtime default, which is not an explicit code constraint and can
		// silently degrade if the binary is compiled with an older toolchain or if
		// the config is reused in a lower-Go environment. Callers that need TLS 1.3
		// exclusively can set MinVersion themselves before calling RunReactorTLS.
		if tlsCfg.MinVersion == 0 {
			tlsCfg.MinVersion = tls.VersionTLS12
		}
		// Advertise h2 and http/1.1 via ALPN so clients can negotiate HTTP/2.
		tlsCfg.NextProtos = append([]string{"h2", "http/1.1"}, tlsCfg.NextProtos...)
		ln = tls.NewListener(ln, tlsCfg)
		// Register an http2.Server so the engine routes h2 connections off the
		// Reactor path and into Go's standard HTTP/2 implementation.
		h2srv := &http2.Server{}
		engine.EnableH2(h2srv)
	}

	// Watch for OS signals; close the listener to unblock engine.Serve.
	// The done channel is closed when Serve returns so the goroutine exits
	// even if no signal arrives (e.g. engine stopped due to an error).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-quit:
			ln.Close()
		case <-done:
		}
	}()

	serveErr := engine.Serve(ln)

	// Deregister the channel and unblock the goroutine if it hasn't exited yet.
	signal.Stop(quit)
	close(done)

	// Run OnStop hooks regardless of how Serve returned.
	timeout := time.Duration(a.options.ShutdownTimeout) * time.Second
	stopCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	a.lifecycle.RunStopHooks(stopCtx)

	// ln.Close() causes Serve to return a "use of closed network connection"
	// error which indicates a clean shutdown — not an application error.
	if serveErr != nil && !isClosedErr(serveErr) {
		return serveErr
	}
	return nil
}

// isClosedErr reports whether err is the benign "use of closed network
// connection" error that results from intentionally closing a listener.
func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	// net package returns this string for closed-connection operations.
	const closed = "use of closed network connection"
	e := err.Error()
	for i := 0; i <= len(e)-len(closed); i++ {
		if e[i:i+len(closed)] == closed {
			return true
		}
	}
	return false
}

// RunReactorH2C starts the Reactor engine with h2c (plain-text HTTP/2) support.
// Both HTTP/1.1 and HTTP/2 clients can connect without TLS.
// For TLS+H2, use RunReactorTLS instead.
func (a *App) RunReactorH2C(addr string) error {
	h2srv := &http2.Server{}
	handler := h2c.NewHandler(a, h2srv)
	return a.runReactor(addr, nil, handler)
}

// WSEventLooper is the interface that the websocket.WSEventLoop satisfies.
// It allows app.RunReactorWS to accept the event loop without importing
// the websocket package (avoiding a circular dependency).
type WSEventLooper interface {
	RegisterEngine(e *netengine.Engine)
}

// RunReactorWS starts the Reactor engine with WebSocket event loop integration.
// This is the recommended way to run an Astra server that serves both HTTP
// and WebSocket endpoints at high concurrency.
//
// The wsLoop parameter must be a *websocket.WSEventLoop (created via
// websocket.NewWSEventLoop). It is bound to the Reactor engine so that
// WebSocket connections are managed by epoll/kqueue instead of goroutines.
//
// Example:
//
//	wsLoop := websocket.NewWSEventLoop(
//	    websocket.WithOnMessage(func(c *websocket.WSConn, t int, d []byte) {
//	        c.WriteMessage(t, d) // echo
//	    }),
//	)
//	app.GET("/ws", wsLoop.Handler())
//	app.RunReactorWS(":8080", wsLoop)
//
// Note: WebSocket endpoints MUST use wsLoop.Handler() (not websocket.Handler)
// when using RunReactorWS. The standard Hub-mode Handler spawns goroutines
// and does not integrate with the Reactor engine.
func (a *App) RunReactorWS(addr string, wsLoop WSEventLooper) error {
	return a.runReactorWS(addr, nil, a, wsLoop)
}

// RunReactorWSHandler is like RunReactorWS but uses h as the HTTP handler
// instead of the App itself.
func (a *App) RunReactorWSHandler(addr string, h http.Handler, wsLoop WSEventLooper) error {
	return a.runReactorWS(addr, nil, h, wsLoop)
}

func (a *App) runReactorWS(addr string, tlsCfg *tls.Config, h http.Handler, wsLoop WSEventLooper) error {
	// Run OnStart hooks.
	if err := a.lifecycle.RunStartHooks(context.Background()); err != nil {
		return err
	}

	engine, err := netengine.New(h, netengine.ReactorConfig{})
	if err != nil {
		// epoll/kqueue not available on this platform — fall back to net/http.
		srv := newDefaultServer(addr, h)
		return a.runWithGracefulShutdown(srv, srv.ListenAndServe)
	}

	// Bind the WebSocket event loop to the engine BEFORE serving.
	wsLoop.RegisterEngine(engine)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("astra: reactor-ws listen %s: %w", addr, err)
	}
	if tlsCfg != nil {
		if tlsCfg.MinVersion == 0 {
			tlsCfg.MinVersion = tls.VersionTLS12
		}
		tlsCfg.NextProtos = append([]string{"h2", "http/1.1"}, tlsCfg.NextProtos...)
		ln = tls.NewListener(ln, tlsCfg)
		h2srv := &http2.Server{}
		engine.EnableH2(h2srv)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-quit:
			ln.Close()
		case <-done:
		}
	}()

	serveErr := engine.Serve(ln)

	signal.Stop(quit)
	close(done)

	timeout := time.Duration(a.options.ShutdownTimeout) * time.Second
	stopCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	a.lifecycle.RunStopHooks(stopCtx)

	if serveErr != nil && !isClosedErr(serveErr) {
		return serveErr
	}
	return nil
}
