// client_bundle.go — opinionated client-side interceptor bundle for GRPCPool.
//
// GRPCClientBundle mirrors ServerBundle on the client side: one struct that
// wires tracing, outbound mesh-header propagation, and deadline injection into
// every connection the pool dials.
//
//	pool := client.NewGRPCPool(
//	    client.DefaultClientBundle().Apply()...,
//	    client.WithGRPCRegistry(reg),
//	)
//
// # Outbound mesh-header propagation
//
// In Istio/Envoy deployments each service must forward correlation headers on
// every outbound call. GRPCClientBundle.Propagation copies the listed keys from
// the incoming gRPC metadata already present in ctx to the outgoing metadata,
// so downstream services receive the full trace context without any manual work.
//
// # Deadline propagation
//
// When Deadline > 0 and the outgoing context has no deadline, the interceptor
// injects one. If the context already carries a tighter deadline, it is left
// unchanged — the interceptor never extends a deadline.
package client

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// GRPCClientBundle is an opinionated set of client-side interceptors for
// GRPCPool. Configure the fields you need and call Apply() to get a slice of
// GRPCOptions ready to pass to NewGRPCPool.
//
//	bundle := client.DefaultClientBundle()
//	bundle.Deadline = 5 * time.Second
//	pool := client.NewGRPCPool(bundle.Apply()...)
type GRPCClientBundle struct {
	// Propagation lists metadata keys to copy from the incoming context to the
	// outgoing context on every outbound call (mesh-header forwarding).
	// nil = no forwarding.
	// Use grpcserver.IstioHeaders, grpcserver.W3CHeaders, etc. as starting points,
	// or supply your own key list.
	Propagation []string
	// Deadline injects a per-call deadline when the context has none.
	// 0 = no deadline injection (default).
	Deadline time.Duration
}

// DefaultClientBundle returns a bundle suitable for most services:
// W3C TraceContext header propagation, no deadline injection.
func DefaultClientBundle() GRPCClientBundle {
	return GRPCClientBundle{
		Propagation: w3cHeaders,
	}
}

// MeshClientBundle returns a bundle tuned for Istio / Envoy deployments:
// Istio B3 + W3C header propagation, no deadline injection.
func MeshClientBundle() GRPCClientBundle {
	keys := make([]string, 0, len(istioHeaders)+len(w3cHeaders))
	keys = append(keys, istioHeaders...)
	keys = append(keys, w3cHeaders...)
	return GRPCClientBundle{
		Propagation: keys,
	}
}

// Apply converts the bundle into a slice of GRPCOptions ready to pass to
// NewGRPCPool. Interceptors are registered in the order:
//
//	Propagation → Deadline
//
// Propagation runs first so the deadline interceptor sees the enriched context.
func (b GRPCClientBundle) Apply() []GRPCOption {
	var unary []grpc.UnaryClientInterceptor
	var stream []grpc.StreamClientInterceptor

	if len(b.Propagation) > 0 {
		unary = append(unary, meshPropagationUnaryInterceptor(b.Propagation))
		stream = append(stream, meshPropagationStreamInterceptor(b.Propagation))
	}

	if b.Deadline > 0 {
		unary = append(unary, deadlineUnaryInterceptor(b.Deadline))
	}

	var opts []GRPCOption
	if len(unary) > 0 {
		opts = append(opts, WithGRPCDialOptions(grpc.WithChainUnaryInterceptor(unary...)))
	}
	if len(stream) > 0 {
		opts = append(opts, WithGRPCDialOptions(grpc.WithChainStreamInterceptor(stream...)))
	}
	return opts
}

// ─── header key sets (mirrors grpcserver package, kept local to avoid import) ─

var (
	istioHeaders = []string{
		"x-request-id",
		"x-b3-traceid",
		"x-b3-spanid",
		"x-b3-parentspanid",
		"x-b3-sampled",
		"x-b3-flags",
	}
	w3cHeaders = []string{
		"traceparent",
		"tracestate",
		"baggage",
	}
)

// ─── outbound mesh-header propagation ────────────────────────────────────────

// meshPropagationUnaryInterceptor copies keys from the incoming metadata
// already in ctx to the outgoing metadata, enabling transparent header
// forwarding across service hops.
func meshPropagationUnaryInterceptor(keys []string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		return invoker(forwardToOutgoing(ctx, keys), method, req, reply, cc, opts...)
	}
}

// meshPropagationStreamInterceptor copies keys from incoming to outgoing
// metadata at stream setup time.
func meshPropagationStreamInterceptor(keys []string) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(forwardToOutgoing(ctx, keys), desc, cc, method, opts...)
	}
}

// forwardToOutgoing reads keys from the incoming metadata in ctx and appends
// them to the outgoing metadata. Existing outgoing metadata is preserved.
// Keys absent from the incoming metadata are silently skipped.
func forwardToOutgoing(ctx context.Context, keys []string) context.Context {
	if len(keys) == 0 {
		return ctx
	}
	incoming, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	var pairs []string
	for _, key := range keys {
		for _, v := range incoming.Get(key) {
			pairs = append(pairs, key, v)
		}
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

// ─── deadline propagation ─────────────────────────────────────────────────────

// deadlineUnaryInterceptor injects a deadline of d when the context has none.
// If the context already carries a tighter deadline, it is left unchanged.
func deadlineUnaryInterceptor(d time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
