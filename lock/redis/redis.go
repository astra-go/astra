// Package redis provides a Redis-backed distributed lock implementation.
//
// The lock uses SET key value NX PX ttl to acquire and DEL to release.
// A background goroutine extends the expiry every ttl/3 while the lock is held,
// so short TTLs can be used safely even for longer-running critical sections.
//
// # Usage
//
//	import lockredis "github.com/astra-go/astra/lock/redis"
//
//	locker := lockredis.New(redisClient)
//
//	// Blocking lock
//	release, err := locker.Lock(ctx, "payment:order-42", 30*time.Second)
//	defer release()
//
//	// Non-blocking
//	release, err = locker.TryLock(ctx, "payment:order-42", 30*time.Second)
//	if errors.Is(err, lock.ErrNotAcquired) { ... }
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/astra-go/astra/lock"
)

const (
	defaultRetryInterval = 50 * time.Millisecond
	renewFraction        = 3 // renew at 1/3 of TTL
)

// Locker is a Redis-backed distributed lock.
type Locker struct {
	client redis.UniversalClient
}

// New creates a Redis-backed Locker.
func New(client redis.UniversalClient) *Locker {
	return &Locker{client: client}
}

// Lock acquires the named lock, blocking until it is available or ctx is done.
func (l *Locker) Lock(ctx context.Context, key string, ttl time.Duration) (lock.ReleaseFunc, error) {
	for {
		release, err := l.TryLock(ctx, key, ttl)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, lock.ErrNotAcquired) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("lock/redis: Lock %q: %w", key, ctx.Err())
		case <-time.After(defaultRetryInterval):
		}
	}
}

// TryLock attempts to acquire the lock immediately.
// Returns lock.ErrNotAcquired if the lock is held by another holder.
func (l *Locker) TryLock(ctx context.Context, key string, ttl time.Duration) (lock.ReleaseFunc, error) {
	token := uuid.NewString()

	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("lock/redis: TryLock %q: %w", key, err)
	}
	if !ok {
		return nil, lock.ErrNotAcquired
	}

	ctx, cancel := context.WithCancel(context.Background())
	go l.keepAlive(ctx, key, token, ttl)

	var once bool
	release := func() {
		if once {
			return
		}
		once = true
		cancel()
		_ = l.release(context.Background(), key, token)
	}
	return release, nil
}

// keepAlive renews the lock TTL every ttl/renewFraction until ctx is done.
func (l *Locker) keepAlive(ctx context.Context, key, token string, ttl time.Duration) {
	interval := ttl / renewFraction
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = l.renew(ctx, key, token, ttl)
		}
	}
}

// renew extends the TTL only if we still own the lock (token matches).
var renewScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
    return 0
end
`)

func (l *Locker) renew(ctx context.Context, key, token string, ttl time.Duration) error {
	ms := fmt.Sprintf("%d", ttl.Milliseconds())
	return renewScript.Run(ctx, l.client, []string{key}, token, ms).Err()
}

// release deletes the key only if the token still matches (atomic CAS-delete).
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`)

func (l *Locker) release(ctx context.Context, key, token string) error {
	return releaseScript.Run(ctx, l.client, []string{key}, token).Err()
}

// Verify Locker implements lock.Locker at compile time.
var _ lock.Locker = (*Locker)(nil)
