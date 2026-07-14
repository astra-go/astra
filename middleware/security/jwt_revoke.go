package security

import (
	"sync"
	"time"
)

type revokeEntry struct {
	expireAt int64 // Unix seconds; entry is valid while time.Now().Unix() < expireAt
}

// MemoryRevokeStore is a thread-safe in-process token revocation store.
//
// Revoked entries are kept until their expireAt passes, after which they are
// lazily evicted on the next Revoke call or by an explicit Purge.
//
// Suitable for single-instance deployments. For multi-instance deployments,
// implement TokenRevokeStore backed by a shared store (e.g. Redis SET with TTL).
type MemoryRevokeStore struct {
	mu      sync.RWMutex
	entries map[string]revokeEntry
	stop    chan struct{}
	once    sync.Once
}

// NewMemoryRevokeStore creates an empty in-memory revocation store
// with a background purge goroutine that runs every 5 minutes.
// Call Close to stop the background goroutine and release resources.
func NewMemoryRevokeStore() *MemoryRevokeStore {
	s := &MemoryRevokeStore{
		entries: make(map[string]revokeEntry),
		stop:    make(chan struct{}),
	}
	go s.purgeLoop()
	return s
}

// IsRevoked implements TokenRevokeStore.
func (s *MemoryRevokeStore) IsRevoked(sig string) bool {
	now := time.Now().Unix()
	s.mu.RLock()
	e, ok := s.entries[sig]
	s.mu.RUnlock()
	return ok && e.expireAt > now
}

// Revoke implements TokenRevokeStore. Entries whose expireAt is already past
// are silently ignored. Expired entries are lazily purged on each Revoke call.
func (s *MemoryRevokeStore) Revoke(sig string, expireAt int64) {
	now := time.Now().Unix()
	if expireAt <= now {
		return // token already expired — no need to track
	}
	s.mu.Lock()
	s.entries[sig] = revokeEntry{expireAt: expireAt}
	s.purgeExpiredLocked(now)
	s.mu.Unlock()
}

// Purge removes all entries whose expireAt has passed. Call periodically to
// reclaim memory in long-running servers with high token churn.
func (s *MemoryRevokeStore) Purge() {
	now := time.Now().Unix()
	s.mu.Lock()
	s.purgeExpiredLocked(now)
	s.mu.Unlock()
}

// Len returns the number of currently tracked (not yet expired) revoked tokens.
func (s *MemoryRevokeStore) Len() int {
	now := time.Now().Unix()
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, e := range s.entries {
		if e.expireAt > now {
			count++
		}
	}
	return count
}

// purgeExpiredLocked removes expired entries. Caller must hold s.mu.Lock().
func (s *MemoryRevokeStore) purgeExpiredLocked(now int64) {
	for k, e := range s.entries {
		if e.expireAt <= now {
			delete(s.entries, k)
		}
	}
}

// Close stops the background purge goroutine. Safe to call multiple times.
func (s *MemoryRevokeStore) Close() {
	s.once.Do(func() { close(s.stop) })
}

// purgeLoop runs a periodic purge of expired entries every 5 minutes.
func (s *MemoryRevokeStore) purgeLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Purge()
		case <-s.stop:
			return
		}
	}
}

// ─── Helper ───────────────────────────────────────────────────────────────────

// RevokeToken is a convenience wrapper that extracts the signature segment from
// a raw JWT string and calls store.Revoke with the token's expiry time.
// expireAt is the Unix timestamp at which the token expires (from the exp claim).
//
//	store.Revoke is a no-op when expireAt is already in the past, so it is safe
//	to call RevokeToken even for tokens that are close to expiry.
func RevokeToken(store TokenRevokeStore, rawToken string, expireAt int64) {
	store.Revoke(tokenSignature(rawToken), expireAt)
}
