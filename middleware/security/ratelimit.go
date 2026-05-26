package security

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/astra-go/astra"
)

// RateLimitConfig configures the RateLimit middleware.
type RateLimitConfig struct {
	// Rate is the number of requests allowed per second per client.
	Rate float64
	// Burst is the maximum burst size.
	Burst int
	// KeyFunc extracts a key from the context to identify clients.
	// Defaults to ClientIP.
	KeyFunc func(*astra.Ctx) string
	// Skipper skips rate limiting for matching requests.
	Skipper Skipper
	// ErrorHandler is called when the rate limit is exceeded.
	// Default: 429 Too Many Requests with Retry-After header.
	ErrorHandler ErrorHandler

	// ExceededHandler is an alias for ErrorHandler (deprecated).
	// Prefer ErrorHandler for consistency across middleware.
	// If ErrorHandler is nil and ExceededHandler is set, ExceededHandler is used.
	ExceededHandler astra.HandlerFunc // Deprecated: use ErrorHandler
	// Context controls the lifetime of the internal cleanup goroutine.
	// When the context is cancelled the goroutine exits cleanly.
	// If nil and App is also nil, context.Background() is used and the goroutine
	// runs until the process exits — suitable for top-level app middleware.
	Context context.Context
	// App, when set, wires the cleanup goroutine lifetime to the application
	// shutdown lifecycle automatically.  Takes precedence over Context.
	//
	// Deprecated: pass a context.Context derived from your shutdown signal instead.
	// This field will be removed in a future major version.
	// Example: ctx, cancel := context.WithCancel(context.Background())
	//          app.OnStop(func(_ context.Context) error { cancel(); return nil })
	App *astra.App
}

// DefaultRateLimitConfig allows 100 requests/second with a burst of 20.
var DefaultRateLimitConfig = RateLimitConfig{
	Rate:  100,
	Burst: 20,
}

// RateLimit returns a token-bucket rate limiter middleware.
// Inspired by go-zero's period limiter and token-bucket algorithm.
//
// The internal cleanup goroutine is bound to the App lifecycle when the
// middleware is registered via app.Use(); otherwise it runs until the process
// exits. For controlled shutdown in tests, use NewRateLimiter instead.
//
// IMPORTANT: When used without an App (e.g. in tests), the cleanup goroutine
// runs until the process exits. Prefer NewRateLimiter(rate, burst) in tests:
//
//	mw, stop := middleware.NewRateLimiter(100, 20)
//	defer stop() // ensures the goroutine exits
func RateLimit(rate float64, burst int) astra.HandlerFunc {
	cfg := DefaultRateLimitConfig
	cfg.Rate = rate
	cfg.Burst = burst
	return RateLimitWithConfig(cfg)
}

// NewRateLimiter returns a rate limiter middleware and a stop function.
// Calling stop() cancels the cleanup goroutine immediately, making it safe
// for use in tests and scenarios where the middleware is replaced dynamically:
//
//	mw, stop := middleware.NewRateLimiter(100, 20)
//	defer stop()
//	app.Use(mw)
//
// For app-level lifecycle integration, prefer RateLimitWithConfig with a
// Context derived from app.OnStop.
func NewRateLimiter(rate float64, burst int) (astra.HandlerFunc, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := DefaultRateLimitConfig
	cfg.Rate = rate
	cfg.Burst = burst
	cfg.Context = ctx
	return RateLimitWithConfig(cfg), cancel
}

// RateLimitWithConfig returns a RateLimit middleware with custom config.
func RateLimitWithConfig(cfg RateLimitConfig) astra.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *astra.Ctx) string {
			return c.ClientIP()
		}
	}
	// Resolve deprecated ExceededHandler → ErrorHandler
	if cfg.ErrorHandler == nil && cfg.ExceededHandler != nil {
		cfg.ErrorHandler = ErrorHandler(cfg.ExceededHandler)
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(c *astra.Ctx) error {
			return astra.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
		}
	}
	cfg.Context = resolveContext(cfg.Context, cfg.App)

	store := &tokenBucketStore{
		buckets: make(map[string]*tokenBucket),
		rate:    cfg.Rate,
		burst:   cfg.Burst,
	}

	// Cleanup goroutine: periodically remove stale entries.
	// Exits when cfg.Context is cancelled so tests and dynamic middleware
	// creation do not accumulate goroutines indefinitely.
	go store.cleanup(cfg.Context)

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			return nil
		}
		key := cfg.KeyFunc(c)
		if !store.allow(key) {
			c.Writer().Header().Set("Retry-After", "1")
			return cfg.ErrorHandler(c)
		}
		return nil
	}
}

// tokenBucket implements the token bucket algorithm.
type tokenBucket struct {
	tokens   float64
	lastTime time.Time
	mu       sync.Mutex
}

func (tb *tokenBucket) allow(rate float64, burst int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()
	tb.lastTime = now

	// Refill tokens
	tb.tokens += elapsed * rate
	if tb.tokens > float64(burst) {
		tb.tokens = float64(burst)
	}

	if tb.tokens >= 1.0 {
		tb.tokens--
		return true
	}
	return false
}

// tokenBucketStore manages per-key token buckets.
type tokenBucketStore struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
	rate    float64
	burst   int
}

func (s *tokenBucketStore) allow(key string) bool {
	s.mu.RLock()
	tb, ok := s.buckets[key]
	s.mu.RUnlock()

	if !ok {
		s.mu.Lock()
		// Double-check after acquiring write lock
		if tb, ok = s.buckets[key]; !ok {
			tb = &tokenBucket{
				tokens:   float64(s.burst),
				lastTime: time.Now(),
			}
			s.buckets[key] = tb
		}
		s.mu.Unlock()
	}

	return tb.allow(s.rate, s.burst)
}

func (s *tokenBucketStore) cleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		cutoff := time.Now().Add(-10 * time.Minute)

		// Phase 1: identify stale keys under a read lock so requests are not blocked.
		s.mu.RLock()
		var stale []string
		for key, tb := range s.buckets {
			tb.mu.Lock()
			if tb.lastTime.Before(cutoff) {
				stale = append(stale, key)
			}
			tb.mu.Unlock()
		}
		s.mu.RUnlock()

		if len(stale) == 0 {
			continue
		}

		// Phase 2: remove stale entries under a write lock, re-validating each
		// to guard against activity between phase 1 and phase 2.
		s.mu.Lock()
		for _, key := range stale {
			if tb, ok := s.buckets[key]; ok {
				tb.mu.Lock()
				if tb.lastTime.Before(cutoff) {
					delete(s.buckets, key)
				}
				tb.mu.Unlock()
			}
		}
		s.mu.Unlock()
	}
}
