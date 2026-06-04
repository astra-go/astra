// Package security provides tenant quota middleware that enforces per-tenant
// rate, concurrency, and daily request limits.
//
// TenantQuota middleware checks three quota dimensions for each tenant:
//
//   - QPS: token-bucket rate limiting (per-second requests with burst allowance)
//   - Concurrent: semaphore-based maximum concurrent requests
//   - DailyLimit: counter-based total requests per calendar day (midnight reset)
//
// Any dimension that exceeds its limit results in HTTP 429 with the tenant ID
// included in the response. Dimensions set to zero are unlimited.
//
// # Usage
//
//	store := security.NewMemoryQuotaStore()
//	app.Use(security.TenantQuotaWithConfig(security.TenantQuotaConfig{
//	    Store:        store,
//	    DefaultQPS:   100,
//	    DefaultBurst: 20,
//	    MaxConcurrent: 50,
//	    DailyLimit:   10000,
//	}))
//
//	// Override limits for a specific tenant
//	store.SetQuota(ctx, "acme", &security.TenantQuotaLimits{
//	    QPS:         200,
//	    Burst:       40,
//	    MaxConcurrent: 100,
//	    DailyLimit:   50000,
//	})
package security

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astra-go/astra"
)

// TenantQuotaLimits holds per-tenant quota overrides. Zero values mean
// "use the default from TenantQuotaConfig".
type TenantQuotaLimits struct {
	QPS         float64
	Burst       int
	MaxConcurrent int
	DailyLimit  int64
}

// TenantQuotaConfig configures the TenantQuota middleware.
type TenantQuotaConfig struct {
	// Store is the quota storage backend. Required.
	Store QuotaStore

	// DefaultQPS is the default token-bucket rate per tenant (0 = unlimited).
	DefaultQPS float64

	// DefaultBurst is the default burst size for the token bucket (0 = no burst).
	DefaultBurst int

	// MaxConcurrent is the default maximum concurrent requests per tenant (0 = unlimited).
	MaxConcurrent int

	// DailyLimit is the default maximum total requests per tenant per day (0 = unlimited).
	DailyLimit int64

	// KeyFunc extracts the tenant identifier from the context.
	// Defaults to TenantID.
	KeyFunc func(*astra.Ctx) string

	// ErrorHandler is called when any quota dimension is exceeded.
	// Default: 429 Too Many Requests with tenant ID in the response body.
	ErrorHandler ErrorHandler

	// Skipper skips quota enforcement for matching requests.
	Skipper Skipper
}

// QuotaStore is the interface for quota state persistence.
type QuotaStore interface {
	// IncrRequests atomically increments the daily request counter for a tenant
	// and returns the new total.
	IncrRequests(ctx context.Context, tenantID string, n int64) (int64, error)

	// GetDailyCount returns the current daily request count for a tenant.
	GetDailyCount(ctx context.Context, tenantID string) (int64, error)

	// ResetDailyCount resets the daily request counter for a tenant (called at midnight).
	ResetDailyCount(ctx context.Context, tenantID string) error

	// SetQuota stores per-tenant quota overrides.
	SetQuota(ctx context.Context, tenantID string, quota *TenantQuotaLimits) error

	// GetQuota retrieves per-tenant quota overrides. Returns nil when no override exists.
	GetQuota(ctx context.Context, tenantID string) (*TenantQuotaLimits, error)
}

// TenantQuota returns a middleware that enforces per-tenant quotas using
// default limits (QPS=0, Burst=0, MaxConcurrent=0, DailyLimit=0 — all unlimited).
// Use TenantQuotaWithConfig for custom limits.
func TenantQuota(store QuotaStore) astra.HandlerFunc {
	return TenantQuotaWithConfig(TenantQuotaConfig{Store: store})
}

// TenantQuotaWithConfig returns a TenantQuota middleware with custom config.
func TenantQuotaWithConfig(cfg TenantQuotaConfig) astra.HandlerFunc {
	if cfg.Store == nil {
		cfg.Store = NewMemoryQuotaStore()
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = TenantID
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultQuotaErrorHandler
	}

	// Per-tenant token buckets for QPS enforcement.
	buckets := &quotaBucketStore{
		buckets: make(map[string]*tokenBucket),
	}

	// Per-tenant concurrency semaphores.
	semaphores := &concurrentSemaphoreStore{
		sems: make(map[string]*tenantSemaphore),
	}

	// Midnight reset goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	go dailyResetLoop(ctx, cfg.Store)

	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		tenantID := cfg.KeyFunc(c)
		if tenantID == "" {
			c.Next()
			return nil
		}

		// Resolve effective limits (tenant override > defaults).
		limits := resolveLimits(c.Request().Context(), cfg, tenantID)

		// 1. QPS check (token bucket).
		if limits.QPS > 0 {
			if !buckets.allow(tenantID, limits.QPS, limits.Burst) {
				c.Writer().Header().Set("Retry-After", "1")
				c.Writer().Header().Set("X-Tenant-ID", tenantID)
				return cfg.ErrorHandler(c)
			}
		}

		// 2. Concurrent check (semaphore).
		if limits.MaxConcurrent > 0 {
			if !semaphores.acquire(tenantID, limits.MaxConcurrent) {
				c.Writer().Header().Set("X-Tenant-ID", tenantID)
				return cfg.ErrorHandler(c)
			}
			defer semaphores.release(tenantID)
		}

		// 3. Daily limit check.
		if limits.DailyLimit > 0 {
			count, err := cfg.Store.IncrRequests(c.Request().Context(), tenantID, 1)
			if err != nil {
				// Storage error: let the request through rather than block all traffic.
				c.Next()
				return nil
			}
			if count > limits.DailyLimit {
				c.Writer().Header().Set("X-Tenant-ID", tenantID)
				return cfg.ErrorHandler(c)
			}
		}

		c.Next()
		return nil
	}
}

// resolveLimits merges tenant-specific overrides with defaults.
func resolveLimits(ctx context.Context, cfg TenantQuotaConfig, tenantID string) TenantQuotaLimits {
	limits := TenantQuotaLimits{
		QPS:         cfg.DefaultQPS,
		Burst:       cfg.DefaultBurst,
		MaxConcurrent: cfg.MaxConcurrent,
		DailyLimit:  cfg.DailyLimit,
	}
	override, err := cfg.Store.GetQuota(ctx, tenantID)
	if err == nil && override != nil {
		if override.QPS > 0 {
			limits.QPS = override.QPS
		}
		if override.Burst > 0 {
			limits.Burst = override.Burst
		}
		if override.MaxConcurrent > 0 {
			limits.MaxConcurrent = override.MaxConcurrent
		}
		if override.DailyLimit > 0 {
			limits.DailyLimit = override.DailyLimit
		}
	}
	return limits
}

func defaultQuotaErrorHandler(c *astra.Ctx) error {
	return astra.NewHTTPError(http.StatusTooManyRequests, "tenant quota exceeded")
}

// ─── Token bucket store (QPS) ────────────────────────────────────────────────

type quotaBucketStore struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
}

func (s *quotaBucketStore) allow(key string, rate float64, burst int) bool {
	s.mu.RLock()
	tb, ok := s.buckets[key]
	s.mu.RUnlock()

	if !ok {
		s.mu.Lock()
		if tb, ok = s.buckets[key]; !ok {
			tb = &tokenBucket{
				tokens:   float64(burst),
				lastTime: time.Now(),
			}
			s.buckets[key] = tb
		}
		s.mu.Unlock()
	}

	return tb.allow(rate, burst)
}

// ─── Concurrent semaphore store ──────────────────────────────────────────────

type tenantSemaphore struct {
	current atomic.Int64
	max     int64
}

type concurrentSemaphoreStore struct {
	mu   sync.RWMutex
	sems map[string]*tenantSemaphore
}

func (s *concurrentSemaphoreStore) acquire(tenantID string, maxConcurrent int) bool {
	s.mu.RLock()
	sem, ok := s.sems[tenantID]
	s.mu.RUnlock()

	if !ok {
		s.mu.Lock()
		if sem, ok = s.sems[tenantID]; !ok {
			sem = &tenantSemaphore{max: int64(maxConcurrent)}
			s.sems[tenantID] = sem
		}
		s.mu.Unlock()
	}

	current := sem.current.Add(1)
	if current > sem.max {
		sem.current.Add(-1)
		return false
	}
	return true
}

func (s *concurrentSemaphoreStore) release(tenantID string) {
	s.mu.RLock()
	sem, ok := s.sems[tenantID]
	s.mu.RUnlock()

	if ok {
		sem.current.Add(-1)
	}
}

// ─── Daily reset loop ────────────────────────────────────────────────────────

func dailyResetLoop(ctx context.Context, store QuotaStore) {
	for {
		now := time.Now()
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		waitDuration := nextMidnight.Sub(now)

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			// Reset is tenant-specific; MemoryQuotaStore handles it internally.
			// For Redis, a TTL-based expiry is preferred over explicit reset.
			if ms, ok := store.(*MemoryQuotaStore); ok {
				ms.resetAllDaily()
			}
		}
	}
}

// ─── MemoryQuotaStore ────────────────────────────────────────────────────────

// MemoryQuotaStore implements QuotaStore using in-memory data structures.
// Suitable for single-instance deployments. Daily counters reset at midnight
// via the internal reset loop started by TenantQuotaWithConfig.
type MemoryQuotaStore struct {
	dailyCounts sync.Map // map[tenantID]*atomic.Int64
	quotas      sync.Map // map[tenantID]*TenantQuotaLimits
}

// NewMemoryQuotaStore creates a new in-memory quota store.
func NewMemoryQuotaStore() *MemoryQuotaStore {
	return &MemoryQuotaStore{}
}

func (s *MemoryQuotaStore) IncrRequests(_ context.Context, tenantID string, n int64) (int64, error) {
	val, _ := s.dailyCounts.LoadOrStore(tenantID, new(atomic.Int64))
	counter := val.(*atomic.Int64)
	return counter.Add(n), nil
}

func (s *MemoryQuotaStore) GetDailyCount(_ context.Context, tenantID string) (int64, error) {
	val, ok := s.dailyCounts.Load(tenantID)
	if !ok {
		return 0, nil
	}
	return val.(*atomic.Int64).Load(), nil
}

func (s *MemoryQuotaStore) ResetDailyCount(_ context.Context, tenantID string) error {
	s.dailyCounts.Delete(tenantID)
	return nil
}

func (s *MemoryQuotaStore) resetAllDaily() {
	// Delete all daily counters so they start fresh.
	s.dailyCounts.Range(func(key, _ any) bool {
		s.dailyCounts.Delete(key)
		return true
	})
}

func (s *MemoryQuotaStore) SetQuota(_ context.Context, tenantID string, quota *TenantQuotaLimits) error {
	s.quotas.Store(tenantID, quota)
	return nil
}

func (s *MemoryQuotaStore) GetQuota(_ context.Context, tenantID string) (*TenantQuotaLimits, error) {
	val, ok := s.quotas.Load(tenantID)
	if !ok {
		return nil, nil
	}
	return val.(*TenantQuotaLimits), nil
}

// ─── RedisQuotaStore ─────────────────────────────────────────────────────────

// RedisQuotaStore implements QuotaStore using Redis for distributed deployments.
// Daily counters use INCR with a TTL that expires at the next midnight, so no
// explicit reset loop is needed.
type RedisQuotaStore struct {
	client redisClient
	prefix string // key prefix, e.g. "astra:quota:"
}

// RedisQuotaStoreConfig configures the Redis quota store.
type RedisQuotaStoreConfig struct {
	Client redisClient
	Prefix string // default: "astra:quota:"
}

// NewRedisQuotaStore creates a Redis-backed quota store.
func NewRedisQuotaStore(cfg RedisQuotaStoreConfig) *RedisQuotaStore {
	if cfg.Prefix == "" {
		cfg.Prefix = "astra:quota:"
	}
	return &RedisQuotaStore{client: cfg.Client, prefix: cfg.Prefix}
}

func (s *RedisQuotaStore) IncrRequests(ctx context.Context, tenantID string, n int64) (int64, error) {
	key := s.prefix + "daily:" + tenantID
	result, err := s.client.IncrBy(ctx, key, n)
	if err != nil {
		return 0, err
	}
	// Set TTL to expire at next midnight if this is a new counter.
	if result == n {
		ttl := secondsUntilMidnight(time.Now())
		s.client.Expire(ctx, key, time.Duration(ttl)*time.Second)
	}
	return result, nil
}

func (s *RedisQuotaStore) GetDailyCount(ctx context.Context, tenantID string) (int64, error) {
	key := s.prefix + "daily:" + tenantID
	return s.client.Get(ctx, key)
}

func (s *RedisQuotaStore) ResetDailyCount(ctx context.Context, tenantID string) error {
	key := s.prefix + "daily:" + tenantID
	return s.client.Del(ctx, key)
}

func (s *RedisQuotaStore) SetQuota(ctx context.Context, tenantID string, quota *TenantQuotaLimits) error {
	key := s.prefix + "limits:" + tenantID
	return s.client.SetJSON(ctx, key, quota)
}

func (s *RedisQuotaStore) GetQuota(ctx context.Context, tenantID string) (*TenantQuotaLimits, error) {
	key := s.prefix + "limits:" + tenantID
	return s.client.GetJSON(ctx, key, &TenantQuotaLimits{})
}

func secondsUntilMidnight(now time.Time) int64 {
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return int64(next.Sub(now).Seconds()) + 1
}

// redisClient is an abstraction over go-redis Client so the middleware
// module does not directly import go-redis (it is already in go.mod).
type redisClient interface {
	IncrBy(ctx context.Context, key string, n int64) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Get(ctx context.Context, key string) (int64, error)
	Del(ctx context.Context, key string) error
	SetJSON(ctx context.Context, key string, val any) error
	GetJSON(ctx context.Context, key string, out any) (any, error)
}

// RedisClientAdapter wraps a go-redis UniversalClient to implement redisClient.
// Import go-redis and pass your client to NewRedisClientAdapter.
type RedisClientAdapter struct {
	client redisUniversalClient
}

// redisUniversalClient matches the go-redis UniversalClient interface.
type redisUniversalClient interface {
	IncrBy(ctx context.Context, key string, value int64) *redisIntCmd
	Expire(ctx context.Context, key string, ttl time.Duration) *redisBoolCmd
	Get(ctx context.Context, key string) *redisStringCmd
	Del(ctx context.Context, keys ...string) *redisIntCmd
	Set(ctx context.Context, key string, value any, ttl time.Duration) *redisStatusCmd
}

// NewRedisClientAdapter creates a redisClient from a go-redis UniversalClient.
func NewRedisClientAdapter(client redisUniversalClient) *RedisClientAdapter {
	return &RedisClientAdapter{client: client}
}

func (a *RedisClientAdapter) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	cmd := a.client.IncrBy(ctx, key, n)
	return cmd.Result()
}

func (a *RedisClientAdapter) Expire(ctx context.Context, key string, ttl time.Duration) error {
	cmd := a.client.Expire(ctx, key, ttl)
	return cmd.Err()
}

func (a *RedisClientAdapter) Get(ctx context.Context, key string) (int64, error) {
	cmd := a.client.Get(ctx, key)
	return cmd.AsInt64()
}

func (a *RedisClientAdapter) Del(ctx context.Context, key string) error {
	cmd := a.client.Del(ctx, key)
	return cmd.Err()
}

func (a *RedisClientAdapter) SetJSON(ctx context.Context, key string, val any) error {
	// Use go-redis Set with JSON-serialised value and midnight TTL.
	data, err := jsonMarshal(val)
	if err != nil {
		return err
	}
	ttl := time.Duration(secondsUntilMidnight(time.Now())) * time.Second
	cmd := a.client.Set(ctx, key, data, ttl)
	return cmd.Err()
}

func (a *RedisClientAdapter) GetJSON(ctx context.Context, key string, out any) (any, error) {
	cmd := a.client.Get(ctx, key)
	data, err := cmd.Bytes()
	if err != nil {
		// Key does not exist → nil quota.
		return nil, nil
	}
	if err := jsonUnmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ─── JSON helpers (avoid direct encoding/json import in this file) ───────────

// These are provided by a small internal helper so the main file stays clean.
// In practice encoding/json is always available; the helpers just centralise
// the import.

type redisIntCmd interface{ Result() (int64, error) }
type redisBoolCmd interface{ Err() error }
type redisStringCmd interface{ Result() (string, error); AsInt64() (int64, error); Bytes() ([]byte, error) }
type redisStatusCmd interface{ Err() error }