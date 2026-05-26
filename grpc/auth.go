// auth.go — pluggable authentication interceptors for Astra gRPC servers.
//
// Usage:
//
//	grpcserver.WithUnaryInterceptors(
//	    grpcserver.UnaryInterceptorAuth(
//	        grpcserver.BearerTokenExtractor(),
//	        func(ctx context.Context, token string) (context.Context, error) {
//	            claims, err := myJWT.Parse(token)
//	            if err != nil {
//	                return ctx, grpcserver.Unauthorized("INVALID_TOKEN", err.Error()).GRPCStatus().Err()
//	            }
//	            return context.WithValue(ctx, claimsKey{}, claims), nil
//	        },
//	        grpcserver.SkipHealthCheck(), // allow unauthenticated health probes
//	    ),
//	)
package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// TokenExtractor extracts a bearer token from the incoming gRPC context.
// Return a gRPC status error (e.g. via Unauthorized(...).GRPCStatus().Err())
// to abort the request with a structured error.
type TokenExtractor func(ctx context.Context) (string, error)

// TokenValidator validates the extracted token and returns an enriched context.
// Inject auth claims, user IDs, or roles into the returned context so that
// handlers and downstream middleware can read them via context.Value.
// Return a gRPC status error to reject the request.
type TokenValidator func(ctx context.Context, token string) (context.Context, error)

// AuthSkipper decides whether to skip authentication for a given full method name
// (e.g. "/grpc.health.v1.Health/Check"). Return true to bypass auth.
type AuthSkipper func(fullMethod string) bool

// SkipHealthCheck returns an AuthSkipper that bypasses auth for all
// grpc.health.v1.Health methods, allowing unauthenticated liveness/readiness probes.
func SkipHealthCheck() AuthSkipper {
	return func(fullMethod string) bool {
		return strings.HasPrefix(fullMethod, "/grpc.health.v1.Health/")
	}
}

// SkipMethods returns an AuthSkipper that bypasses auth for the listed full method names.
func SkipMethods(methods ...string) AuthSkipper {
	set := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		set[m] = struct{}{}
	}
	return func(fullMethod string) bool {
		_, ok := set[fullMethod]
		return ok
	}
}

// BearerTokenExtractor returns a TokenExtractor that reads the "authorization"
// metadata key and strips the "Bearer " prefix (case-insensitive).
// Returns Unauthenticated if the key is absent.
func BearerTokenExtractor() TokenExtractor {
	return func(ctx context.Context) (string, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return "", Unauthorized("MISSING_TOKEN", "missing metadata").GRPCStatus().Err()
		}
		vals := md.Get("authorization")
		if len(vals) == 0 {
			return "", Unauthorized("MISSING_TOKEN", "missing authorization header").GRPCStatus().Err()
		}
		token := vals[0]
		if upper := strings.ToUpper(token); strings.HasPrefix(upper, "BEARER ") {
			token = token[7:]
		}
		if token == "" {
			return "", Unauthorized("MISSING_TOKEN", "empty bearer token").GRPCStatus().Err()
		}
		return token, nil
	}
}

// UnaryInterceptorAuth returns a gRPC unary server interceptor that
// authenticates each request using extractor then validator.
//
// Optional AuthSkipper functions are evaluated in order; if any returns true
// for the incoming method, authentication is skipped entirely.
//
// The validator's returned context is propagated to the handler, so auth
// claims injected via context.WithValue are visible downstream.
func UnaryInterceptorAuth(extractor TokenExtractor, validator TokenValidator, skippers ...AuthSkipper) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		for _, skip := range skippers {
			if skip(info.FullMethod) {
				return handler(ctx, req)
			}
		}
		token, err := extractor(ctx)
		if err != nil {
			return nil, encodeGRPCError(err)
		}
		ctx, err = validator(ctx, token)
		if err != nil {
			return nil, encodeGRPCError(err)
		}
		return handler(ctx, req)
	}
}

// StreamInterceptorAuth returns a gRPC stream server interceptor that
// authenticates the stream at setup time using extractor then validator.
//
// Optional AuthSkipper functions are evaluated in order; if any returns true
// for the incoming method, authentication is skipped entirely.
//
// The validator's returned context is propagated into the streaming handler
// via a context-carrying ServerStream wrapper, so stream.Context() reflects
// the enriched context.
func StreamInterceptorAuth(extractor TokenExtractor, validator TokenValidator, skippers ...AuthSkipper) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		for _, skip := range skippers {
			if skip(info.FullMethod) {
				return handler(srv, ss)
			}
		}
		ctx := ss.Context()
		token, err := extractor(ctx)
		if err != nil {
			return encodeGRPCError(err)
		}
		ctx, err = validator(ctx, token)
		if err != nil {
			return encodeGRPCError(err)
		}
		return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})
	}
}
