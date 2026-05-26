package grpcserver_test

import (
	"context"
	"errors"
	"net"
	"testing"

	grpcserver "github.com/astra-go/astra/grpc"
	"github.com/astra-go/astra/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// peerCtx returns a context carrying a fake peer at the given IP.
func peerCtx(ip string) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 12345},
	})
}

// ─── UnaryInterceptorRateLimit ────────────────────────────────────────────────

func TestUnaryInterceptorRateLimit_AllowsWithinBurst(t *testing.T) {
	interceptor := grpcserver.UnaryInterceptorRateLimit(100, 3)
	ctx := peerCtx("10.0.0.1")
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{}

	for i := 0; i < 3; i++ {
		resp, err := interceptor(ctx, nil, info, handler)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, "ok", resp)
	}
}

func TestUnaryInterceptorRateLimit_RejectsWhenExhausted(t *testing.T) {
	interceptor := grpcserver.UnaryInterceptorRateLimit(100, 1)
	ctx := peerCtx("10.0.0.2")
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{}

	// Consume the single burst token.
	_, err := interceptor(ctx, nil, info, handler)
	testutil.AssertNoError(t, err)

	// Next call must be rejected with ResourceExhausted.
	_, err = interceptor(ctx, nil, info, handler)
	if err == nil {
		t.Fatal("expected rate-limit error, got nil")
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	testutil.AssertEqual(t, codes.ResourceExhausted, s.Code())
}

func TestUnaryInterceptorRateLimit_SeparateBucketsPerPeer(t *testing.T) {
	interceptor := grpcserver.UnaryInterceptorRateLimit(100, 1)
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{}

	// Exhaust peer A's bucket.
	ctxA := peerCtx("10.0.0.10")
	interceptor(ctxA, nil, info, handler) //nolint — consume burst
	_, err := interceptor(ctxA, nil, info, handler)
	if err == nil {
		t.Fatal("peer A: second call should be rate limited")
	}

	// Peer B must still have its own fresh bucket.
	ctxB := peerCtx("10.0.0.11")
	_, err = interceptor(ctxB, nil, info, handler)
	testutil.AssertNoError(t, err)
}

// ─── StreamInterceptorRateLimit ───────────────────────────────────────────────

// mockServerStream is a minimal grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

func TestStreamInterceptorRateLimit_AllowsWithinBurst(t *testing.T) {
	interceptor := grpcserver.StreamInterceptorRateLimit(100, 2)
	ss := &mockServerStream{ctx: peerCtx("10.1.0.1")}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }
	info := &grpc.StreamServerInfo{}

	testutil.AssertNoError(t, interceptor(nil, ss, info, handler))
	testutil.AssertNoError(t, interceptor(nil, ss, info, handler))
}

func TestStreamInterceptorRateLimit_RejectsWhenExhausted(t *testing.T) {
	interceptor := grpcserver.StreamInterceptorRateLimit(100, 1)
	ss := &mockServerStream{ctx: peerCtx("10.1.0.2")}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }
	info := &grpc.StreamServerInfo{}

	// Consume burst.
	testutil.AssertNoError(t, interceptor(nil, ss, info, handler))

	// Next stream setup must be rejected.
	err := interceptor(nil, ss, info, handler)
	if err == nil {
		t.Fatal("expected rate-limit error, got nil")
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	testutil.AssertEqual(t, codes.ResourceExhausted, s.Code())
}

// ─── StreamInterceptorMiddleware ─────────────────────────────────────────────

func TestStreamInterceptorMiddleware_CallsChain(t *testing.T) {
	var called bool
	mw := func(next grpcserver.Handler) grpcserver.Handler {
		return func(ctx context.Context, req any) (any, error) {
			called = true
			return next(ctx, req)
		}
	}

	interceptor := grpcserver.StreamInterceptorMiddleware(mw)
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	ss := &mockServerStream{ctx: context.Background()}
	err := interceptor(nil, ss, &grpc.StreamServerInfo{}, handler)
	testutil.AssertNoError(t, err)
	if !called {
		t.Error("middleware was not called for stream interceptor")
	}
}

func TestStreamInterceptorMiddleware_ErrorPropagation(t *testing.T) {
	want := errors.New("stream auth failed")
	mw := func(_ grpcserver.Handler) grpcserver.Handler {
		return func(_ context.Context, _ any) (any, error) {
			return nil, want
		}
	}

	interceptor := grpcserver.StreamInterceptorMiddleware(mw)
	ss := &mockServerStream{ctx: context.Background()}
	err := interceptor(nil, ss, &grpc.StreamServerInfo{},
		func(_ any, _ grpc.ServerStream) error { return nil })

	testutil.AssertErrorIs(t, err, want)
}

func TestStreamInterceptorMiddleware_PropagatesContextToHandler(t *testing.T) {
	type ctxKey struct{}

	mw := func(next grpcserver.Handler) grpcserver.Handler {
		return func(ctx context.Context, req any) (any, error) {
			// Inject a value into the context before calling the handler.
			return next(context.WithValue(ctx, ctxKey{}, "injected"), req)
		}
	}

	var gotValue any
	handler := func(_ any, ss grpc.ServerStream) error {
		gotValue = ss.Context().Value(ctxKey{})
		return nil
	}

	interceptor := grpcserver.StreamInterceptorMiddleware(mw)
	ss := &mockServerStream{ctx: context.Background()}
	testutil.AssertNoError(t, interceptor(nil, ss, &grpc.StreamServerInfo{}, handler))

	if gotValue != "injected" {
		t.Errorf("expected injected context value in stream handler, got %v", gotValue)
	}
}

func TestStreamInterceptorMiddleware_ChainOrder(t *testing.T) {
	var order []string
	mkMW := func(label string) grpcserver.Middleware {
		return func(next grpcserver.Handler) grpcserver.Handler {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, label+" before")
				resp, err := next(ctx, req)
				order = append(order, label+" after")
				return resp, err
			}
		}
	}

	interceptor := grpcserver.StreamInterceptorMiddleware(mkMW("m1"), mkMW("m2"))
	ss := &mockServerStream{ctx: context.Background()}
	grpcserver.StreamInterceptorMiddleware(mkMW("m1"), mkMW("m2"))

	err := interceptor(nil, ss, &grpc.StreamServerInfo{},
		func(_ any, _ grpc.ServerStream) error { return nil })
	testutil.AssertNoError(t, err)

	expected := []string{"m1 before", "m2 before", "m2 after", "m1 after"}
	if len(order) != len(expected) {
		t.Fatalf("order len: want %d, got %d: %v", len(expected), len(order), order)
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("step %d: want %q, got %q", i, want, order[i])
		}
	}
}
