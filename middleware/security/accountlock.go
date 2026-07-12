package security

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Default account lock parameters.
const (
	DefaultMaxFails        = 5
	DefaultLockoutDuration = 30 * time.Minute
	DefaultWindow          = 30 * time.Minute
)

// AccountLocker tracks login failures and enforces account lockouts.
// Implementations must be safe for concurrent use.
type AccountLocker interface {
	// IncrLoginFail increments the failure count for the given key.
	// Returns the new failure count.  When the count reaches the configured
	// threshold the account is automatically locked.
	IncrLoginFail(ctx context.Context, key string) (int64, error)

	// GetLoginFailCount returns the current failure count for the given key.
	GetLoginFailCount(ctx context.Context, key string) (int64, error)

	// ResetLoginFail clears the failure count for the given key.
	ResetLoginFail(ctx context.Context, key string) error

	// IsLocked reports whether the account is currently locked.
	IsLocked(ctx context.Context, key string) (bool, error)

	// LockAccount locks the account for the configured duration.
	LockAccount(ctx context.Context, key string) error
}

// ============================================================================
// Redis implementation
// ============================================================================

// AccountLockOption configures a RedisAccountLocker.
type AccountLockOption func(*lockerConfig)

type lockerConfig struct {
	maxFails        int
	lockoutDuration time.Duration
	window          time.Duration
	keyPrefix       string
}

// WithMaxFails sets the failure threshold before lockout (default 5).
func WithMaxFails(n int) AccountLockOption {
	return func(c *lockerConfig) { c.maxFails = n }
}

// WithLockoutDuration sets the duration an account stays locked (default 30 min).
func WithLockoutDuration(d time.Duration) AccountLockOption {
	return func(c *lockerConfig) { c.lockoutDuration = d }
}

// WithFailureWindow sets the sliding window for counting failures (default 30 min).
func WithFailureWindow(d time.Duration) AccountLockOption {
	return func(c *lockerConfig) { c.window = d }
}

// WithAccountLockPrefix sets the Redis key prefix (default "astra:accountlock:").
func WithAccountLockPrefix(prefix string) AccountLockOption {
	return func(c *lockerConfig) { c.keyPrefix = prefix }
}

// RedisAccountLocker implements AccountLocker using Redis.
type RedisAccountLocker struct {
	client          *redis.Client
	keyPrefix       string
	maxFails        int
	lockoutDuration time.Duration
	window          time.Duration
}

// NewRedisAccountLocker creates a Redis-backed account locker.
// Configure via AccountLockOption or use defaults.
//
//	locker := security.NewRedisAccountLocker(rdb,
//	    security.WithMaxFails(3),
//	    security.WithLockoutDuration(time.Hour),
//	    security.WithAccountLockPrefix("myapp:lock:"),
//	)
func NewRedisAccountLocker(client *redis.Client, opts ...AccountLockOption) *RedisAccountLocker {
	cfg := &lockerConfig{
		maxFails:        DefaultMaxFails,
		lockoutDuration: DefaultLockoutDuration,
		window:          DefaultWindow,
		keyPrefix:       "astra:accountlock:",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &RedisAccountLocker{
		client:          client,
		keyPrefix:       cfg.keyPrefix,
		maxFails:        cfg.maxFails,
		lockoutDuration: cfg.lockoutDuration,
		window:          cfg.window,
	}
}

func (r *RedisAccountLocker) failKey(key string) string { return r.keyPrefix + "fail:" + key }
func (r *RedisAccountLocker) lockKey(key string) string { return r.keyPrefix + "lock:" + key }

func (r *RedisAccountLocker) IncrLoginFail(ctx context.Context, key string) (int64, error) {
	fk := r.failKey(key)
	count, err := r.client.Incr(ctx, fk).Result()
	if err != nil {
		return 0, fmt.Errorf("incr fail count: %w", err)
	}
	if count == 1 {
		if err := r.client.Expire(ctx, fk, r.window).Err(); err != nil {
			return count, fmt.Errorf("set fail counter TTL: %w", err)
		}
	}
	if count >= int64(r.maxFails) {
		if err := r.LockAccount(ctx, key); err != nil {
			return count, fmt.Errorf("lock account: %w", err)
		}
	}
	return count, nil
}

func (r *RedisAccountLocker) GetLoginFailCount(ctx context.Context, key string) (int64, error) {
	val, err := r.client.Get(ctx, r.failKey(key)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get fail count: %w", err)
	}
	return val, nil
}

func (r *RedisAccountLocker) ResetLoginFail(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.failKey(key)).Err()
}

func (r *RedisAccountLocker) IsLocked(ctx context.Context, key string) (bool, error) {
	val, err := r.client.Exists(ctx, r.lockKey(key)).Result()
	if err != nil {
		return false, fmt.Errorf("check lock: %w", err)
	}
	return val > 0, nil
}

func (r *RedisAccountLocker) LockAccount(ctx context.Context, key string) error {
	return r.client.Set(ctx, r.lockKey(key), "1", r.lockoutDuration).Err()
}

// ============================================================================
// In-memory implementation (for development / testing)
// ============================================================================

// MemoryAccountLocker implements AccountLocker using in-memory state.
type MemoryAccountLocker struct {
	mu              sync.RWMutex
	fails           map[string]int64
	locks           map[string]time.Time
	maxFails        int
	lockoutDuration time.Duration
	window          time.Duration
}

// NewMemoryAccountLocker creates an in-memory account locker.
func NewMemoryAccountLocker(opts ...AccountLockOption) *MemoryAccountLocker {
	cfg := &lockerConfig{
		maxFails:        DefaultMaxFails,
		lockoutDuration: DefaultLockoutDuration,
		window:          DefaultWindow,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &MemoryAccountLocker{
		fails:           make(map[string]int64),
		locks:           make(map[string]time.Time),
		maxFails:        cfg.maxFails,
		lockoutDuration: cfg.lockoutDuration,
		window:          cfg.window,
	}
}

func (m *MemoryAccountLocker) IncrLoginFail(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fails[key]++
	count := m.fails[key]
	if count >= int64(m.maxFails) {
		m.locks[key] = time.Now().Add(m.lockoutDuration)
	}
	return count, nil
}

func (m *MemoryAccountLocker) GetLoginFailCount(_ context.Context, key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fails[key], nil
}

func (m *MemoryAccountLocker) ResetLoginFail(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.fails, key)
	return nil
}

func (m *MemoryAccountLocker) IsLocked(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	expiry, ok := m.locks[key]
	m.mu.RUnlock()

	if !ok {
		return false, nil
	}
	if time.Now().After(expiry) {
		// Lazy cleanup: remove expired lock
		m.mu.Lock()
		delete(m.locks, key)
		m.mu.Unlock()
		return false, nil
	}
	return true, nil
}

func (m *MemoryAccountLocker) LockAccount(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locks[key] = time.Now().Add(m.lockoutDuration)
	return nil
}

// compile-time interface checks
var _ AccountLocker = (*RedisAccountLocker)(nil)
var _ AccountLocker = (*MemoryAccountLocker)(nil)
