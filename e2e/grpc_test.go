package e2e_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/e2e/testapp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGRPC_Echo_Authenticated(t *testing.T) {
	app := testapp.New(t)

	// Register + login to get a token.
	token := registerAndLogin(t, app, "grpc_user", "grpcpass1")

	conn := app.GRPCConn(t)
	resp, err := grpcEcho(conn, "ping", token)
	if err != nil {
		t.Fatalf("grpc echo: %v", err)
	}
	if resp.Message != "ping" {
		t.Errorf("want 'ping', got %q", resp.Message)
	}
}

func TestGRPC_Echo_Unauthenticated(t *testing.T) {
	app := testapp.New(t)
	conn := app.GRPCConn(t)

	_, err := grpcEcho(conn, "ping", "")
	if err == nil {
		t.Fatal("expected Unauthenticated error")
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if s.Code() != codes.Unauthenticated {
		t.Errorf("want Unauthenticated, got %v", s.Code())
	}
}

func TestGRPC_Echo_InvalidToken(t *testing.T) {
	app := testapp.New(t)
	conn := app.GRPCConn(t)

	_, err := grpcEcho(conn, "ping", "not.a.valid.token")
	if err == nil {
		t.Fatal("expected Unauthenticated error for invalid token")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.Unauthenticated {
		t.Errorf("want Unauthenticated, got %v", s.Code())
	}
}

// grpcEcho calls EchoService.Echo using grpc.Invoke (no generated client needed).
// token may be empty to test unauthenticated calls.
func grpcEcho(conn *grpc.ClientConn, message, token string) (*testapp.EchoResponse, error) {
	req := &testapp.EchoRequest{Message: message}
	resp := &testapp.EchoResponse{}

	ctx := context.Background()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}

	err := conn.Invoke(ctx, "/"+testapp.EchoServiceName+"/Echo", req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
