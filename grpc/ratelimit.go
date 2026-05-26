package grpcserver

import (
	"context"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// UnaryInterceptorRateLimit returns a gRPC unary server interceptor that
// enforces a per-client token-bucket rate limit.
//
// Clients are identified by their peer IP address. Requests that exceed the
// limit receive a ResourceExhausted (429) status error.
//
// Example:
//
//	s := grpcserver.New(app,
//	    grpcserver.WithUnaryInterceptors(
//	        grpcserver.UnaryInterceptorRateLimit(100, 20),
//	        grpcserver.UnaryInterceptorTracing(),
//	    ),
//	)
func UnaryInterceptorRateLimit(rate float64, burst int) grpc.UnaryServerInterceptor {
	limiter := newGRPCRateLimiter(rate, burst)
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if !limiter.allow(peerAddr(ctx)) {
			return nil, TooManyRequests("RATE_LIMIT", "rate limit exceeded").GRPCStatus().Err()
		}
		return handler(ctx, req)
	}
}

// StreamInterceptorRateLimit returns a gRPC stream server interceptor that
// rate limits stream establishment per client peer.
//
// The check runs once at stream setup time. If the limit is exceeded the stream
// is rejected immediately with ResourceExhausted before the handler is called.
//
// Example:
//
//	s := grpcserver.New(app,
//	    grpcserver.WithStreamInterceptors(
//	        grpcserver.StreamInterceptorRateLimit(50, 10),
//	        grpcserver.StreamInterceptorTracing(),
//	    ),
//	)
func StreamInterceptorRateLimit(rate float64, burst int) grpc.StreamServerInterceptor {
	limiter := newGRPCRateLimiter(rate, burst)
	return func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if !limiter.allow(peerAddr(ss.Context())) {
			return TooManyRequests("RATE_LIMIT", "rate limit exceeded").GRPCStatus().Err()
		}
		return handler(srv, ss)
	}
}

// ─── internal token-bucket store ─────────────────────────────────────────────

// grpcRateLimiter manages per-peer token buckets for gRPC interceptors.
type grpcRateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*grpcBucket
	rate    float64
	burst   float64
}

type grpcBucket struct {
	mu       sync.Mutex
	tokens   float64
	lastTime time.Time
}

func newGRPCRateLimiter(rate float64, burst int) *grpcRateLimiter {
	l := &grpcRateLimiter{
		buckets: make(map[string]*grpcBucket),
		rate:    rate,
		burst:   float64(burst),
	}
	go l.cleanup()
	return l
}

func (l *grpcRateLimiter) allow(key string) bool {
	l.mu.RLock()
	b, ok := l.buckets[key]
	l.mu.RUnlock()

	if !ok {
		l.mu.Lock()
		if b, ok = l.buckets[key]; !ok {
			b = &grpcBucket{tokens: l.burst, lastTime: time.Now()}
			l.buckets[key] = b
		}
		l.mu.Unlock()
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens += now.Sub(b.lastTime).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.lastTime = now
	if b.tokens >= 1.0 {
		b.tokens--
		return true
	}
	return false
}

// cleanup removes buckets that have been idle for more than 10 minutes.
// Runs in a background goroutine for the server lifetime.
func (l *grpcRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)

		l.mu.RLock()
		var stale []string
		for key, b := range l.buckets {
			b.mu.Lock()
			if b.lastTime.Before(cutoff) {
				stale = append(stale, key)
			}
			b.mu.Unlock()
		}
		l.mu.RUnlock()

		if len(stale) == 0 {
			continue
		}

		l.mu.Lock()
		for _, key := range stale {
			if b, ok := l.buckets[key]; ok {
				b.mu.Lock()
				if b.lastTime.Before(cutoff) {
					delete(l.buckets, key)
				}
				b.mu.Unlock()
			}
		}
		l.mu.Unlock()
	}
}

// peerAddr extracts the client IP from the gRPC peer in ctx.
// Falls back to "unknown" when no peer information is available.
func peerAddr(ctx context.Context) string {
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			return host
		}
		return p.Addr.String()
	}
	return "unknown"
}
