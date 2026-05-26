package e2e_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/astra-go/astra/e2e/testapp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ── UnaryInterceptorAuth ──────────────────────────────────────────────────────

// TestGRPC_Auth_Unary_Valid verifies that a valid JWT passes UnaryInterceptorAuth.
func TestGRPC_Auth_Unary_Valid(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	token := registerAndLoginURL(t, app.HTTP.Client(), app.HTTPURL(), "auth_unary_ok", "pass1234")

	conn := app.GRPCConn(t)
	resp, err := grpcEcho(conn, "hello", token)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if resp.Message != "hello" {
		t.Errorf("want 'hello', got %q", resp.Message)
	}
}

// TestGRPC_Auth_Unary_Missing verifies that a missing token returns Unauthenticated.
func TestGRPC_Auth_Unary_Missing(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	conn := app.GRPCConn(t)

	_, err := grpcEcho(conn, "hello", "")
	assertGRPCCode(t, err, codes.Unauthenticated)
}

// TestGRPC_Auth_Unary_Invalid verifies that a malformed token returns Unauthenticated.
func TestGRPC_Auth_Unary_Invalid(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	conn := app.GRPCConn(t)

	_, err := grpcEcho(conn, "hello", "not.a.valid.jwt")
	assertGRPCCode(t, err, codes.Unauthenticated)
}

// ── StreamInterceptorAuth ─────────────────────────────────────────────────────

// TestGRPC_Auth_Stream_Valid verifies that a valid JWT passes StreamInterceptorAuth.
func TestGRPC_Auth_Stream_Valid(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	token := registerAndLoginURL(t, app.HTTP.Client(), app.HTTPURL(), "auth_stream_ok", "pass1234")

	conn := app.GRPCConn(t)
	resp, err := grpcStreamEcho(conn, "stream-hello", token)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if resp.Message != "stream-hello" {
		t.Errorf("want 'stream-hello', got %q", resp.Message)
	}
}

// TestGRPC_Auth_Stream_Missing verifies that a missing token rejects the stream.
func TestGRPC_Auth_Stream_Missing(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	conn := app.GRPCConn(t)

	_, err := grpcStreamEcho(conn, "hello", "")
	assertGRPCCode(t, err, codes.Unauthenticated)
}

// ── UnaryInterceptorMetadataForwarding ────────────────────────────────────────

// TestGRPC_MetadataForwarding_Unary verifies that W3C headers sent by the client
// are forwarded into the outgoing context by UnaryInterceptorMetadataForwarding.
func TestGRPC_MetadataForwarding_Unary(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	token := registerAndLoginURL(t, app.HTTP.Client(), app.HTTPURL(), "fwd_unary", "pass1234")

	conn := app.GRPCConn(t)
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"authorization", "Bearer "+token,
		"traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"tracestate", "vendor=value",
	)
	req := &testapp.EchoRequest{Message: "fwd-test"}
	resp := &testapp.EchoResponse{}
	if err := conn.Invoke(ctx, "/"+testapp.EchoServiceName+"/Echo", req, resp); err != nil {
		t.Fatalf("expected success with forwarded headers, got: %v", err)
	}
	if resp.Message != "fwd-test" {
		t.Errorf("want 'fwd-test', got %q", resp.Message)
	}
}

// TestGRPC_MetadataForwarding_Stream verifies that W3C headers are forwarded
// into the outgoing context for streaming RPCs. The StreamEchoService reads
// the outgoing traceparent and echoes it back in ForwardedID.
func TestGRPC_MetadataForwarding_Stream(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	token := registerAndLoginURL(t, app.HTTP.Client(), app.HTTPURL(), "fwd_stream", "pass1234")

	conn := app.GRPCConn(t)
	const traceParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	resp, err := grpcStreamEchoWithTrace(conn, "fwd-stream-test", token, traceParent)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if resp.Message != "fwd-stream-test" {
		t.Errorf("want 'fwd-stream-test', got %q", resp.Message)
	}
	if resp.ForwardedID != traceParent {
		t.Errorf("traceparent not forwarded: want %q, got %q", traceParent, resp.ForwardedID)
	}
}

// ── StreamInterceptorTimeout ──────────────────────────────────────────────────

// TestGRPC_StreamTimeout_WithinLimit verifies that a fast stream completes
// successfully within the 5 s timeout configured in NewWithInterceptors.
func TestGRPC_StreamTimeout_WithinLimit(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	token := registerAndLoginURL(t, app.HTTP.Client(), app.HTTPURL(), "timeout_ok", "pass1234")

	conn := app.GRPCConn(t)
	resp, err := grpcStreamEcho(conn, "fast", token)
	if err != nil {
		t.Fatalf("expected success within timeout, got: %v", err)
	}
	if resp.Message != "fast" {
		t.Errorf("want 'fast', got %q", resp.Message)
	}
}

// ── SetReady / SetAlive ───────────────────────────────────────────────────────

// TestGRPC_HealthProbes_InitiallyServing verifies that both readiness and
// liveness probes report SERVING immediately after server start.
func TestGRPC_HealthProbes_InitiallyServing(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	conn := app.GRPCConn(t)

	for _, svc := range []string{"readiness", "liveness"} {
		st := testapp.HealthCheck(t, conn, svc)
		if st != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("probe %q: want SERVING, got %v", svc, st)
		}
	}
}

// TestGRPC_HealthProbes_SetReady verifies that SetReady(false) flips the
// readiness probe to NOT_SERVING without affecting liveness.
func TestGRPC_HealthProbes_SetReady(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	conn := app.GRPCConn(t)

	app.Server.SetReady(false)

	if st := testapp.HealthCheck(t, conn, "readiness"); st != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("readiness after SetReady(false): want NOT_SERVING, got %v", st)
	}
	if st := testapp.HealthCheck(t, conn, "liveness"); st != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("liveness after SetReady(false): want SERVING, got %v", st)
	}

	app.Server.SetReady(true)
	if st := testapp.HealthCheck(t, conn, "readiness"); st != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("readiness after SetReady(true): want SERVING, got %v", st)
	}
}

// TestGRPC_HealthProbes_SetAlive verifies that SetAlive(false) flips the
// liveness probe to NOT_SERVING without affecting readiness.
func TestGRPC_HealthProbes_SetAlive(t *testing.T) {
	app := testapp.NewWithInterceptors(t)
	conn := app.GRPCConn(t)

	app.Server.SetAlive(false)

	if st := testapp.HealthCheck(t, conn, "liveness"); st != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("liveness after SetAlive(false): want NOT_SERVING, got %v", st)
	}
	if st := testapp.HealthCheck(t, conn, "readiness"); st != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Errorf("readiness after SetAlive(false): want SERVING, got %v", st)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// grpcStreamEcho calls StreamEchoService.StreamEcho and returns the first response.
func grpcStreamEcho(conn *grpc.ClientConn, message, token string) (*testapp.StreamEchoResponse, error) {
	return grpcStreamEchoWithTrace(conn, message, token, "")
}

func grpcStreamEchoWithTrace(conn *grpc.ClientConn, message, token, traceParent string) (*testapp.StreamEchoResponse, error) {
	ctx := context.Background()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	if traceParent != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "traceparent", traceParent)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	sd := grpc.StreamDesc{ServerStreams: true}
	stream, err := conn.NewStream(ctx, &sd, "/"+testapp.StreamEchoServiceName+"/StreamEcho")
	if err != nil {
		return nil, err
	}
	if err := stream.SendMsg(&testapp.StreamEchoRequest{Message: message}); err != nil {
		return nil, err
	}
	if err := stream.CloseSend(); err != nil {
		return nil, err
	}
	resp := &testapp.StreamEchoResponse{}
	if err := stream.RecvMsg(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func assertGRPCCode(t testing.TB, err error, want codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %v, got nil", want)
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if s.Code() != want {
		t.Errorf("want gRPC code %v, got %v", want, s.Code())
	}
}

// Ensure http is used (imported for registerAndLoginURL calls via suite_test.go helpers).
var _ *http.Client
