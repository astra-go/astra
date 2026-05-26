// middleware.go — Kratos-compatible middleware abstraction for Astra gRPC servers.
//
// Kratos defines middleware as a function that wraps a Handler:
//
//	type Handler    func(ctx context.Context, req any) (any, error)
//	type Middleware func(Handler) Handler
//
// This is more ergonomic than raw gRPC interceptors because middleware can be
// written once and reused across HTTP and gRPC without duplication.
//
// # Wiring to gRPC
//
// UnaryInterceptorMiddleware adapts a Middleware chain into a standard
// grpc.UnaryServerInterceptor so the two worlds compose cleanly:
//
//	s := grpcserver.New(app,
//	    grpcserver.WithUnaryInterceptors(
//	        grpcserver.UnaryInterceptorMiddleware(
//	            myAuthMiddleware,
//	            myLoggingMiddleware,
//	        ),
//	        grpcserver.UnaryInterceptorTracing(),
//	    ),
//	)
//
// # Execution order
//
// Chain(m1, m2, m3) executes in the order m1 → m2 → m3 → handler, i.e. the
// first middleware in the list is the outermost wrapper.
package grpcserver

import (
	"context"

	"google.golang.org/grpc"
)

// Handler is the unified handler signature used by Kratos-style middleware.
// It matches the shape of gRPC unary handlers after type erasure.
type Handler func(ctx context.Context, req any) (any, error)

// Middleware wraps a Handler, enabling cross-cutting concerns (auth, logging,
// tracing, rate-limiting, …) to be applied uniformly across all RPC methods.
type Middleware func(Handler) Handler

// Chain combines multiple Middleware into a single Middleware.
// Execution order is left-to-right: Chain(m1, m2, m3) runs m1 first,
// then m2, then m3, then the actual handler.
//
//	Chain(logging, auth, validate)(handler)
//	// → logging → auth → validate → handler → validate → auth → logging
func Chain(middlewares ...Middleware) Middleware {
	return func(next Handler) Handler {
		// Apply in reverse so that the first middleware in the slice is outermost.
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// UnaryInterceptorMiddleware converts a Middleware chain into a
// grpc.UnaryServerInterceptor.
//
// This lets you write middleware once using the Handler/Middleware abstraction
// and attach it to both the Kratos-style chain and the native gRPC interceptor
// chain without duplication.
//
// Example:
//
//	grpcserver.WithUnaryInterceptors(
//	    grpcserver.UnaryInterceptorMiddleware(authMiddleware, loggingMiddleware),
//	)
func UnaryInterceptorMiddleware(middlewares ...Middleware) grpc.UnaryServerInterceptor {
	chain := Chain(middlewares...)
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Wrap the native gRPC handler in a Handler-shaped function.
		h := func(ctx context.Context, req any) (any, error) {
			return handler(ctx, req)
		}
		return chain(h)(ctx, req)
	}
}

// StreamInterceptorMiddleware converts a Middleware chain into a
// grpc.StreamServerInterceptor.
//
// Because streaming RPCs have no single request value, the Handler receives nil
// as req and returns nil as response. Middleware in the chain can inspect the
// context (metadata, peer info, auth tokens injected by prior interceptors) and
// abort the stream early by returning an error before the handler is called.
//
// Any context modifications made by middleware are propagated into the streaming
// handler via a context-carrying ServerStream wrapper, so the handler's
// stream.Context() reflects the enriched context.
//
// Example:
//
//	grpcserver.WithStreamInterceptors(
//	    grpcserver.StreamInterceptorMiddleware(authMiddleware, rateLimitMiddleware),
//	)
func StreamInterceptorMiddleware(middlewares ...Middleware) grpc.StreamServerInterceptor {
	chain := Chain(middlewares...)
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		h := func(ctx context.Context, _ any) (any, error) {
			return nil, handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})
		}
		_, err := chain(h)(ss.Context(), nil)
		return err
	}
}
