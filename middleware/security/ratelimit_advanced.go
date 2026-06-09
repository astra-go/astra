package security

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astra-go/astra"
)

// ─── Sliding-window rate limiter ─────────────────────────────────────────────
//
// This file extends the basic token-bucket RateLimit with a more accurate
// sliding-window counter algorithm and richer keying options:
//
//   - Per-route keying:   combine route pattern + client IP
//   - Per-user keying:    extract user ID or API key from context
//   - Per-API-key quota:  per-key rate/burst configured at middleware creation
//   - Sliding window:     fairer than a token bucket at window boundaries
//
// Algorithm — two-window sliding counter approximation:
//
//	Within [t-window, t), weight the previous window count by the fraction of
//	the current window that has elapsed, add the current-window count, and
//	compare against the limit.
//
//	This is the same algorithm used by Cloudflare, Nginx limit_req_zone, and
//	Redis CELL (CRedit ELimiter).  It is O(1) per check and uses only two
//	int64 counters per key.

// SlidingWindowConfig configures the sliding-window rate limiter.
type SlidingWindowConfig struct {
	// Limit is the maximum number of requests allowed per Window.
	Limit int64
	// Window is the rate-limiting time window. Default: 1s.
	Window time.Duration
	// KeyFunc extracts the throttle key from the request context.
	// The default key is the client IP address.
	// Override to implement per-user, per-route, or combined keying:
	//
	//   // Per user ID stored in the context by the JWT middleware:
	//   KeyFunc: func(c *contract.Context) string { return c.GetString("user_id") }
	//
	//   // Per route + IP (most selective):
	//   KeyFunc: func(c *contract.Context) string {
	//       return c.Request.URL.Path + ":" + c.ClientIP()
	//   }
	KeyFunc func(*astra.Ctx) string
	// Skipper skips rate limiting for matching requests.
	Skipper Skipper
	// ErrorHandler is invoked when the limit is exceeded.
	// Default: 429 Too Many Requests with a Retry-After header.
	ErrorHandler ErrorHandler
	// Keys are matched against the value returned by KeyFunc.
	// Unmatched keys fall back to Limit.
	//
	// Useful for per-API-key tiered quotas:
	//
	//   PerKeyLimits: map[string]int64{
	//       "api-key-premium": 10000,
	//       "api-key-free":    100,
	//   }
	PerKeyLimits map[string]int64
	// Context controls the lifetime of the internal cleanup goroutine.
	// When cancelled the goroutine exits cleanly.
	// If nil and App is also nil, context.Background() is used (goroutine runs
	// until the process exits — fine for long-lived app middleware).
	Context context.Context
}

// SlidingWindow returns a middleware that enforces the sliding-window rate limit
// described by cfg.
//
// Unlike the basic RateLimit middleware (token bucket), SlidingWindow prevents
// bursty traffic at window boundaries and provides a smoother enforcement curve.
//
// The cleanup goroutine runs until the process exits.
// Use NewSlidingWindow or SlidingWindowWithConfig with Context/App when you
// need controlled shutdown (tests, dynamic middleware replacement).
func SlidingWindow(limit int64, window time.Duration) astra.HandlerFunc {
	mw, _ := SlidingWindowWithConfig(SlidingWindowConfig{
		Limit:  limit,
		Window: window,
	})
	return mw
}

// NewSlidingWindow returns a sliding-window middleware and a stop function.
// Calling stop() cancels the cleanup goroutine immediately, making it safe
// for use in tests and scenarios where the middleware is replaced dynamically:
//
//	mw, stop := middleware.NewSlidingWindow(100, time.Second)
//	defer stop()
//	app.Use(mw)
func NewSlidingWindow(limit int64, window time.Duration) (astra.HandlerFunc, func()) {
	return SlidingWindowWithConfig(SlidingWindowConfig{
		Limit:  limit,
		Window: window,
	})
}

// SlidingWindowWithConfig returns a SlidingWindow middleware with full config.
func SlidingWindowWithConfig(cfg SlidingWindowConfig) (astra.HandlerFunc, func()) {
	if cfg.Window <= 0 {
		cfg.Window = time.Second
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *astra.Ctx) string { return c.ClientIP() }
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(c *astra.Ctx) error {
			c.Writer().Header().Set("Retry-After",
				time.Duration(cfg.Window).String())
			return astra.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
		}
	}

	ctx, cancel := context.WithCancel(resolveContext(cfg.Context))

	store := &swStore{window: cfg.Window}

	// Periodic cleanup — remove entries idle for more than 2 windows.
	// Goroutine exits when ctx is cancelled.
	go func() {
		ticker := time.NewTicker(cfg.Window * 10)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				store.evict(time.Now())
			}
		}
	}()

	mw := func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			return nil
		}
		key := cfg.KeyFunc(c)

		limit := cfg.Limit
		if cfg.PerKeyLimits != nil {
			if perKey, ok := cfg.PerKeyLimits[key]; ok {
				limit = perKey
			}
		}

		if !store.allow(key, limit) {
			return cfg.ErrorHandler(c)
		}
		return nil
	}
	return mw, cancel
}

// ─── swStore — the per-key sliding-window store ───────────────────────────────

// swEntry holds the two-window counters for a single key.
type swEntry struct {
	// prevCount and currCount are atomic to avoid lock contention on the hot path.
	prevCount int64
	currCount int64
	// windowStart is the start of the current window (guarded by mu).
	windowStart time.Time
	mu          sync.Mutex
	lastSeen    time.Time // for eviction (guarded by mu)
}

// swStore manages per-key sliding-window entries.
type swStore struct {
	mu      sync.RWMutex
	entries map[string]*swEntry
	window  time.Duration
}

// allow returns true if the request is within the limit.
func (s *swStore) allow(key string, limit int64) bool {
	now := time.Now()
	entry := s.getOrCreate(key, now)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.lastSeen = now

	// Determine which window we're in.
	elapsed := now.Sub(entry.windowStart)
	if elapsed >= s.window {
		// Roll over: the current window becomes previous, start a new one.
		atomic.StoreInt64(&entry.prevCount, atomic.LoadInt64(&entry.currCount))
		atomic.StoreInt64(&entry.currCount, 0)
		entry.windowStart = now
		elapsed = 0
	}

	// Sliding estimate: weight previous window by the fraction of the current
	// window that has NOT yet elapsed.
	fraction := 1.0 - float64(elapsed)/float64(s.window)
	prev := float64(atomic.LoadInt64(&entry.prevCount))
	curr := float64(atomic.LoadInt64(&entry.currCount))

	estimated := prev*fraction + curr

	if int64(estimated) >= limit {
		return false
	}

	atomic.AddInt64(&entry.currCount, 1)
	return true
}

func (s *swStore) getOrCreate(key string, now time.Time) *swEntry {
	s.mu.RLock()
	e, ok := s.entries[key]
	s.mu.RUnlock()

	if ok {
		return e
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock.
	if e, ok = s.entries[key]; ok {
		return e
	}

	e = &swEntry{windowStart: now, lastSeen: now}
	if s.entries == nil {
		s.entries = make(map[string]*swEntry)
	}
	s.entries[key] = e
	return e
}

// evict removes entries that have been idle for more than 2 windows.
func (s *swStore) evict(now time.Time) {
	cutoff := now.Add(-2 * s.window)

	// Phase 1: collect stale keys under a read lock.
	s.mu.RLock()
	var stale []string
	for k, e := range s.entries {
		e.mu.Lock()
		if e.lastSeen.Before(cutoff) {
			stale = append(stale, k)
		}
		e.mu.Unlock()
	}
	s.mu.RUnlock()

	if len(stale) == 0 {
		return
	}

	// Phase 2: remove under a write lock, re-validating staleness.
	s.mu.Lock()
	for _, k := range stale {
		if e, ok := s.entries[k]; ok {
			e.mu.Lock()
			if e.lastSeen.Before(cutoff) {
				delete(s.entries, k)
			}
			e.mu.Unlock()
		}
	}
	s.mu.Unlock()
}

// ─── Per-route quota middleware ───────────────────────────────────────────────

// RouteQuotaConfig defines per-route rate limits.
type RouteQuotaConfig struct {
	// Routes maps URL path prefixes to their (limit, window) tuples.
	// The first matching prefix wins.
	Routes []RouteQuota
	// DefaultLimit applies when no route prefix matches.
	DefaultLimit int64
	// DefaultWindow applies when no route prefix matches. Default: 1s.
	DefaultWindow time.Duration
	// KeyFunc extracts the per-client identifier.
	// Default: client IP.
	KeyFunc func(*astra.Ctx) string
	// Skipper skips rate limiting for matching requests.
	Skipper Skipper
	// ErrorHandler is invoked when the limit is exceeded.
	ErrorHandler ErrorHandler
	// Context controls the lifetime of the internal cleanup goroutines.
	// When cancelled all goroutines exit cleanly.
	// If nil, context.Background() is used.
	Context context.Context
}

// RouteQuota pairs a URL path prefix with its rate limit.
type RouteQuota struct {
	// Prefix is the URL path prefix to match (e.g. "/api/v1/upload").
	Prefix string
	// Limit is the maximum number of requests per Window.
	Limit int64
	// Window is the rate-limiting window. Default: 1s.
	Window time.Duration
}

// NewRouteQuotaMiddleware returns a route-quota middleware and a stop function.
// Calling stop() cancels all internal cleanup goroutines immediately, making
// it safe for use in tests and dynamic middleware replacement:
//
//	mw, stop := middleware.NewRouteQuotaMiddleware(middleware.RouteQuotaConfig{...})
//	defer stop()
//	app.Use(mw)
func NewRouteQuotaMiddleware(cfg RouteQuotaConfig) (astra.HandlerFunc, func()) {
	return RouteQuotaMiddleware(cfg)
}

// RouteQuotaMiddleware returns a middleware that applies different rate limits
// per URL path prefix.
//
// Example — tighter limits on expensive endpoints:
//
//	app.Use(middleware.RouteQuotaMiddleware(middleware.RouteQuotaConfig{
//	    Routes: []middleware.RouteQuota{
//	        {Prefix: "/api/v1/upload",  Limit: 5,   Window: time.Minute},
//	        {Prefix: "/api/v1/report",  Limit: 10,  Window: time.Minute},
//	        {Prefix: "/api/",          Limit: 200,  Window: time.Second},
//	    },
//	    DefaultLimit:  500,
//	    DefaultWindow: time.Second,
//	}))
func RouteQuotaMiddleware(cfg RouteQuotaConfig) (astra.HandlerFunc, func()) {
	if cfg.DefaultWindow <= 0 {
		cfg.DefaultWindow = time.Second
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *astra.Ctx) string { return c.ClientIP() }
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(c *astra.Ctx) error {
			return astra.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
		}
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].Window <= 0 {
			cfg.Routes[i].Window = time.Second
		}
	}

	ctx, cancel := context.WithCancel(resolveContext(cfg.Context))

	// One swStore per route entry + one for the default.
	stores := make([]*swStore, len(cfg.Routes)+1)
	for i := range stores {
		w := cfg.DefaultWindow
		if i < len(cfg.Routes) {
			w = cfg.Routes[i].Window
		}
		stores[i] = &swStore{window: w}
		idx := i
		go func() {
			ticker := time.NewTicker(w * 10)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					stores[idx].evict(time.Now())
				}
			}
		}()
	}
	defaultStore := stores[len(stores)-1]

	mw := func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			return nil
		}
		path := c.Request().URL.Path
		key := cfg.KeyFunc(c)

		for i, route := range cfg.Routes {
			if hasPrefix(path, route.Prefix) {
				if !stores[i].allow(key, route.Limit) {
					return cfg.ErrorHandler(c)
				}
				return nil
			}
		}

		// No route matched: apply default limit.
		if cfg.DefaultLimit > 0 && !defaultStore.allow(key, cfg.DefaultLimit) {
			return cfg.ErrorHandler(c)
		}
		return nil
	}
	return mw, cancel
}

// hasPrefix returns true if path starts with prefix.
// The prefix is matched at a path-component boundary so that "/api" does not
// accidentally match "/apiv2".
func hasPrefix(path, prefix string) bool {
	if prefix == "" || prefix == "/" {
		return true
	}
	if len(path) < len(prefix) {
		return false
	}
	if path[:len(prefix)] != prefix {
		return false
	}
	// Ensure we're at a path-component boundary.
	if len(path) == len(prefix) {
		return true
	}
	return path[len(prefix)] == '/'
}

// ─── shared helpers ───────────────────────────────────────────────────────────

// resolveContext returns the context to use for cleanup goroutines:
//  1. If explicit ctx != nil, use it directly.
//  2. Fall back to context.Background() (goroutine lives until process exits).
func resolveContext(explicit context.Context) context.Context {
	if explicit != nil {
		return explicit
	}
	return context.Background()
}
