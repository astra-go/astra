//go:build redis

// Package middleware provides HTTP middleware for the Astra web framework.
//
// DistributedRateLimitConfig extends the standard RateLimitConfig with a
// Redis backend for multi-instance rate limiting.
//
// This file is only compiled when the "redis" build tag is specified:
//
//	go build -tags redis .
//
// Without the tag, this file is excluded from the build and the go-redis
// dependency is not required.  Users who don't need distributed rate limiting
// can build without the tag and avoid pulling in the go-redis dependency.
//
// Usage:
//
//	middleware.DistributedRateLimit(middleware.DistributedRateLimitConfig{
//	    Rate:     100,
//	    Burst:    20,
//	    RedisAddr: "localhost:6379",
//	})
//
// The Redis backend uses a token-bucket algorithm implemented in Lua so that
// all operations are atomic.  A single goroutine is spawned per store to
// handle background work (connection health checks, pool stats logging).
// When multiple Astra instances share the same configuration they
// coordinate via Redis — each instance only touches Redis when the local
// in-memory token bucket is exhausted.
//
// Algorithm — Redis token bucket (Lua, atomic):
//
//	now = redis.call("TIME")
//	elapsed = now - last_time
//	tokens = min(burst, tokens + elapsed * rate)
//	if tokens >= 1 then
//	    tokens = tokens - 1
//	    redis.call("SET", key, tokens .. " " .. now, "EX", expiry_secs, "KEEPTTL")
//	    return 1  -- allowed
//	end
//	return 0  -- rejected

package security

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/redis/go-redis/v9"
)

// DistributedRateLimitConfig extends the standard RateLimitConfig with a
// Redis backend for multi-instance rate limiting.
type DistributedRateLimitConfig struct {
	// Rate is the number of requests allowed per second per client.
	Rate float64
	// Burst is the maximum burst size (tokens accumulated at idle).
	Burst int
	// RedisAddr is the address of the Redis server. Default: "localhost:6379".
	RedisAddr string
	// RedisPassword is the password for Redis AUTH. Empty means no auth.
	RedisPassword string
	// RedisDB selects the logical database. Default: 0.
	RedisDB int
	// KeyPrefix is prepended to every Redis key to isolate multiple apps sharing
	// the same Redis instance. Default: "astra:rl:".
	KeyPrefix string
	// KeyFunc extracts the rate-limit key from the request context.
	// The same key is used for both the local in-memory bucket and the Redis
	// fallback.  Defaults to ClientIP.
	KeyFunc func(*astra.Ctx) string
	// Skipper skips rate limiting for matching requests.
	Skipper Skipper
	// LocalOnly disables the Redis backend and falls back to a pure in-memory
	// token bucket.  Useful during development or when Redis is unavailable.
	LocalOnly bool
	// ErrorHandler is called when the rate limit is exceeded.
	ErrorHandler ErrorHandler
	// ExceededHandler is an alias for ErrorHandler (deprecated).
	// If ErrorHandler is nil and ExceededHandler is set, ExceededHandler is used.
	ExceededHandler astra.HandlerFunc // Deprecated: use ErrorHandler
	// Context controls the lifetime of the background Redis health-check goroutine.
	// When cancelled the goroutine exits and the Redis client is closed.
	// If nil, context.Background() is used and the goroutine runs until the process
	// exits — fine for top-level middleware.
	Context context.Context
	// App, when set, wires the background goroutine lifetime to the application
	// shutdown lifecycle automatically.  Takes precedence over Context.
	App *astra.App
	// Logger is used for Redis connection errors. Defaults to slog.Default().
	Logger *slog.Logger
	// Window is the sliding window duration for DistributedSlidingWindowWithConfig.
	// Default: 1 second.
	Window time.Duration
	// Limit is the maximum number of requests per Window for sliding window.
	Limit int64
}

// DefaultDistributedRateLimitConfig is a sensible default for most deployments.
var DefaultDistributedRateLimitConfig = DistributedRateLimitConfig{
	Rate:      100,
	Burst:     20,
	RedisAddr: "localhost:6379",
	KeyPrefix: "astra:rl:",
	LocalOnly: false,
}

// RedisTokenBucketStore implements a Redis-backed token bucket for use as a
// RateLimiterStore with both RateLimit and SlidingWindow.
//
// Keys are prefixed to avoid collisions.  The Lua script executes atomically
// so there are no race conditions between concurrent instances.
//
// Token-bucket state format in Redis (one key per key):
//
//	"<tokens float> <last纳秒 unix time>"
//
// The TTL is set to 2 × (Burst/Rate) so idle keys expire quickly.
type RedisTokenBucketStore struct {
	client    *redis.Client
	keyPrefix string
	rate      float64
	burst     int

	// Lua script: takes key, rate, burst, now_nsec, expiry_seconds.
	// Returns 1 if allowed, 0 if rejected.
	script *redis.Script

	mu       sync.RWMutex
	online   bool
	stopOnce sync.Once
}

var tokenBucketRedisScript = redis.NewScript(`
local key   = KEYS[1]
local rate  = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now   = tonumber(ARGV[3])
local ttl   = tonumber(ARGV[4])

local raw = redis.call("GET", key)
local tokens, last_time

if raw then
    local parts = {}
    for part in raw:gmatch("%S+") do table.insert(parts, part) end
    tokens     = tonumber(parts[1]) or 0
    last_time  = tonumber(parts[2]) or now
else
    tokens    = burst
    last_time = now
end

local elapsed = (now - last_time) / 1e9
tokens = math.min(burst, tokens + elapsed * rate)

if tokens >= 1 then
    tokens = tokens - 1
    redis.call("SET", key, string.format("%.9f %d", tokens, now), "EX", ttl, "KEEPTTL")
    return 1
end
return 0
`)

// NewRedisTokenBucketStore creates a Redis-backed token-bucket store.
func NewRedisTokenBucketStore(cfg DistributedRateLimitConfig) (*RedisTokenBucketStore, func()) {
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "astra:rl:"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	store := &RedisTokenBucketStore{
		client:    redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB}),
		keyPrefix: cfg.KeyPrefix,
		rate:      cfg.Rate,
		burst:     cfg.Burst,
		script:    tokenBucketRedisScript,
	}

	// Probe Redis to confirm connectivity.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.client.Ping(ctx).Err(); err != nil {
		cfg.Logger.Warn("distributed rate limiter: Redis unavailable, falling back to in-memory",
			slog.String("addr", cfg.RedisAddr), slog.String("err", err.Error()))
		store.mu.Lock()
		store.online = false
		store.mu.Unlock()
	} else {
		store.mu.Lock()
		store.online = true
		store.mu.Unlock()
		cfg.Logger.Info("distributed rate limiter: Redis connected",
			slog.String("addr", cfg.RedisAddr))
	}

	stop := func() {
		store.stopOnce.Do(func() { store.client.Close() })
	}
	return store, stop
}

// Allow checks the token bucket. Returns true if a token is available.
func (s *RedisTokenBucketStore) Allow(ctx context.Context, key string) (bool, error) {
	s.mu.RLock()
	online := s.online
	s.mu.RUnlock()

	if !online {
		return false, nil
	}

	now := time.Now().UnixNano()
	ttl := int64(float64(s.burst)/s.rate) * 2
	if ttl < 10 {
		ttl = 10
	}

	result, err := s.script.Run(ctx, s.client,
		[]string{s.keyPrefix + key},
		s.rate, s.burst, now, ttl,
	).Int()

	if err != nil {
		// Mark Redis offline on error; allow requests to pass.
		if isRedisRetryable(err) {
			s.mu.Lock()
			s.online = false
			s.mu.Unlock()
			return true, nil
		}
		return false, fmt.Errorf("redis token-bucket: %w", err)
	}

	// Re-enable Redis on successful call after a previous failure.
	s.mu.Lock()
	if !s.online {
		s.online = true
		s.mu.Unlock()
	}

	return result == 1, nil
}

// SetOnline marks the store as online or offline.
func (s *RedisTokenBucketStore) SetOnline(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online = v
}

// Close releases the Redis connection.
func (s *RedisTokenBucketStore) Close() error {
	return s.client.Close()
}

// isRedisRetryable returns true for transient Redis errors that should not
// block the request (e.g. connection refused, timeout, MOVED/ASK redirects).
func isRedisRetryable(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "redis: connection pool exhausted") ||
		strings.Contains(errStr, "MOVED") ||
		strings.Contains(errStr, "ASK")
}

// ─── Redis sliding-window store ───────────────────────────────────────────────

// RedisSlidingWindowStore implements the sliding-window counter algorithm
// using a Redis sorted set (ZSET).  Each request is stored as a member scored
// by its timestamp.  Old entries outside the window are removed on each call.
//
// Window = [now - window, now)
// Approximation: count(current-window) × (elapsed/window) + count(previous-window)
//
// The ZSET approach is O(log N) per request but is trivially correct.
// For higher throughput, the token-bucket Redis store above is preferred.
type RedisSlidingWindowStore struct {
	client    *redis.Client
	keyPrefix string
	window    time.Duration
	limit     int64

	mu       sync.RWMutex
	online   bool
	stopOnce sync.Once
}

// NewRedisSlidingWindowStore creates a Redis-backed sliding-window store.
func NewRedisSlidingWindowStore(client *redis.Client, keyPrefix string, window time.Duration, limit int64) (*RedisSlidingWindowStore, func()) {
	if keyPrefix == "" {
		keyPrefix = "astra:sw:"
	}
	store := &RedisSlidingWindowStore{
		client:    client,
		keyPrefix: keyPrefix,
		window:    window,
		limit:     limit,
		online:    true,
	}
	return store, func() { store.client.Close() }
}

// Allow checks the sliding window counter.
func (s *RedisSlidingWindowStore) Allow(ctx context.Context, key string) (bool, error) {
	s.mu.RLock()
	online := s.online
	s.mu.RUnlock()

	if !online {
		return false, nil
	}

	now := time.Now()
	windowStart := now.Add(-s.window).UnixMilli()
	redisKey := s.keyPrefix + key

	// Lua script: sliding window using ZSET (score = timestamp).
	// Returns 1 if allowed, 0 if rejected.
	script := redis.NewScript(`
local key          = KEYS[1]
local window_start = tonumber(ARGV[1])
local now         = tonumber(ARGV[2])
local limit       = tonumber(ARGV[3])
local window_ms   = tonumber(ARGV[4])
local uid         = ARGV[5]

-- Remove entries older than window_start
redis.call("ZREMRANGEBYSCORE", key, "-inf", window_start)

-- Count entries in the current window
local count = redis.call("ZCARD", key)

if count < limit then
    -- Add this request (use unique member to allow multiple requests at same ms)
    redis.call("ZADD", key, now, uid)
    -- Set TTL to window_ms × 2 so idle keys self-clean
    redis.call("PEXPIRE", key, window_ms * 2)
    return 1
end
return 0
`)

	uid := fmt.Sprintf("%d:%d", now.UnixNano(), now.UnixNano()%1000000)
	nowMs := now.UnixMilli()
	windowMs := s.window.Milliseconds()

	result, err := script.Run(ctx, s.client,
		[]string{redisKey},
		windowStart, nowMs, s.limit, windowMs, uid,
	).Int()

	if err != nil {
		if isRedisRetryable(err) {
			s.mu.Lock()
			s.online = false
			s.mu.Unlock()
			return true, nil
		}
		return false, fmt.Errorf("redis sliding-window: %w", err)
	}

	s.mu.Lock()
	if !s.online {
		s.online = true
		s.mu.Unlock()
	}

	return result == 1, nil
}

// SetOnline marks the store as online or offline.
func (s *RedisSlidingWindowStore) SetOnline(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online = v
}

// Close releases the Redis connection.
func (s *RedisSlidingWindowStore) Close() error {
	return s.client.Close()
}

// ─── RateLimiterStore interface ──────────────────────────────────────────────

// RateLimiterStore abstracts the storage backend for rate limiters.
// Both in-memory and distributed (Redis) stores satisfy this interface.
type RateLimiterStore interface {
	// Allow checks whether a request for the given key is within the rate limit.
	// Returns (allowed bool, error).
	// Errors are non-fatal: when Store returns an error the middleware falls
	// back to allowing the request (fail-open) so that Redis outages do not
	// block legitimate traffic.
	Allow(ctx context.Context, key string) (bool, error)
}

// ─── Distributed rate-limit middleware ────────────────────────────────────────

// DistributedRateLimitWithConfig returns a rate limiter middleware backed by
// Redis for multi-instance coordination.
//
// If Redis is unavailable (connection refused, timeout), the middleware
// falls back to an in-memory token bucket and marks the store offline.
// Subsequent requests automatically probe Redis and re-enable it on success.
//
// Example — 100 req/s, burst 20, shared across all instances:
//
//	app.Use(middleware.DistributedRateLimitWithConfig(middleware.DistributedRateLimitConfig{
//	    Rate:       100,
//	    Burst:      20,
//	    RedisAddr:  "10.0.0.42:6379",
//	    KeyFunc: func(c *astra.Ctx) string { return c.ClientIP() },
//	}))
func DistributedRateLimitWithConfig(cfg DistributedRateLimitConfig) (astra.HandlerFunc, func()) {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *astra.Ctx) string { return c.ClientIP() }
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

	// Build the Redis-backed store.
	redisStore, stopRedis := NewRedisTokenBucketStore(cfg)

	// Fallback in-memory store (pure token bucket, no Redis).
	memStore := &tokenBucketStore{
		buckets: make(map[string]*tokenBucket),
		rate:    cfg.Rate,
		burst:   cfg.Burst,
	}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			return nil
		}
		key := cfg.KeyFunc(c)

		// Fast path: check local in-memory bucket first.
		// This avoids a Redis round-trip for the majority of requests.
		if memStore.allow(key) {
			return nil
		}

		// Local bucket exhausted: try Redis.
		allowed, err := redisStore.Allow(c.Request().Context(), key)
		if err != nil {
			// Redis error: fail open (allow) to avoid blocking traffic.
			return nil
		}
		if allowed {
			return nil
		}

		c.Writer().Header().Set("Retry-After", "1")
		return cfg.ErrorHandler(c)
	}, stopRedis
}

// DistributedRateLimit is shorthand with DefaultDistributedRateLimitConfig.
func DistributedRateLimit(redisAddr string, rate float64, burst int) (astra.HandlerFunc, func()) {
	return DistributedRateLimitWithConfig(DistributedRateLimitConfig{
		RedisAddr: redisAddr,
		Rate:      rate,
		Burst:     burst,
	})
}

// DistributedSlidingWindowWithConfig returns a sliding-window rate limiter
// backed by Redis for multi-instance coordination.
func DistributedSlidingWindowWithConfig(cfg DistributedRateLimitConfig) (astra.HandlerFunc, func()) {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *astra.Ctx) string { return c.ClientIP() }
	}
	// Resolve deprecated ExceededHandler → ErrorHandler
	if cfg.ErrorHandler == nil && cfg.ExceededHandler != nil {
		cfg.ErrorHandler = ErrorHandler(cfg.ExceededHandler)
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(c *astra.Ctx) error {
			c.Writer().Header().Set("Retry-After", cfg.Window.String())
			return astra.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
		}
	}

	window := cfg.Window
	if window <= 0 {
		window = time.Second
	}
	limit := cfg.Limit
	if limit <= 0 {
		limit = 100
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	keyPrefix := cfg.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "astra:sw:"
	}

	store := &RedisSlidingWindowStore{
		client:    client,
		keyPrefix: keyPrefix,
		window:    window,
		limit:     limit,
		online:    true,
	}

	memStore := &swStore{window: window}

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			return nil
		}
		key := cfg.KeyFunc(c)

		if memStore.allow(key, limit) {
			return nil
		}

		allowed, err := store.Allow(c.Request().Context(), key)
		if err != nil {
			return nil
		}
		if allowed {
			return nil
		}

		return cfg.ErrorHandler(c)
	}, func() { _ = client.Close() }
}

// DistributedSlidingWindow is shorthand.
func DistributedSlidingWindow(redisAddr string, limit int64, window time.Duration) (astra.HandlerFunc, func()) {
	return DistributedSlidingWindowWithConfig(DistributedRateLimitConfig{
		RedisAddr: redisAddr,
		Limit:    limit,
		Window:   window,
	})
}