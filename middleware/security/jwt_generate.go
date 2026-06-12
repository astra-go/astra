package security

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// ─── Token generation helpers ─────────────────────────────────────────────────
//
// These are application-layer utilities for issuing JWT tokens, separate from
// the JWTWithConfig middleware that validates incoming tokens.

// GenerateJWT generates a signed HS256 JWT token with the given claims.
//
//	tokenStr, err := middleware.GenerateJWT(
//	    jwt.MapClaims{
//	        "sub":     "user:42",
//	        "user_id": 42,
//	        "exp":     time.Now().Add(time.Hour).Unix(),
//	    },
//	    "my-secret",
//	)
func GenerateJWT(claims jwt.Claims, secret string) (string, error) {
	if len(secret) < MinJWTKeyLength {
		return "", fmt.Errorf("jwt: HMAC key must be at least %d bytes (got %d)", MinJWTKeyLength, len(secret))
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateJWTRSA generates an RS256 JWT using an RSA private key.
// pemBytes must be a PEM-encoded PKCS#8 or PKCS#1 private key.
func GenerateJWTRSA(claims jwt.Claims, pemBytes []byte) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return "", fmt.Errorf("jwt: parse RSA private key: %w", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(key)
}
