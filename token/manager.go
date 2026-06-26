// Package token provides Access/Refresh token lifecycle management for Astra
// applications.
//
// It complements astra/middleware/security/jwt — while the JWT package
// handles token *validation* (signature, expiry, revocation), this package
// handles token *lifecycle*:
//   - Issue:  generate opaque Access + Refresh token pairs
//   - Store:  hash tokens (SHA-256), persist entries
//   - Find:   lookup by access token, refresh token, or account ID
//   - Revoke: single-token blacklist or account-wide revocation
//   - JTI:    replay-attack prevention for one-time use tokens
//   - SSO:    single-login enforcement (later login invalidates earlier)
//
// # Quick start
//
//	import "github.com/astra-go/astra/token"
//
//	// Memory-backed (development / testing)
//	mgr := token.NewMemoryManager()
//	access, refresh, entry, _ := mgr.Issue(ctx, 42, 15*time.Minute, 24*time.Hour)
//
//	// Redis-backed (production)
//	mgr := token.NewRedisManager(rdb, "myapp:token:")
//	found, _ := mgr.FindByAccess(ctx, access)
//	mgr.Blacklist(ctx, access, 15*time.Minute) // logout
//
//	// JTI replay protection
//	mgr.SetJTI(ctx, jti, 5*time.Minute)
//	used, _ := mgr.IsJTIUsed(ctx, jti)
package token

import (
	"context"
	"time"
)

// Entry holds the hashed representation of an issued token pair.
// TokenHash and RefreshHash are SHA-256 digests of the opaque strings
// returned to the client — the raw tokens are never stored.
type Entry struct {
	// UIN is the user / account identifier that owns these tokens.
	UIN int64

	// TokenHash is SHA-256(accessToken).
	TokenHash string

	// RefreshHash is SHA-256(refreshToken).
	RefreshHash string

	// ExpiresAt is the Unix timestamp when the access token expires.
	ExpiresAt int64

	// RefreshExpiry is the Unix timestamp when the refresh token expires.
	RefreshExpiry int64

	// CreatedAt is the Unix timestamp when the pair was issued.
	CreatedAt int64
}

// Manager defines the token lifecycle API.
//
// Implementations must be safe for concurrent use.
// The zero value is not usable — use NewMemoryManager or NewRedisManager.
//
// # Lifecycle
//
//	Issue → store → FindByAccess / FindByRefresh → Blacklist / DeleteByAccount
//	                          ↓
//	                   (token valid)
type Manager interface {
	// Issue creates a new token pair for a user.
	// If the user already has an active pair, it is replaced (SSO enforcement).
	// Returns the raw access token, refresh token, and the stored Entry.
	Issue(ctx context.Context, uin int64, accessTTL, refreshTTL time.Duration) (accessToken string, refreshToken string, entry *Entry, err error)

	// FindByAccess looks up an entry by its raw access token.
	// Returns nil when the token is unknown or blacklisted.
	FindByAccess(ctx context.Context, token string) (*Entry, error)

	// FindByRefresh looks up an entry by its raw refresh token.
	FindByRefresh(ctx context.Context, refreshToken string) (*Entry, error)

	// DeleteByAccess removes the entry indexed by the access token.
	DeleteByAccess(ctx context.Context, token string) error

	// DeleteByRefresh removes the entry indexed by the refresh token.
	DeleteByRefresh(ctx context.Context, refreshToken string) error

	// DeleteByAccount removes all tokens for a user (logout everywhere / SSO).
	DeleteByAccount(ctx context.Context, uin int64) error

	// Blacklist marks a token as revoked. The token is rejected even if its
	// expiry has not yet passed. ttl should match the token's remaining TTL.
	Blacklist(ctx context.Context, token string, ttl time.Duration) error

	// IsBlacklisted reports whether a token has been revoked.
	IsBlacklisted(ctx context.Context, token string) (bool, error)

	// SetJTI records a JWT ID (JTI) as used for the given TTL, preventing
	// replay attacks where the same JTI is presented twice.
	SetJTI(ctx context.Context, jti string, ttl time.Duration) error

	// IsJTIUsed reports whether a JTI has already been consumed.
	IsJTIUsed(ctx context.Context, jti string) (bool, error)
}

// ─── compile-time interface checks ──────────────────────────────────────────

var (
	_ Manager = (*MemoryManager)(nil)
	_ Manager = (*RedisManager)(nil)
)
