// Package lock provides a unified distributed lock abstraction.
//
// The Locker interface is implemented by two backends:
//
//   - lock/redis  — Redis SET NX EX with automatic expiry and auto-renewal
//   - lock/etcd   — etcd lease-based lock with session TTL
//
// # Usage
//
//	import (
//	    "github.com/astra-go/astra/lock"
//	    lockredis "github.com/astra-go/astra/lock/redis"
//	)
//
//	locker := lockredis.New(redisClient)
//
//	// Blocking lock — waits until acquired or ctx cancelled
//	release, err := locker.Lock(ctx, "order:pay:123", 30*time.Second)
//	if err != nil { ... }
//	defer release()
//
//	// Non-blocking — returns immediately if lock is held by another holder
//	release, err = locker.TryLock(ctx, "order:pay:123", 30*time.Second)
//	if errors.Is(err, lock.ErrNotAcquired) {
//	    // another instance holds the lock
//	}
package lock

import (
	"context"
	"errors"
	"time"
)

// ErrNotAcquired is returned by TryLock when the lock is already held.
var ErrNotAcquired = errors.New("lock: not acquired")

// ReleaseFunc releases the lock. It is safe to call multiple times.
type ReleaseFunc func()

// Locker is the unified distributed lock interface.
type Locker interface {
	// Lock acquires the named lock for at most ttl, blocking until it is
	// available or ctx is cancelled.
	// Returns a ReleaseFunc that must be called to release the lock.
	Lock(ctx context.Context, key string, ttl time.Duration) (ReleaseFunc, error)

	// TryLock attempts to acquire the named lock immediately.
	// Returns ErrNotAcquired (without wrapping) when the lock is held by
	// another holder; the caller can check with errors.Is(err, lock.ErrNotAcquired).
	TryLock(ctx context.Context, key string, ttl time.Duration) (ReleaseFunc, error)
}
