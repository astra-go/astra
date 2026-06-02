package security

import (
	"context"
	"strings"
	"sync"
	"time"
)

const jwtCacheShards = 16

type jwtCacheEntry struct {
	claims   *Claims
	expireAt int64 // Unix seconds — 80 % of remaining token validity at insertion time
}

type jwtCacheShard struct {
	mu      sync.RWMutex
	entries map[string]jwtCacheEntry
	maxSize int
}

type jwtCache struct {
	shards [jwtCacheShards]jwtCacheShard
}

// ensure jwtCache satisfies JWTCacheBackend
var _ JWTCacheBackend = (*jwtCache)(nil)

func (c *jwtCache) Get(_ context.Context, sig string) (*Claims, bool) {
	now := time.Now().Unix()
	return c.get(sig, now)
}

func (c *jwtCache) Set(_ context.Context, sig string, claims *Claims, expireAt int64) {
	now := time.Now().Unix()
	c.set(sig, claims, expireAt, now)
}

func newJWTCache(maxTotal int) *jwtCache {
	perShard := max(maxTotal/jwtCacheShards, 1)
	c := &jwtCache{}
	for i := range c.shards {
		c.shards[i].entries = make(map[string]jwtCacheEntry, perShard)
		c.shards[i].maxSize = perShard
	}
	return c
}

// tokenSignature returns the signature segment of a JWT (the last dot-separated field).
// Using only the signature as cache key reduces key length from ~200 chars to ~43 chars
// (HS256), speeding up FNV-1a hash computation and map key comparison.
func tokenSignature(raw string) string {
	i := strings.LastIndexByte(raw, '.')
	if i < 0 {
		return raw
	}
	return raw[i+1:]
}

func (c *jwtCache) shardFor(sig string) *jwtCacheShard {
	var h uint32 = 2166136261
	for i := 0; i < len(sig); i++ {
		h ^= uint32(sig[i])
		h *= 16777619
	}
	return &c.shards[h%jwtCacheShards]
}

func (c *jwtCache) get(sig string, now int64) (*Claims, bool) {
	sh := c.shardFor(sig)
	sh.mu.RLock()
	e, ok := sh.entries[sig]
	sh.mu.RUnlock()
	if !ok || e.expireAt <= now {
		return nil, false
	}
	return e.claims, true
}

// set stores claims keyed by the token's signature segment.
// cacheUntil is computed as now + (expireAt-now)*4/5 so the entry is evicted
// before the token actually expires, preventing stale-claim hits on edge cases.
func (c *jwtCache) set(sig string, claims *Claims, expireAt, now int64) {
	cacheUntil := now + (expireAt-now)*4/5
	if cacheUntil <= now {
		return // token expires so soon it is not worth caching
	}
	sh := c.shardFor(sig)
	sh.mu.Lock()
	if len(sh.entries) >= sh.maxSize {
		evictJWTCacheShard(sh)
	}
	sh.entries[sig] = jwtCacheEntry{claims: claims, expireAt: cacheUntil}
	sh.mu.Unlock()
}

func (c *jwtCache) Delete(_ context.Context, sig string) error {
	sh := c.shardFor(sig)
	sh.mu.Lock()
	delete(sh.entries, sig)
	sh.mu.Unlock()
	return nil
}

// evictJWTCacheShard removes all expired entries; if still over half capacity,
// removes arbitrary entries until at half capacity. Caller must hold sh.mu.Lock().
func evictJWTCacheShard(sh *jwtCacheShard) {
	now := time.Now().Unix()
	for k, e := range sh.entries {
		if e.expireAt <= now {
			delete(sh.entries, k)
		}
	}
	target := sh.maxSize / 2
	for k := range sh.entries {
		if len(sh.entries) <= target {
			break
		}
		delete(sh.entries, k)
	}
}
