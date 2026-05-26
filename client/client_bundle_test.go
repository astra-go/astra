package client

import (
	"context"
	"testing"
	"time"

	"github.com/astra-go/astra/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ─── GRPCClientBundle.Apply ───────────────────────────────────────────────────

func TestGRPCClientBundle_Apply_EmptyBundle_NoOptions(t *testing.T) {
	b := GRPCClientBundle{}
	opts := b.Apply()
	testutil.AssertEqual(t, 0, len(opts))
}

func TestGRPCClientBundle_Apply_PropagationOnly_TwoOptions(t *testing.T) {
	b := GRPCClientBundle{Propagation: []string{"traceparent"}}
	opts := b.Apply()
	// unary interceptors + stream interceptors = 2 options.
	testutil.AssertEqual(t, 2, len(opts))
}

func TestGRPCClientBundle_Apply_DeadlineOnly_OneOption(t *testing.T) {
	b := GRPCClientBundle{Deadline: 5 * time.Second}
	opts := b.Apply()
	// deadline only affects unary; no stream deadline interceptor.
	testutil.AssertEqual(t, 1, len(opts))
}

func TestGRPCClientBundle_Apply_BothFields_TwoOptions(t *testing.T) {
	b := GRPCClientBundle{
		Propagation: []string{"traceparent"},
		Deadline:    3 * time.Second,
	}
	opts := b.Apply()
	testutil.AssertEqual(t, 2, len(opts))
}

// ─── DefaultClientBundle ─────────────────────────────────────────────────────

func TestDefaultClientBundle_HasW3CHeaders(t *testing.T) {
	b := DefaultClientBundle()
	if len(b.Propagation) == 0 {
		t.Error("DefaultClientBundle: Propagation should be non-empty")
	}
	propSet := make(map[string]bool)
	for _, k := range b.Propagation {
		propSet[k] = true
	}
	for _, k := range []string{"traceparent", "tracestate", "baggage"} {
		if !propSet[k] {
			t.Errorf("DefaultClientBundle: missing W3C header %q", k)
		}
	}
}

func TestDefaultClientBundle_NoDeadline(t *testing.T) {
	b := DefaultClientBundle()
	testutil.AssertEqual(t, time.Duration(0), b.Deadline)
}

// ─── MeshClientBundle ────────────────────────────────────────────────────────

func TestMeshClientBundle_ContainsIstioAndW3CHeaders(t *testing.T) {
	b := MeshClientBundle()
	propSet := make(map[string]bool)
	for _, k := range b.Propagation {
		propSet[k] = true
	}
	for _, k := range append(istioHeaders, w3cHeaders...) {
		if !propSet[k] {
			t.Errorf("MeshClientBundle: missing header %q", k)
		}
	}
}

// ─── mesh propagation interceptor ────────────────────────────────────────────

// TestMeshPropagation_ForwardsIncomingToOutgoing verifies that the propagation
// interceptor copies keys from the incoming metadata in ctx to the outgoing
// metadata so downstream gRPC calls carry the headers.
func TestMeshPropagation_ForwardsIncomingToOutgoing(t *testing.T) {
	inMD := metadata.Pairs("traceparent", "00-trace-span-01", "x-request-id", "req-99")
	ctx := metadata.NewIncomingContext(context.Background(), inMD)

	var invokedCtx context.Context
	fakeInvoker := func(c context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokedCtx = c
		return nil
	}

	interceptor := meshPropagationUnaryInterceptor([]string{"traceparent", "x-request-id"})
	err := interceptor(ctx, "/svc/Method", nil, nil, nil, fakeInvoker)
	testutil.AssertNoError(t, err)

	outMD, ok := metadata.FromOutgoingContext(invokedCtx)
	if !ok {
		t.Fatal("no outgoing metadata after propagation interceptor")
	}
	vals := outMD.Get("traceparent")
	if len(vals) == 0 || vals[0] != "00-trace-span-01" {
		t.Errorf("traceparent: want %q, got %v", "00-trace-span-01", vals)
	}
	vals = outMD.Get("x-request-id")
	if len(vals) == 0 || vals[0] != "req-99" {
		t.Errorf("x-request-id: want %q, got %v", "req-99", vals)
	}
}

// TestMeshPropagation_NoIncomingMetadata_PassesThrough verifies that the
// interceptor is a no-op when the context carries no incoming metadata.
func TestMeshPropagation_NoIncomingMetadata_PassesThrough(t *testing.T) {
	interceptor := meshPropagationUnaryInterceptor([]string{"traceparent"})

	var invokedCtx context.Context
	fakeInvoker := func(c context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokedCtx = c
		return nil
	}

	err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, fakeInvoker)
	testutil.AssertNoError(t, err)

	if md, ok := metadata.FromOutgoingContext(invokedCtx); ok {
		if len(md.Get("traceparent")) > 0 {
			t.Error("traceparent should not be present when no incoming metadata")
		}
	}
}

// ─── deadline interceptor ─────────────────────────────────────────────────────

func TestDeadlineInterceptor_InjectsDeadlineWhenAbsent(t *testing.T) {
	interceptor := deadlineUnaryInterceptor(100 * time.Millisecond)

	var deadlineSet bool
	fakeInvoker := func(c context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		_, deadlineSet = c.Deadline()
		return nil
	}

	err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, fakeInvoker)
	testutil.AssertNoError(t, err)
	if !deadlineSet {
		t.Error("expected deadline to be injected, but context had none")
	}
}

func TestDeadlineInterceptor_DoesNotOverrideTighterDeadline(t *testing.T) {
	interceptor := deadlineUnaryInterceptor(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	originalDeadline, _ := ctx.Deadline()

	var capturedCtx context.Context
	fakeInvoker := func(c context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = c
		return nil
	}

	err := interceptor(ctx, "/svc/Method", nil, nil, nil, fakeInvoker)
	testutil.AssertNoError(t, err)

	got, ok := capturedCtx.Deadline()
	if !ok {
		t.Fatal("expected deadline in context")
	}
	if got.After(originalDeadline.Add(time.Millisecond)) {
		t.Errorf("deadline was extended: original %v, got %v", originalDeadline, got)
	}
}

// ─── pool construction with bundle ───────────────────────────────────────────

func TestNewGRPCPool_WithBundle_DoesNotPanic(t *testing.T) {
	b := MeshClientBundle()
	b.Deadline = 5 * time.Second
	pool := NewGRPCPool(b.Apply()...)
	if pool == nil {
		t.Error("NewGRPCPool returned nil")
	}
	_ = pool.Close()
}
