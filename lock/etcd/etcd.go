// Package etcd provides an etcd-backed distributed lock implementation.
//
// Each lock acquisition creates a short-lived etcd Session (lease). The lease
// is auto-renewed while the lock is held and revoked on release, so the lock
// is freed even if the process crashes.
//
// The underlying implementation delegates to etcd's concurrency.Mutex, which
// uses an etcd lease + watch to provide fair, blocking mutual exclusion.
//
// # Note on TTL
//
// etcd leases use a TTL in seconds (minimum 1 second). The ttl argument is
// rounded up to the next whole second.
//
// # Usage
//
//	import (
//	    clientv3 "go.etcd.io/etcd/client/v3"
//	    lockdetcd "github.com/astra-go/astra/lock/etcd"
//	)
//
//	cli, _ := clientv3.New(clientv3.Config{Endpoints: []string{"localhost:2379"}})
//	locker := lockdetcd.New(cli)
//
//	release, err := locker.Lock(ctx, "/locks/order-42", 30*time.Second)
//	defer release()
package etcd

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"

	"github.com/astra-go/astra/lock"
)

// Locker is an etcd-backed distributed lock.
type Locker struct {
	client *clientv3.Client
}

// New creates an etcd-backed Locker.
func New(client *clientv3.Client) *Locker {
	return &Locker{client: client}
}

// Lock acquires the named lock, blocking until it is available or ctx is done.
func (l *Locker) Lock(ctx context.Context, key string, ttl time.Duration) (lock.ReleaseFunc, error) {
	sess, mu, err := l.newMutex(ctx, key, ttl)
	if err != nil {
		return nil, err
	}
	if err := mu.Lock(ctx); err != nil {
		_ = sess.Close()
		return nil, fmt.Errorf("lock/etcd: Lock %q: %w", key, err)
	}
	return l.makeRelease(ctx, sess, mu, key), nil
}

// TryLock attempts to acquire the lock immediately.
// Returns lock.ErrNotAcquired if the lock is held by another holder.
func (l *Locker) TryLock(ctx context.Context, key string, ttl time.Duration) (lock.ReleaseFunc, error) {
	sess, mu, err := l.newMutex(ctx, key, ttl)
	if err != nil {
		return nil, err
	}
	if err := mu.TryLock(ctx); err != nil {
		_ = sess.Close()
		if isAlreadyLocked(err) {
			return nil, lock.ErrNotAcquired
		}
		return nil, fmt.Errorf("lock/etcd: TryLock %q: %w", key, err)
	}
	return l.makeRelease(ctx, sess, mu, key), nil
}

func (l *Locker) newMutex(ctx context.Context, key string, ttl time.Duration) (*concurrency.Session, *concurrency.Mutex, error) {
	ttlSec := int(ttl.Seconds())
	if ttlSec < 1 {
		ttlSec = 1
	}
	sess, err := concurrency.NewSession(l.client,
		concurrency.WithTTL(ttlSec),
		concurrency.WithContext(ctx),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("lock/etcd: create session for %q: %w", key, err)
	}
	return sess, concurrency.NewMutex(sess, key), nil
}

func (l *Locker) makeRelease(_ context.Context, sess *concurrency.Session, mu *concurrency.Mutex, key string) lock.ReleaseFunc {
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mu.Unlock(ctx)
		_ = sess.Close()
		_ = key // suppress unused warning
	}
}

// isAlreadyLocked detects the etcd "already locked" error returned by TryLock.
func isAlreadyLocked(err error) bool {
	// etcd concurrency.ErrLocked is the canonical sentinel.
	return errors.Is(err, concurrency.ErrLocked)
}

// Verify Locker implements lock.Locker at compile time.
var _ lock.Locker = (*Locker)(nil)
