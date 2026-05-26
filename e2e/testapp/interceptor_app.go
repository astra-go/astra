// interceptor_app.go — testapp variant that wires grpcserver interceptors.
// Used by e2e tests that verify Auth, MetadataForwarding, Metrics, and
// StreamTimeout interceptors against a real in-process gRPC server.
package testapp

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	grpcserver "github.com/astra-go/astra/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

// InterceptorApp is a testapp variant whose gRPC server is built with the
// grpcserver interceptor helpers (Auth, MetadataForwarding, Metrics, etc.).
type InterceptorApp struct {
	HTTP     *httptest.Server
	GRPCAddr string
	Server   *grpcserver.Server // exposes SetReady / SetAlive
}

// claimsKey is the context key used by the test TokenValidator.
type claimsKey struct{}

// NewWithInterceptors creates an InterceptorApp wired with:
//   - UnaryInterceptorAuth  (BearerTokenExtractor + JWT validator)
//   - StreamInterceptorAuth (same)
//   - UnaryInterceptorMetadataForwarding (W3CHeaders)
//   - StreamInterceptorMetadataForwarding (W3CHeaders)
//   - UnaryInterceptorMetrics / StreamInterceptorMetrics
//   - StreamInterceptorTimeout (5 s)
func NewWithInterceptors(t testing.TB) *InterceptorApp {
	t.Helper()

	store := NewUserStore()
	astraApp := astra.New(astra.WithMode(astra.ModeTest))

	auth := astraApp.Group("/auth")
	auth.POST("/register", registerHandler(store))
	auth.POST("/login", loginHandler(store, TestJWTSecret))

	jwtMW := middleware.JWT(TestJWTSecret)
	api := astraApp.Group("/api", jwtMW)
	api.GET("/me", meHandler(store))

	httpSrv := httptest.NewServer(astraApp)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("interceptor testapp: gRPC listen: %v", err)
	}

	// JWT TokenValidator: validates the token and injects the subject into ctx.
	validator := grpcserver.TokenValidator(func(ctx context.Context, token string) (context.Context, error) {
		if err := validateJWT(token, TestJWTSecret); err != nil {
			return ctx, grpcserver.Unauthorized("INVALID_TOKEN", err.Error()).GRPCStatus().Err()
		}
		return context.WithValue(ctx, claimsKey{}, token), nil
	})

	srv := grpcserver.New(astraApp,
		grpcserver.WithGRPCAddr(lis.Addr().String()),
		grpcserver.WithUnaryInterceptors(
			grpcserver.UnaryInterceptorAuth(grpcserver.BearerTokenExtractor(), validator, grpcserver.SkipHealthCheck()),
			grpcserver.UnaryInterceptorMetadataForwarding(grpcserver.W3CHeaders...),
			grpcserver.UnaryInterceptorMetrics(grpcserver.GRPCMetricsConfig{}),
		),
		grpcserver.WithStreamInterceptors(
			grpcserver.StreamInterceptorAuth(grpcserver.BearerTokenExtractor(), validator, grpcserver.SkipHealthCheck()),
			grpcserver.StreamInterceptorMetadataForwarding(grpcserver.W3CHeaders...),
			grpcserver.StreamInterceptorMetrics(grpcserver.GRPCMetricsConfig{}),
			grpcserver.StreamInterceptorTimeout(5*1e9), // 5 s
		),
	)

	// Register the echo service on the grpcserver.Server's underlying gRPC server.
	echoSvc := NewEchoServicePassthrough() // auth is handled by interceptor, not service
	srv.GRPC.RegisterService(&EchoServiceDesc, echoSvc)

	// Also register the streaming echo service.
	srv.GRPC.RegisterService(&StreamEchoServiceDesc, NewStreamEchoService())

	// Start only the gRPC server (not the HTTP server — httptest.Server handles HTTP).
	go func() { _ = srv.GRPC.Serve(lis) }()

	app := &InterceptorApp{
		HTTP:     httpSrv,
		GRPCAddr: lis.Addr().String(),
		Server:   srv,
	}

	t.Cleanup(func() {
		srv.GRPC.GracefulStop()
		httpSrv.Close()
	})

	return app
}

// HTTPURL returns the base URL of the HTTP test server.
func (a *InterceptorApp) HTTPURL() string { return a.HTTP.URL }

// GRPCConn dials the gRPC server and returns a client connection.
func (a *InterceptorApp) GRPCConn(t testing.TB) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(a.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("interceptor testapp: grpc dial %s: %v", a.GRPCAddr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// ── Pass-through EchoService (auth delegated to interceptor) ─────────────────

// EchoServicePassthrough echoes without doing its own token check — the
// UnaryInterceptorAuth interceptor handles authentication before the handler runs.
type EchoServicePassthrough struct{}

func NewEchoServicePassthrough() *EchoServicePassthrough { return &EchoServicePassthrough{} }

func (s *EchoServicePassthrough) Echo(_ context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{Message: req.Message}, nil
}

// ── Streaming EchoService ─────────────────────────────────────────────────────

// StreamEchoRequest / StreamEchoResponse are the wire types for the streaming RPC.
type StreamEchoRequest struct{ Message string }
type StreamEchoResponse struct {
	Message    string
	ForwardedID string // echoes back the x-request-id forwarded via outgoing metadata
}

const StreamEchoServiceName = "testapp.StreamEchoService"

// StreamEchoService implements a server-streaming RPC for e2e tests.
type StreamEchoService struct{}

func NewStreamEchoService() *StreamEchoService { return &StreamEchoService{} }

// StreamEcho sends the request message back once, then reads the outgoing
// metadata to verify that MetadataForwarding populated it.
func (s *StreamEchoService) StreamEcho(req *StreamEchoRequest, stream grpc.ServerStream) error {
	// Read outgoing metadata that MetadataForwarding should have injected.
	var forwardedID string
	if md, ok := metadata.FromOutgoingContext(stream.Context()); ok {
		if vals := md.Get("traceparent"); len(vals) > 0 {
			forwardedID = vals[0]
		}
	}
	return stream.SendMsg(&StreamEchoResponse{
		Message:    req.Message,
		ForwardedID: forwardedID,
	})
}

// StreamEchoServiceDesc is the grpc.ServiceDesc for StreamEchoService.
var StreamEchoServiceDesc = grpc.ServiceDesc{
	ServiceName: StreamEchoServiceName,
	HandlerType: (*streamEchoServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamEcho",
			Handler:       streamEchoHandler,
			ServerStreams: true,
		},
	},
}

type streamEchoServer interface {
	StreamEcho(*StreamEchoRequest, grpc.ServerStream) error
}

func streamEchoHandler(srv any, stream grpc.ServerStream) error {
	req := new(StreamEchoRequest)
	if err := stream.RecvMsg(req); err != nil {
		return err
	}
	return srv.(streamEchoServer).StreamEcho(req, stream)
}

// HealthClient wraps grpc_health_v1 for probe tests.
func HealthCheck(t testing.TB, conn *grpc.ClientConn, service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	t.Helper()
	client := grpc_health_v1.NewHealthClient(conn)
	resp, err := client.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		t.Fatalf("health check %q: %v", service, err)
	}
	return resp.Status
}
