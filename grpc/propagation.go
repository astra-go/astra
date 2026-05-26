// propagation.go — metadata forwarding interceptors for service mesh environments.
//
// In Istio/Envoy deployments each service must explicitly forward correlation
// headers (x-request-id, x-b3-*, traceparent, …) on every outbound call.
// These interceptors copy the specified keys from the incoming gRPC metadata
// to the outgoing context so that downstream calls made inside the handler
// automatically carry the headers.
//
// Usage:
//
//	grpcserver.WithUnaryInterceptors(
//	    grpcserver.UnaryInterceptorMetadataForwarding(grpcserver.IstioHeaders...),
//	)
package grpcserver

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Predefined header sets for common service mesh environments.
// Pass these (or a subset) to UnaryInterceptorMetadataForwarding /
// StreamInterceptorMetadataForwarding.
var (
	// EnvoyHeaders are the standard headers propagated by Envoy proxy.
	EnvoyHeaders = []string{
		"x-request-id",
		"x-envoy-attempt-count",
		"x-forwarded-for",
		"x-forwarded-proto",
		"x-envoy-expected-rq-timeout-ms",
	}

	// IstioHeaders are the headers required for Istio distributed tracing
	// (Zipkin B3 format).
	IstioHeaders = []string{
		"x-request-id",
		"x-b3-traceid",
		"x-b3-spanid",
		"x-b3-parentspanid",
		"x-b3-sampled",
		"x-b3-flags",
	}

	// W3CHeaders are the W3C Trace Context headers used by OpenTelemetry.
	W3CHeaders = []string{
		"traceparent",
		"tracestate",
		"baggage",
	}
)

// UnaryInterceptorMetadataForwarding returns a gRPC unary server interceptor
// that copies the specified metadata keys from the incoming context to the
// outgoing context, enabling transparent header propagation across service hops.
//
// Keys absent from the incoming metadata are silently skipped.
// Existing outgoing metadata is preserved; forwarded keys are appended.
//
// Example — forward all Istio tracing headers:
//
//	grpcserver.WithUnaryInterceptors(
//	    grpcserver.UnaryInterceptorMetadataForwarding(grpcserver.IstioHeaders...),
//	)
func UnaryInterceptorMetadataForwarding(keys ...string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		return handler(forwardMetadata(ctx, keys), req)
	}
}

// StreamInterceptorMetadataForwarding returns a gRPC stream server interceptor
// that copies the specified metadata keys from the incoming context to the
// outgoing context at stream setup time.
func StreamInterceptorMetadataForwarding(keys ...string) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := forwardMetadata(ss.Context(), keys)
		return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})
	}
}

// forwardMetadata copies the given keys from the incoming metadata to the
// outgoing metadata in ctx via metadata.AppendToOutgoingContext, which
// preserves any existing outgoing metadata.
func forwardMetadata(ctx context.Context, keys []string) context.Context {
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
