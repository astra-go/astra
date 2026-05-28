package security

import (
	"sync"
	"time"
)

// TokenRevokeStore is the interface for checking and recording revoked JWT tokens.
// Implementations must be safe for concurrent use.
//
// The store is keyed by the token's signature segment (last dot-separated field)
// to keep keys short (~43 chars for HS256) and consistent with the JWT cache.
//
// Built-in implementation: NewMemoryRevokeStore.
// For multi-instance deployments, implement this interface backed by Redis or
// another shared store and pass it via JWTConfig.RevokeStore.
type TokenRevokeStore interface {
	// IsRevoked returns true when the token identified by sig has been revoked
	// and its revocation entry has not yet expired.
	IsRevoked(sig string) bool

	// Revoke marks the token identified by sig as revoked until expireAt (Unix
	// seconds). Entries whose expireAt is in the past are ignored.
	Revoke(sig string, expireAt int64)
}

// ─── In-memory implementation ─────────────────────────────────────────────────

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
}

// NewMemoryRevokeStore creates an empty in-memory revocation store.
func NewMemoryRevokeStore() *MemoryRevokeStore {
	return &MemoryRevokeStore{
		entries: make(map[string]revokeEntry),
	}
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
