// Package testapp provides a self-contained Astra application used exclusively
// by the e2e test suite. It wires together HTTP, WebSocket, and gRPC endpoints
// so the tests can exercise the full request lifecycle without any external deps.
package testapp

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/e2e/chaos"
	"github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestJWTSecret is the shared HMAC secret used by the testapp.
// Never use this value outside of tests.
const TestJWTSecret = "e2e-test-secret-do-not-use-in-prod"

// App bundles the HTTP test server and the gRPC server for e2e tests.
type App struct {
	// HTTP is the running httptest.Server (HTTP + WebSocket endpoints).
	HTTP *httptest.Server
	// GRPCAddr is the address the gRPC server is listening on.
	GRPCAddr string

	grpcSrv *grpc.Server
}

// New creates and starts a testapp. It registers t.Cleanup to stop all servers
// when the test ends, so callers never need to call Close() explicitly.
// New creates and starts a testapp. It registers t.Cleanup to stop all servers
// when the test ends, so callers never need to call Close() explicitly.
func New(t testing.TB) *App {
	return NewWithInjector(t, nil)
}

// NewWithInjector creates a testapp with an optional FaultInjector for chaos tests.
func NewWithInjector(t testing.TB, injector *chaos.FaultInjector) *App {
	t.Helper()

	store := NewUserStore()
	astraApp := astra.New(astra.WithMode(astra.ModeTest))

	// ── Chaos middleware ────────────────────────────────────────────────────────
	if injector != nil {
		astraApp.Use(injector.Middleware())
	}

	// ── Chaos engineering endpoints (test mode only) ───────────────────────────
	if injector != nil {
		chaosGroup := astraApp.Group("/chaos")
		chaosGroup.GET("/timeout", chaosTimeoutHandler(injector))
		chaosGroup.GET("/error", chaosErrorHandler(injector))
		chaosGroup.GET("/latency", chaosLatencyHandler(injector))
		chaosGroup.GET("/panic", chaosPanicHandler(injector))
		chaosGroup.POST("/inject", chaosInjectHandler(injector))
		chaosGroup.POST("/reset", chaosResetHandler(injector))
	}

	// ── Auth routes ──────────────────────────────────────────────────────────
	auth := astraApp.Group("/auth")
	auth.POST("/register", registerHandler(store))
	auth.POST("/login", loginHandler(store, TestJWTSecret))

	// ── Protected API routes ─────────────────────────────────────────────────
	jwtMW := security.JWT(TestJWTSecret)
	api := astraApp.Group("/api", jwtMW)
	api.GET("/me", meHandler(store))

	// ── WebSocket endpoint ───────────────────────────────────────────────────
	// Token is passed via query param because browsers cannot set custom headers
	// during the WebSocket handshake.
	hub := websocket.NewHub()
	go hub.Run()

	astraApp.GET("/ws", security.JWTWithConfig(security.JWTConfig{
		Secret:      security.NewSecretString(TestJWTSecret),
		TokenLookup: "query:token",
	}), websocket.Handler(hub, func(client *websocket.Client, msg []byte) {
		client.Send(msg) // echo back to sender
	}))

	// ── HTTP test server ─────────────────────────────────────────────────────
	httpSrv := httptest.NewServer(astraApp)

	// ── gRPC server ──────────────────────────────────────────────────────────
	// Bind to :0 so the OS assigns a free port — avoids conflicts in parallel tests.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testapp: gRPC listen: %v", err)
	}

	// Wrap with the dual-stack server's built-in interceptors (error encoding, etc.)
	grpcSrv := grpc.NewServer()
	echoSvc := NewEchoService(TestJWTSecret)
	grpcSrv.RegisterService(&EchoServiceDesc, echoSvc)

	go func() { _ = grpcSrv.Serve(lis) }()

	app := &App{
		HTTP:     httpSrv,
		GRPCAddr: lis.Addr().String(),
		grpcSrv:  grpcSrv,
	}

	t.Cleanup(func() {
		grpcSrv.GracefulStop()
		httpSrv.Close()
	})

	return app
}

// HTTPURL returns the base URL of the HTTP test server.
func (a *App) HTTPURL() string { return a.HTTP.URL }

// ── Chaos engineering handlers ────────────────────────────────────────────────

type chaosInjectReq struct {
	Endpoint  string  `json:"endpoint"`
	FaultType string  `json:"fault_type"`  // timeout | error | latency | panic
	Duration  string  `json:"duration"`   // for timeout/latency, e.g. "5s"
	ErrRate   float64 `json:"err_rate"`   // for error, 0.0~1.0
}

func chaosTimeoutHandler(injector *chaos.FaultInjector) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "timeout endpoint ready"})
	}
}

func chaosErrorHandler(injector *chaos.FaultInjector) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "error endpoint ready"})
	}
}

func chaosLatencyHandler(injector *chaos.FaultInjector) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "latency endpoint ready"})
	}
}

func chaosPanicHandler(injector *chaos.FaultInjector) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "panic endpoint ready"})
	}
}

func chaosInjectHandler(injector *chaos.FaultInjector) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		var req chaosInjectReq
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		switch req.FaultType {
		case "timeout":
			dur, _ := time.ParseDuration(req.Duration)
			if dur <= 0 {
				dur = time.Second
			}
			injector.InjectTimeout(req.Endpoint, dur)
		case "error":
			rate := req.ErrRate
			if rate <= 0 {
				rate = 0.5
			}
			injector.InjectError(req.Endpoint, rate)
		case "latency":
			dur, _ := time.ParseDuration(req.Duration)
			if dur <= 0 {
				dur = time.Second
			}
			injector.InjectLatency(req.Endpoint, dur)
		case "panic":
			injector.InjectPanic(req.Endpoint)
		default:
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "unknown fault_type"})
		}

		return c.JSON(http.StatusOK, map[string]string{"status": "injected", "endpoint": req.Endpoint, "fault_type": req.FaultType})
	}
}

func chaosResetHandler(injector *chaos.FaultInjector) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		injector.Reset()
		return c.JSON(http.StatusOK, map[string]string{"status": "reset"})
	}
}

// GRPCConn dials the gRPC server and returns a client connection.
// The connection is registered with t.Cleanup for automatic close.
func (a *App) GRPCConn(t testing.TB) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(a.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("testapp: grpc dial %s: %v", a.GRPCAddr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
