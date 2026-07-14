package security

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// JWTCacheBackend is the interface for JWT token caching.
// Implementations must be safe for concurrent use.
type JWTCacheBackend interface {
	Get(ctx context.Context, sig string) (*Claims, bool)
	Set(ctx context.Context, sig string, claims *Claims, expireAt int64)
	Delete(ctx context.Context, sig string) error
}

// Claims holds parsed JWT claims. Standard registered claims (sub, exp, iat…)
// are embedded; any custom fields are accessible through the Extra map.
type Claims struct {
	jwt.RegisteredClaims
	Extra map[string]any
}

// TokenRevokeStore is the interface for checking and recording revoked JWT tokens.
// Implementations must be safe for concurrent use.
type TokenRevokeStore interface {
	IsRevoked(sig string) bool
	Revoke(sig string, expireAt int64)
}
