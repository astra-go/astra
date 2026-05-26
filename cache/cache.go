// Package cache provides a unified caching abstraction for Astra applications.
//
// All implementations satisfy the Cache interface, making it trivial to swap
// backends (in-process memory → Redis → Memcached) without changing application
// code.
//
// # In-memory cache (single-instance / testing)
//
//	c := cache.NewMemory()
//	defer c.Close()
//
//	c.Set(ctx, "key", []byte("hello"), time.Minute)
//	val, err := c.Get(ctx, "key")
//
// # Redis cache
//
//	import cacheredis "github.com/astra-go/astra/cache/redis"
//
//	c, err := cacheredis.New(cacheredis.Config{Addr: "localhost:6379"})
//
// # JSON helpers
//
//	type User struct { Name string }
//	cache.SetJSON(ctx, c, "user:1", &User{Name: "alice"}, time.Hour)
//
//	var u User
//	cache.GetJSON(ctx, c, "user:1", &u)
//
// # Read-through pattern
//
//	var u User
//	cache.GetOrSet(ctx, c, "user:1", &u, time.Hour, func() (any, error) {
//	    return db.FindUser(1)
//	})
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// ErrCacheMiss is returned when the requested key is not found or has expired.
var ErrCacheMiss = errors.New("cache: key not found")

// Cache is the unified cache interface. All implementations must be safe for
// concurrent use from multiple goroutines.
type Cache interface {
	// Get retrieves the byte value for key.
	// Returns ErrCacheMiss if the key does not exist or has expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores value with the given key and TTL.
	// A TTL of 0 means the entry never expires.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes the given keys. Missing keys are silently ignored.
	Delete(ctx context.Context, keys ...string) error

	// Exists reports whether key exists and has not expired.
	Exists(ctx context.Context, key string) (bool, error)

	// Flush removes all keys. Use with caution in shared caches.
	Flush(ctx context.Context) error

	// Close releases any resources held by the cache client.
	Close() error
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

// GetJSON deserialises the cached value into v.
// Returns ErrCacheMiss when the key does not exist.
func GetJSON(ctx context.Context, c Cache, key string, v any) error {
	b, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// SetJSON serialises v to JSON and stores it with the given TTL.
func SetJSON(ctx context.Context, c Cache, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, b, ttl)
}

// GetOrSet returns a cached value, calling fetch on a cache miss to populate it.
// The result of fetch is marshalled to JSON, stored in the cache, then
// unmarshalled into v. A failed Set is non-fatal — v is still populated.
//
// Security: keys derived from user input must be sanitised by the caller to
// prevent cache-key injection (e.g. path traversal in prefixed key schemes).
func GetOrSet(ctx context.Context, c Cache, key string, v any, ttl time.Duration, fetch func() (any, error)) error {
	if err := GetJSON(ctx, c, key, v); err == nil {
		return nil // cache hit
	} else if !errors.Is(err, ErrCacheMiss) {
		return err // unexpected cache error
	}

	result, err := fetch()
	if err != nil {
		return err
	}

	b, err := json.Marshal(result)
	if err != nil {
		return err
	}

	// Non-fatal: a Set failure shouldn't prevent the caller from receiving data.
	_ = c.Set(ctx, key, b, ttl)

	return json.Unmarshal(b, v)
}

// ─── In-memory cache ──────────────────────────────────────────────────────────

// MemoryCache is a thread-safe in-process cache with per-entry TTL eviction.
// A background goroutine periodically removes expired entries; expired entries
// are also removed lazily on access.
//
// MemoryCache is suitable for development, testing, and single-replica services.
// It does not share state across processes or hosts.
//
// Security: cache keys and values are stored in plain memory. Avoid caching
// secrets (tokens, passwords) that should not outlive the request.
type MemoryCache struct {
	mu       sync.RWMutex
	items    map[string]memItem
	done     chan struct{}
	stopOnce sync.Once
}

type memItem struct {
	value   []byte
	expires time.Time // zero value means never expires
}

const defaultCleanupInterval = 5 * time.Minute

// NewMemory creates a MemoryCache with the default cleanup interval (5 minutes).
func NewMemory() *MemoryCache {
	return newMemory(defaultCleanupInterval)
}

// NewMemoryWithInterval creates a MemoryCache with a custom cleanup interval.
// Shorter intervals keep memory lower; longer intervals reduce CPU overhead.
// Minimum enforced: 1 second.
func NewMemoryWithInterval(interval time.Duration) *MemoryCache {
	if interval < time.Second {
		interval = time.Second
	}
	return newMemory(interval)
}

func newMemory(interval time.Duration) *MemoryCache {
	c := &MemoryCache{
		items: make(map[string]memItem),
		done:  make(chan struct{}),
	}
	go c.cleanupLoop(interval)
	return c
}

// Get retrieves the cached value for key.
// Returns ErrCacheMiss if the key does not exist or has expired.
func (c *MemoryCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return nil, ErrCacheMiss
	}
	if !item.expires.IsZero() && time.Now().After(item.expires) {
		// Lazy eviction on access.
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, ErrCacheMiss
	}
	// Return a defensive copy so callers cannot modify the stored slice.
	return append([]byte(nil), item.value...), nil
}

// Set stores a defensive copy of value. TTL of 0 means no expiry.
func (c *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var expires time.Time
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	stored := append([]byte(nil), value...)

	c.mu.Lock()
	c.items[key] = memItem{value: stored, expires: expires}
	c.mu.Unlock()
	return nil
}

// Delete removes the given keys. Missing keys are silently ignored.
func (c *MemoryCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	for _, k := range keys {
		delete(c.items, k)
	}
	c.mu.Unlock()
	return nil
}

// Exists reports whether key exists and has not expired.
func (c *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.Get(ctx, key)
	if errors.Is(err, ErrCacheMiss) {
		return false, nil
	}
	return err == nil, err
}

// Flush removes all entries from the cache.
func (c *MemoryCache) Flush(_ context.Context) error {
	c.mu.Lock()
	c.items = make(map[string]memItem)
	c.mu.Unlock()
	return nil
}

// Close stops the background cleanup goroutine and releases resources.
// The MemoryCache must not be used after Close is called.
func (c *MemoryCache) Close() error {
	c.stopOnce.Do(func() { close(c.done) })
	return nil
}

func (c *MemoryCache) cleanupLoop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			c.evictExpired()
		}
	}
}

func (c *MemoryCache) evictExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, item := range c.items {
		if !item.expires.IsZero() && now.After(item.expires) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}
