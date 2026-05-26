//go:build redis

package security

import (
	"context"
	"sync/atomic"
	"time"
)

// MultiLevelJWTCache is a two-tier JWT cache: L1 (in-process sharded map) backed
// by L2 (Redis). On a Get, L1 is checked first; on a miss, L2 is queried and the
// result is promoted to L1. On a Set, both tiers are written. On a Delete, both
// tiers are invalidated.
//
// This eliminates redundant Redis round-trips for hot tokens while keeping
// multi-instance deployments consistent via the shared L2 store.
//
// Automatic expiry:
//   - L1 entries carry their own expireAt and are evicted lazily on access or
//     when the shard is full (same policy as the standalone jwtCache).
//   - L2 entries are evicted by Redis TTL.
//
// Fail-open: L2 errors are non-fatal; the cache falls through to JWT parsing.
type MultiLevelJWTCache struct {
	l1     *jwtCache
	l2     *RedisJWTCache
	hits   atomic.Int64 // L1 hits
	l2hits atomic.Int64 // L2 hits (promoted to L1)
	misses atomic.Int64 // full misses
}

// NewMultiLevelJWTCache creates a two-level JWT cache.
//
//	// 1 024-entry L1 + shared Redis L2
//	cache := middleware.NewMultiLevelJWTCache(
//	    1024,
//	    middleware.NewRedisJWTCache(redisClient),
//	)
//	app.Use(middleware.JWTWithConfig(middleware.JWTConfig{
//	    Secret:       "my-secret",
//	    CacheBackend: cache,
//	}))
func NewMultiLevelJWTCache(l1MaxSize int, l2 *RedisJWTCache) *MultiLevelJWTCache {
	if l2 == nil {
		panic("jwt_cache_multilevel: l2 must not be nil")
	}
	if l1MaxSize <= 0 {
		l1MaxSize = 512
	}
	return &MultiLevelJWTCache{
		l1: newJWTCache(l1MaxSize),
		l2: l2,
	}
}

// Get checks L1 first, then L2 on a miss. An L2 hit is promoted to L1.
// Returns (nil, false) when both tiers miss or on any L2 error.
func (m *MultiLevelJWTCache) Get(ctx context.Context, sig string) (*Claims, bool) {
	now := time.Now().Unix()

	// L1 check — no network, no allocation
	if claims, ok := m.l1.get(sig, now); ok {
		m.hits.Add(1)
		return claims, true
	}

	// L2 check — Redis round-trip
	claims, ok := m.l2.Get(ctx, sig)
	if !ok {
		m.misses.Add(1)
		return nil, false
	}
	m.l2hits.Add(1)

	// Promote to L1: derive expireAt from the claims ExpiresAt field.
	// If ExpiresAt is absent the entry is not promoted (no TTL to derive).
	if claims.ExpiresAt != nil {
		m.l1.set(sig, claims, claims.ExpiresAt.Unix(), now)
	}
	return claims, true
}

// Set writes to both L1 and L2.
func (m *MultiLevelJWTCache) Set(ctx context.Context, sig string, claims *Claims, expireAt int64) {
	now := time.Now().Unix()
	m.l1.set(sig, claims, expireAt, now)
	m.l2.Set(ctx, sig, claims, expireAt)
}

// Delete removes the entry from both L1 and L2. Use this after revoking a token.
// L2 deletion errors are returned; L1 deletion is always synchronous and infallible.
func (m *MultiLevelJWTCache) Delete(ctx context.Context, sig string) error {
	m.l1.delete(sig)
	return m.l2.Delete(ctx, sig)
}

// Stats returns a snapshot of cache hit/miss counters.
func (m *MultiLevelJWTCache) Stats() MultiLevelCacheStats {
	return MultiLevelCacheStats{
		L1Hits: m.hits.Load(),
		L2Hits: m.l2hits.Load(),
		Misses: m.misses.Load(),
	}
}

// MultiLevelCacheStats is a point-in-time snapshot of cache counters.
type MultiLevelCacheStats struct {
	L1Hits int64
	L2Hits int64
	Misses int64
}
