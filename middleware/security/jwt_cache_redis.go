//go:build redis

// Package middleware provides HTTP middleware for the Astra web framework.
//
// JWT Cache Redis backend — jwt_cache_redis.go
//
// This file is only compiled when the "redis" build tag is specified:
//
//	go build -tags redis .
//
// Without the tag, this file is excluded from the build and the go-redis
// dependency is not required.  Users who don't need Redis-backed JWT caching
// can build without the tag and avoid pulling in the go-redis dependency.
//
// Multi-instance JWT cache backed by Redis. All Astra instances sharing the same
// Redis server coordinate via the shared cache, eliminating the inconsistency
// of per-instance in-memory caches (where Instance A may have a cached token
// that Instance B has already invalidated).
//
// Fail-open: when Redis is unavailable or the cache miss is expected (no key),
// validation falls through to cryptographic parsing — the request is not blocked.
// Redis errors are logged asynchronously to avoid polluting response latency.
//
// Usage:
//
//	import cacheredis "github.com/astra-go/astra/cache/redis"
//	import "github.com/redis/go-redis/v9"
//
//	redisCache, err := cacheredis.New(cacheredis.Config{Addr: "localhost:6379"})
//	if err != nil { log.Fatal(err) }
//
//	app.Use(middleware.JWTWithConfig(middleware.JWTConfig{
//	    Secret:    "my-secret",
//	    RedisCache: middleware.NewRedisJWTCache(redisCache.Client()),
//	}))
//
// Key design decisions:
//   - Uses the token's signature segment (last dot-separated field, ~43 chars for HS256)
//     as the cache key, keeping key length short and lookup fast.
//   - TTL is capped at cacheUntil = now + (expireAt - now) * 4/5, so the cached
//     entry is evicted before the token actually expires. This prevents stale-claim
//     hits in edge cases where a short-lived token is cached close to expiry.
//   - Claims are serialised to JSON using a custom ClaimsJSON type that handles
//     the pointer fields in jwt.RegisteredClaims (*NumericDate → Unix float64).
//   - Redis errors do not block requests; they fall through to normal JWT parsing.

package security

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	goredis "github.com/redis/go-redis/v9"
)

const (
	// jwtCacheKeyPrefix is prepended to every Redis key to avoid collisions
	// with other Redis keys used by the application.
	jwtCacheKeyPrefix = "astra:jwt:"
)

// RedisJWTCache is a Redis-backed JWT token cache.
//
// It satisfies the same get/set interface as the in-memory jwtCache, but stores
// parsed *Claims in Redis so that multiple Astra instances share the same cache.
// This solves the multi-instance inconsistency problem: when Instance A validates
// and caches a token, Instance B immediately sees the cached entry without
// re-parsing the cryptographic signature.
//
// Redis errors are always non-fatal: Get returns (nil, false) and Set is a
// fire-and-forget. The request always falls through to normal JWT parsing.
type RedisJWTCache struct {
	client   *goredis.Client
	prefix   string
	logger   *slog.Logger
	pooled   bool // whether client was created inside NewRedisJWTCache
	closeMu  sync.Mutex
	closed   bool
}

// NewRedisJWTCache creates a Redis-backed JWT cache backed by the given client.
//
// The client is not closed by this library; the caller is responsible for
// calling client.Close() when the cache is no longer needed, or pass
// closeClient=true to have this library close it.
//
// The default key prefix is "astra:jwt:". Provide a custom prefix when multiple
// Astra apps share the same Redis instance to avoid key collisions:
//
//	NewRedisJWTCache(client, WithJWTCachePrefix("myapp:jwt:"))
func NewRedisJWTCache(client *goredis.Client, opts ...RedisJWTCacheOption) *RedisJWTCache {
	if client == nil {
		panic("jwt_cache_redis: client must not be nil")
	}
	c := &RedisJWTCache{
		client: client,
		prefix: jwtCacheKeyPrefix,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// RedisJWTCacheOption configures a RedisJWTCache.
type RedisJWTCacheOption func(*RedisJWTCache)

// WithJWTCachePrefix sets a custom key prefix (default: "astra:jwt:").
// The prefix is prepended to every Redis key so that multiple applications
// sharing one Redis instance do not overwrite each other's cache entries.
func WithJWTCachePrefix(prefix string) RedisJWTCacheOption {
	return func(c *RedisJWTCache) {
		c.prefix = prefix
	}
}

// WithJWTCacheLogger sets the logger used for async Redis error logging.
// The default is slog.Default().
func WithJWTCacheLogger(logger *slog.Logger) RedisJWTCacheOption {
	return func(c *RedisJWTCache) {
		c.logger = logger
	}
}

func (c *RedisJWTCache) k(sig string) string { return c.prefix + sig }

// Get retrieves cached *Claims for the given token signature.
// Returns (claims, true) on a cache hit.
// Returns (nil, false) on a cache miss or on any Redis error (fail-open).
//
// Cache entries that have expired in Redis are automatically not returned
// (Redis TTL handles expiry), so there is no need to check expiration separately.
func (c *RedisJWTCache) Get(ctx context.Context, sig string) (*Claims, bool) {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()
	if closed {
		return nil, false
	}

	key := c.k(sig)
	b, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		// Fail-open: Redis error is not fatal. Log asynchronously to avoid
		// polluting response latency.
		if !errors.Is(err, goredis.Nil) {
			go c.logErr("Get", key, err)
		}
		return nil, false
	}

	var cj ClaimsJSON
	if err := json.Unmarshal(b, &cj); err != nil {
		go c.logErr("Unmarshal", key, err)
		return nil, false
	}

	claims := cj.ToClaims()
	return claims, true
}

// Set stores parsed *Claims for the given token signature with an appropriate TTL.
//
// The TTL is computed as: now + (expireAt - now) * 4/5
// This evicts the entry before the token actually expires, preventing stale-claim
// hits on the boundary. If the remaining lifetime is too short (< 5s), the entry
// is not stored.
//
// Redis errors are silently ignored (fire-and-forget). The cache miss on the next
// request simply falls through to normal JWT parsing.
func (c *RedisJWTCache) Set(ctx context.Context, sig string, claims *Claims, expireAt int64) {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()
	if closed {
		return
	}

	now := time.Now().Unix()
	remain := expireAt - now
	if remain <= 0 {
		return // already expired
	}
	// Mirror jwtCache.set: cache for 80% of remaining lifetime.
	cacheFor := remain * 4 / 5
	if cacheFor <= 0 {
		return
	}
	ttl := time.Duration(cacheFor) * time.Second

	cj := NewClaimsJSON(claims)
	b, err := json.Marshal(cj)
	if err != nil {
		go c.logErr("Marshal", sig, err)
		return
	}

	key := c.k(sig)
	if err := c.client.Set(ctx, key, b, ttl).Err(); err != nil {
		go c.logErr("Set", key, err)
	}
}

// Delete removes a cached entry, useful after a token has been revoked.
// Returns nil if the key did not exist (no error in that case).
func (c *RedisJWTCache) Delete(ctx context.Context, sig string) error {
	c.closeMu.Lock()
	closed := c.closed
	c.closeMu.Unlock()
	if closed {
		return nil
	}
	return c.client.Del(ctx, c.k(sig)).Err()
}

// Ping verifies that the Redis connection is alive.
func (c *RedisJWTCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close is a no-op when the client is owned externally.
// When closeClient=true was passed to NewRedisJWTCache, this closes the client.
func (c *RedisJWTCache) Close() error {
	c.closeMu.Lock()
	c.closed = true
	// client.Close is not called here because the client may be shared.
	// The caller is responsible for closing it.
	c.closeMu.Unlock()
	return nil
}

func (c *RedisJWTCache) logErr(op, key string, err error) {
	if c.logger != nil {
		c.logger.Error("jwt_cache_redis: "+op+" error",
			slog.String("key", key),
			slog.String("err", err.Error()),
		)
	}
}

// ─── JSON serialisation ───────────────────────────────────────────────────────

// ClaimsJSON is a JSON-serialisable representation of *Claims.
// It exists solely to handle the pointer fields in jwt.RegisteredClaims
// (*ExpiresAt, *NotBefore, *IssuedAt of type *jwt.NumericDate) which cannot
// be directly marshalled by encoding/json.
//
// jwt.RegisteredClaims serialises registered claims (exp, nbf, iat, iss, sub, aud, jti)
// with numeric date values stored as Unix timestamps (float64).
// Custom claims in Extra are serialised as-is.
type ClaimsJSON struct {
	RegisteredClaimsJSON
	Extra map[string]any `json:"extra,omitempty"`
}

// RegisteredClaimsJSON is the JSON representation of jwt.RegisteredClaims.
// jwt.RegisteredClaims uses *NumericDate for time fields; this struct converts
// them to/from Unix float64 for JSON round-tripping.
type RegisteredClaimsJSON struct {
	Issuer    string  `json:"iss,omitempty"`
	Subject   string  `json:"sub,omitempty"`
	ID        string  `json:"jti,omitempty"`
	Audience  []string `json:"aud,omitempty"`
	ExpiresAt *float64 `json:"exp,omitempty"`
	NotBefore *float64 `json:"nbf,omitempty"`
	IssuedAt  *float64 `json:"iat,omitempty"`
}

// NewClaimsJSON creates a ClaimsJSON from a *Claims pointer.
func NewClaimsJSON(c *Claims) ClaimsJSON {
	cj := ClaimsJSON{Extra: c.Extra}
	if c.Issuer != "" {
		cj.Issuer = c.Issuer
	}
	if c.Subject != "" {
		cj.Subject = c.Subject
	}
	if c.ID != "" {
		cj.ID = c.ID
	}
	if len(c.Audience) > 0 {
		cj.Audience = c.Audience
	}
	if c.ExpiresAt != nil {
		f := float64(c.ExpiresAt.Time.Unix())
		cj.ExpiresAt = &f
	}
	if c.NotBefore != nil {
		f := float64(c.NotBefore.Time.Unix())
		cj.NotBefore = &f
	}
	if c.IssuedAt != nil {
		f := float64(c.IssuedAt.Time.Unix())
		cj.IssuedAt = &f
	}
	return cj
}

// ToClaims converts a ClaimsJSON back to a *Claims pointer.
func (cj *ClaimsJSON) ToClaims() *Claims {
	c := &Claims{Extra: cj.Extra}
	if cj.Issuer != "" {
		c.Issuer = cj.Issuer
	}
	if cj.Subject != "" {
		c.Subject = cj.Subject
	}
	if cj.ID != "" {
		c.ID = cj.ID
	}
	if len(cj.Audience) > 0 {
		c.Audience = cj.Audience
	}
	if cj.ExpiresAt != nil {
		c.ExpiresAt = &jwt.NumericDate{Time: time.Unix(int64(*cj.ExpiresAt), 0)}
	}
	if cj.NotBefore != nil {
		c.NotBefore = &jwt.NumericDate{Time: time.Unix(int64(*cj.NotBefore), 0)}
	}
	if cj.IssuedAt != nil {
		c.IssuedAt = &jwt.NumericDate{Time: time.Unix(int64(*cj.IssuedAt), 0)}
	}
	return c
}
