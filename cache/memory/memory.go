// Package memory provides a thread-safe in-process LRU cache that implements
// the cache.Cache interface.
//
// Eviction policy: Least Recently Used (LRU). When the cache reaches its
// capacity, the entry that was least recently read or written is evicted first.
// Expired entries that are discovered on access are removed immediately (lazy
// eviction). A background goroutine periodically removes all remaining expired
// entries so they do not consume capacity.
//
// # Usage
//
//	import cachemem "github.com/astra-go/astra/cache/memory"
//
//	// Unbounded — TTL-based eviction only (development / testing)
//	c := cachemem.New()
//	defer c.Close()
//
//	// Bounded — evict LRU when capacity is reached
//	c := cachemem.New(cachemem.Config{Cap: 1000})
//	defer c.Close()
//
//	c.Set(ctx, "user:1", data, 5*time.Minute)
//	val, err := c.Get(ctx, "user:1")
//
// # JSON helpers (from parent package)
//
//	cache.SetJSON(ctx, c, "user:1", &user, time.Hour)
//
//	var user User
//	cache.GetJSON(ctx, c, "user:1", &user)
package memory

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/astra-go/astra/cache"
)

const defaultCleanupInterval = 5 * time.Minute

// Config configures the LRU cache.
type Config struct {
	// Cap is the maximum number of entries the cache holds.
	// When this limit is reached, the least recently used entry is evicted to
	// make room for the new one.
	// 0 means unlimited — the cache grows without bound (TTL-only eviction).
	Cap int

	// CleanupInterval controls how often the background goroutine scans for and
	// removes expired entries. Default: 5 minutes.
	CleanupInterval time.Duration
}

// Cache is a thread-safe, bounded LRU cache with per-entry TTL expiry.
// All exported methods are safe for concurrent use.
type Cache struct {
	mu       sync.Mutex
	cap      int
	items    map[string]*list.Element
	lru      *list.List // front = MRU (most recent), back = LRU (least recent)
	done     chan struct{}
	stopOnce sync.Once
}

type entry struct {
	key     string
	value   []byte
	expires time.Time // zero means the entry never expires
}

// New creates a new LRU Cache. Pass an optional Config to set capacity or
// override the cleanup interval.
func New(cfgs ...Config) *Cache {
	cfg := Config{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = defaultCleanupInterval
	}

	c := &Cache{
		cap:   cfg.Cap,
		items: make(map[string]*list.Element),
		lru:   list.New(),
		done:  make(chan struct{}),
	}
	go c.cleanupLoop(cfg.CleanupInterval)
	return c
}

// Get retrieves the value for key and promotes it to the front of the LRU list.
// Returns cache.ErrCacheMiss if the key is absent or has expired.
func (c *Cache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return nil, cache.ErrCacheMiss
	}
	e := el.Value.(*entry)
	if isExpired(e) {
		c.remove(el)
		return nil, cache.ErrCacheMiss
	}
	c.lru.MoveToFront(el)
	// Return a defensive copy — callers must not mutate cached bytes.
	return append([]byte(nil), e.value...), nil
}

// Set stores a copy of value under key with the given TTL.
// A TTL of 0 means the entry never expires.
// If the key already exists, its value and TTL are updated in place.
// When the cache is at capacity, the least recently used entry is evicted first.
func (c *Cache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	var expires time.Time
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	stored := append([]byte(nil), value...)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry — move to front, refresh value and TTL.
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry)
		e.value = stored
		e.expires = expires
		c.lru.MoveToFront(el)
		return nil
	}

	// Evict if at capacity.
	if c.cap > 0 && len(c.items) >= c.cap {
		c.evictOne()
	}

	el := c.lru.PushFront(&entry{key: key, value: stored, expires: expires})
	c.items[key] = el
	return nil
}

// Delete removes the given keys. Missing keys are silently ignored.
func (c *Cache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	for _, k := range keys {
		if el, ok := c.items[k]; ok {
			c.remove(el)
		}
	}
	c.mu.Unlock()
	return nil
}

// Exists reports whether key exists and has not expired.
// Unlike Get, Exists does not promote the entry in the LRU order.
func (c *Cache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return false, nil
	}
	e := el.Value.(*entry)
	if isExpired(e) {
		c.remove(el)
		return false, nil
	}
	return true, nil
}

// Flush removes all entries from the cache.
func (c *Cache) Flush(_ context.Context) error {
	c.mu.Lock()
	c.items = make(map[string]*list.Element)
	c.lru.Init()
	c.mu.Unlock()
	return nil
}

// Close stops the background cleanup goroutine and releases resources.
// The cache must not be used after Close returns.
func (c *Cache) Close() error {
	c.stopOnce.Do(func() { close(c.done) })
	return nil
}

// Len returns the current number of entries (including not-yet-evicted expired
// entries that have not been accessed since expiry).
func (c *Cache) Len() int {
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	return n
}

// Cap returns the configured capacity (0 means unlimited).
func (c *Cache) Cap() int { return c.cap }

// ─── internal ─────────────────────────────────────────────────────────────────

func isExpired(e *entry) bool {
	return !e.expires.IsZero() && time.Now().After(e.expires)
}

// remove deletes el from both the hash map and the LRU list.
// Must be called with c.mu held.
func (c *Cache) remove(el *list.Element) {
	e := el.Value.(*entry)
	delete(c.items, e.key)
	c.lru.Remove(el)
}

// evictOne removes a single entry to free capacity.
// Prefers evicting an already-expired entry; falls back to the true LRU.
// Must be called with c.mu held.
func (c *Cache) evictOne() {
	// Fast path: if the LRU tail is expired, evict it.
	if back := c.lru.Back(); back != nil {
		if isExpired(back.Value.(*entry)) {
			c.remove(back)
			return
		}
	}
	// Scan for any expired entry — avoids discarding live data unnecessarily.
	for el := c.lru.Back(); el != nil; el = el.Prev() {
		if isExpired(el.Value.(*entry)) {
			c.remove(el)
			return
		}
	}
	// All entries are live — evict the least recently used.
	if back := c.lru.Back(); back != nil {
		c.remove(back)
	}
}

func (c *Cache) cleanupLoop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			c.purgeExpired()
		}
	}
}

func (c *Cache) purgeExpired() {
	now := time.Now()
	c.mu.Lock()
	for _, el := range c.items {
		e := el.Value.(*entry)
		if !e.expires.IsZero() && now.After(e.expires) {
			c.remove(el)
		}
	}
	c.mu.Unlock()
}

// Compile-time assertion: Cache must implement cache.Cache.
var _ cache.Cache = (*Cache)(nil)
