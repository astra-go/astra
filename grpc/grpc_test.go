package grpcserver_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	grpcserver "github.com/astra-go/astra/grpc"
	"github.com/astra-go/astra/testutil"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── Error constructors ───────────────────────────────────────────────────────

func TestErrorConstructors(t *testing.T) {
	tests := []struct {
		name     string
		err      *grpcserver.Error
		wantCode int32
		wantHTTP codes.Code
	}{
		{"BadRequest", grpcserver.BadRequest("INVALID", "bad"), 400, codes.InvalidArgument},
		{"Unauthorized", grpcserver.Unauthorized("NO_AUTH", "unauth"), 401, codes.Unauthenticated},
		{"Forbidden", grpcserver.Forbidden("FORBIDDEN", "forbidden"), 403, codes.PermissionDenied},
		{"NotFound", grpcserver.NotFound("NOT_FOUND", "not found"), 404, codes.NotFound},
		{"Conflict", grpcserver.Conflict("CONFLICT", "conflict"), 409, codes.AlreadyExists},
		{"TooManyRequests", grpcserver.TooManyRequests("RATE_LIMIT", "slow down"), 429, codes.ResourceExhausted},
		{"InternalServer", grpcserver.InternalServer("INTERNAL", "oops"), 500, codes.Internal},
		{"NotImplemented", grpcserver.NotImplemented("NOT_IMPL", "todo"), 501, codes.Unimplemented},
		{"ServiceUnavailable", grpcserver.ServiceUnavailable("UNAVAIL", "down"), 503, codes.Unavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testutil.AssertEqual(t, tc.wantCode, tc.err.Code)
			// Verify gRPC status code mapping.
			s := tc.err.GRPCStatus()
			testutil.AssertEqual(t, tc.wantHTTP, s.Code())
		})
	}
}

// ─── Error.Error() ────────────────────────────────────────────────────────────

func TestError_ErrorString(t *testing.T) {
	e := grpcserver.NotFound("USER_NOT_FOUND", "user does not exist")
	msg := e.Error()
	if msg == "" {
		t.Error("Error() should return non-empty string")
	}
	if !containsAny(msg, "404", "USER_NOT_FOUND") {
		t.Errorf("Error() = %q; expected to contain code or reason", msg)
	}
}

// ─── GRPCStatus encoding ─────────────────────────────────────────────────────

func TestError_GRPCStatus_EncodesErrorInfo(t *testing.T) {
	e := grpcserver.BadRequest("INVALID_FIELD", "name is required")
	s := e.GRPCStatus()

	testutil.AssertEqual(t, codes.InvalidArgument, s.Code())
	testutil.AssertEqual(t, "name is required", s.Message())

	details := s.Details()
	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}
	info, ok := details[0].(*errdetails.ErrorInfo)
	if !ok {
		t.Fatalf("expected *errdetails.ErrorInfo, got %T", details[0])
	}
	testutil.AssertEqual(t, "INVALID_FIELD", info.Reason)
}

func TestError_GRPCStatus_MetadataInDetail(t *testing.T) {
	e := grpcserver.NotFound("ITEM_NOT_FOUND", "item missing").
		WithMetadata(map[string]string{"item_id": "42"})

	s := e.GRPCStatus()
	details := s.Details()
	if len(details) == 0 {
		t.Fatal("expected details")
	}
	info := details[0].(*errdetails.ErrorInfo)
	testutil.AssertEqual(t, "42", info.Metadata["item_id"])
}

// ─── Error.Is ────────────────────────────────────────────────────────────────

func TestError_Is_SameCodeAndReason(t *testing.T) {
	sentinel := grpcserver.NotFound("USER_NOT_FOUND", "")
	wrapped := grpcserver.NotFound("USER_NOT_FOUND", "user 42 not found")

	if !errors.Is(wrapped, sentinel) {
		t.Error("errors.Is: same code+reason should match")
	}
}

func TestError_Is_DifferentReason_DoesNotMatch(t *testing.T) {
	a := grpcserver.NotFound("FOO", "")
	b := grpcserver.NotFound("BAR", "")
	if errors.Is(a, b) {
		t.Error("errors.Is: different Reason should not match")
	}
}

func TestError_Is_DifferentCode_DoesNotMatch(t *testing.T) {
	a := grpcserver.NotFound("ERR", "")
	b := grpcserver.BadRequest("ERR", "")
	if errors.Is(a, b) {
		t.Error("errors.Is: different Code should not match")
	}
}

// ─── WithMetadata ─────────────────────────────────────────────────────────────

func TestError_WithMetadata_DoesNotMutateOriginal(t *testing.T) {
	orig := grpcserver.InternalServer("INTERNAL", "error")
	clone := orig.WithMetadata(map[string]string{"request_id": "xyz"})

	if orig.Metadata != nil {
		t.Error("WithMetadata should not mutate the original error")
	}
	testutil.AssertEqual(t, "xyz", clone.Metadata["request_id"])
}

// ─── FromError ────────────────────────────────────────────────────────────────

func TestFromError_WithStarError(t *testing.T) {
	orig := grpcserver.NotFound("X", "not found")
	e := grpcserver.FromError(orig)
	testutil.AssertEqual(t, orig.Code, e.Code)
	testutil.AssertEqual(t, orig.Reason, e.Reason)
}

func TestFromError_WithGRPCStatusError(t *testing.T) {
	// Build a gRPC status error with ErrorInfo detail (as produced by GRPCStatus().Err()).
	orig := grpcserver.Forbidden("PERM_DENIED", "access denied")
	grpcErr := orig.GRPCStatus().Err()

	e := grpcserver.FromError(grpcErr)
	testutil.AssertEqual(t, int32(http.StatusForbidden), e.Code)
	testutil.AssertEqual(t, "PERM_DENIED", e.Reason)
}

func TestFromError_WithPlainError_Wraps500(t *testing.T) {
	plain := errors.New("connection refused")
	e := grpcserver.FromError(plain)
	testutil.AssertEqual(t, int32(http.StatusInternalServerError), e.Code)
}

func TestFromError_WithBareGRPCStatus_NoErrorInfo(t *testing.T) {
	// A plain gRPC status without ErrorInfo detail.
	grpcErr := status.Error(codes.NotFound, "resource gone")
	e := grpcserver.FromError(grpcErr)
	testutil.AssertEqual(t, int32(http.StatusNotFound), e.Code)
}

func TestFromError_NilReturnsNil(t *testing.T) {
	if grpcserver.FromError(nil) != nil {
		t.Error("FromError(nil) should return nil")
	}
}

// ─── Is* helpers ─────────────────────────────────────────────────────────────

func TestIsHelpers(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		fn     func(error) bool
		wantOK bool
	}{
		{"IsBadRequest true", grpcserver.BadRequest("X", "x"), grpcserver.IsBadRequest, true},
		{"IsBadRequest false", grpcserver.NotFound("X", "x"), grpcserver.IsBadRequest, false},
		{"IsUnauthorized", grpcserver.Unauthorized("X", "x"), grpcserver.IsUnauthorized, true},
		{"IsForbidden", grpcserver.Forbidden("X", "x"), grpcserver.IsForbidden, true},
		{"IsNotFound", grpcserver.NotFound("X", "x"), grpcserver.IsNotFound, true},
		{"IsConflict", grpcserver.Conflict("X", "x"), grpcserver.IsConflict, true},
		{"IsTooManyRequests", grpcserver.TooManyRequests("X", "x"), grpcserver.IsTooManyRequests, true},
		{"IsInternalServer", grpcserver.InternalServer("X", "x"), grpcserver.IsInternalServer, true},
		{"IsServiceUnavailable", grpcserver.ServiceUnavailable("X", "x"), grpcserver.IsServiceUnavailable, true},
		{"IsNotFound with plain error", errors.New("plain"), grpcserver.IsNotFound, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(tc.err)
			testutil.AssertEqual(t, tc.wantOK, got)
		})
	}
}

// ─── Middleware Chain ─────────────────────────────────────────────────────────

func TestChain_ExecutionOrder(t *testing.T) {
	var order []string

	newMiddleware := func(label string) grpcserver.Middleware {
		return func(next grpcserver.Handler) grpcserver.Handler {
			return func(ctx context.Context, req any) (any, error) {
				order = append(order, label+" before")
				resp, err := next(ctx, req)
				order = append(order, label+" after")
				return resp, err
			}
		}
	}

	handler := func(_ context.Context, _ any) (any, error) {
		order = append(order, "handler")
		return "ok", nil
	}

	chain := grpcserver.Chain(newMiddleware("m1"), newMiddleware("m2"), newMiddleware("m3"))
	_, err := chain(handler)(context.Background(), nil)
	testutil.AssertNoError(t, err)

	expected := []string{
		"m1 before", "m2 before", "m3 before",
		"handler",
		"m3 after", "m2 after", "m1 after",
	}
	if len(order) != len(expected) {
		t.Fatalf("order len: want %d, got %d: %v", len(expected), len(order), order)
	}
	for i, want := range expected {
		if order[i] != want {
			t.Errorf("step %d: want %q, got %q", i, want, order[i])
		}
	}
}

func TestChain_Empty_CallsHandlerDirectly(t *testing.T) {
	chain := grpcserver.Chain()
	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return nil, nil
	}
	_, err := chain(handler)(context.Background(), nil)
	testutil.AssertNoError(t, err)
	if !called {
		t.Error("handler should have been called directly")
	}
}

// ─── UnaryInterceptorMiddleware ───────────────────────────────────────────────

func TestUnaryInterceptorMiddleware_CallsChain(t *testing.T) {
	var called bool
	mw := func(next grpcserver.Handler) grpcserver.Handler {
		return func(ctx context.Context, req any) (any, error) {
			called = true
			return next(ctx, req)
		}
	}

	interceptor := grpcserver.UnaryInterceptorMiddleware(mw)
	handler := func(_ context.Context, _ any) (any, error) {
		return "result", nil
	}

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, handler)
	testutil.AssertNoError(t, err)
	if !called {
		t.Error("middleware was not called")
	}
	testutil.AssertEqual(t, "result", resp)
}

func TestUnaryInterceptorMiddleware_ErrorPropagation(t *testing.T) {
	want := errors.New("handler error")
	interceptor := grpcserver.UnaryInterceptorMiddleware()
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, want
	}

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	testutil.AssertErrorIs(t, err, want)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
