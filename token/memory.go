package token

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// MemoryManager is an in-process token Manager backed by plain Go maps.
// Suitable for development, testing, and single-instance deployments.
type MemoryManager struct {
	mu        sync.RWMutex
	byAccess  map[string]*Entry  // tokenHash → Entry
	byRefresh map[string]*Entry  // refreshHash → Entry
	byAccount map[int64]*Entry   // UIN → Entry
	blacklist map[string]time.Time // tokenHash → expiry
	jtiSet    map[string]time.Time // jti → expiry
}

// NewMemoryManager creates an empty in-memory token manager.
func NewMemoryManager() *MemoryManager {
	return &MemoryManager{
		byAccess:  make(map[string]*Entry),
		byRefresh: make(map[string]*Entry),
		byAccount: make(map[int64]*Entry),
		blacklist: make(map[string]time.Time),
		jtiSet:    make(map[string]time.Time),
	}
}

// ─── Manager interface ──────────────────────────────────────────────────────

func (m *MemoryManager) Issue(_ context.Context, uin int64, accessTTL, refreshTTL time.Duration) (string, string, *Entry, error) {
	accessToken := generateToken()
	refreshToken := generateToken()
	now := time.Now()

	entry := &Entry{
		UIN:           uin,
		TokenHash:     hashToken(accessToken),
		RefreshHash:   hashToken(refreshToken),
		ExpiresAt:     now.Add(accessTTL).Unix(),
		RefreshExpiry: now.Add(refreshTTL).Unix(),
		CreatedAt:     now.Unix(),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// SSO enforcement: evict previous pair for this user.
	if existing, ok := m.byAccount[uin]; ok {
		delete(m.byAccess, existing.TokenHash)
		delete(m.byRefresh, existing.RefreshHash)
		delete(m.byAccount, uin)
	}

	m.byAccess[entry.TokenHash] = entry
	m.byRefresh[entry.RefreshHash] = entry
	m.byAccount[uin] = entry

	return accessToken, refreshToken, entry, nil
}

func (m *MemoryManager) FindByAccess(_ context.Context, token string) (*Entry, error) {
	hash := hashToken(token)

	// Check blacklist first.
	if m.isBlacklisted(hash) {
		return nil, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byAccess[hash], nil
}

func (m *MemoryManager) FindByRefresh(_ context.Context, refreshToken string) (*Entry, error) {
	hash := hashToken(refreshToken)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byRefresh[hash], nil
}

func (m *MemoryManager) DeleteByAccess(_ context.Context, token string) error {
	hash := hashToken(token)

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.byAccess[hash]
	if !ok {
		return nil
	}
	delete(m.byAccess, hash)
	delete(m.byRefresh, entry.RefreshHash)
	delete(m.byAccount, entry.UIN)
	return nil
}

func (m *MemoryManager) DeleteByRefresh(_ context.Context, refreshToken string) error {
	hash := hashToken(refreshToken)

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.byRefresh[hash]
	if !ok {
		return nil
	}
	delete(m.byRefresh, hash)
	delete(m.byAccess, entry.TokenHash)
	delete(m.byAccount, entry.UIN)
	return nil
}

func (m *MemoryManager) DeleteByAccount(_ context.Context, uin int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.byAccount[uin]
	if !ok {
		return nil
	}
	delete(m.byAccount, uin)
	delete(m.byAccess, entry.TokenHash)
	delete(m.byRefresh, entry.RefreshHash)
	return nil
}

func (m *MemoryManager) Blacklist(_ context.Context, token string, ttl time.Duration) error {
	hash := hashToken(token)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.blacklist[hash] = time.Now().Add(ttl)
	return nil
}

func (m *MemoryManager) IsBlacklisted(_ context.Context, token string) (bool, error) {
	hash := hashToken(token)
	return m.isBlacklisted(hash), nil
}

func (m *MemoryManager) SetJTI(_ context.Context, jti string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupJTILocked()
	m.jtiSet[jti] = time.Now().Add(ttl)
	return nil
}

func (m *MemoryManager) IsJTIUsed(_ context.Context, jti string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	expiry, ok := m.jtiSet[jti]
	if !ok {
		return false, nil
	}
	if time.Now().After(expiry) {
		return false, nil
	}
	return true, nil
}

// ─── internal helpers ───────────────────────────────────────────────────────

func (m *MemoryManager) isBlacklisted(hash string) bool {
	m.mu.RLock()
	expiry, ok := m.blacklist[hash]
	m.mu.RUnlock()

	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		m.mu.Lock()
		delete(m.blacklist, hash)
		m.mu.Unlock()
		return false
	}
	return true
}

func (m *MemoryManager) cleanupJTILocked() {
	now := time.Now()
	for jti, expiry := range m.jtiSet {
		if now.After(expiry) {
			delete(m.jtiSet, jti)
		}
	}
}

// ─── token helpers ──────────────────────────────────────────────────────────

// generateToken returns a cryptographically-random hex string (64 hex chars).
func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// hashToken returns the SHA-256 hex digest of a token value.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
