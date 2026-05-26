// Package testapp provides a self-contained Astra application used exclusively
// by the e2e test suite. It wires together HTTP, WebSocket, and gRPC endpoints
// so the tests can exercise the full request lifecycle without any external deps.
package testapp

import (
	"net"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
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
func New(t testing.TB) *App {
	t.Helper()

	store := NewUserStore()
	astraApp := astra.New(astra.WithMode(astra.ModeTest))

	// ── Auth routes ──────────────────────────────────────────────────────────
	auth := astraApp.Group("/auth")
	auth.POST("/register", registerHandler(store))
	auth.POST("/login", loginHandler(store, TestJWTSecret))

	// ── Protected API routes ─────────────────────────────────────────────────
	jwtMW := middleware.JWT(TestJWTSecret)
	api := astraApp.Group("/api", jwtMW)
	api.GET("/me", meHandler(store))

	// ── WebSocket endpoint ───────────────────────────────────────────────────
	// Token is passed via query param because browsers cannot set custom headers
	// during the WebSocket handshake.
	hub := websocket.NewHub()
	go hub.Run()

	astraApp.GET("/ws", middleware.JWTWithConfig(middleware.JWTConfig{
		Secret:      TestJWTSecret,
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
