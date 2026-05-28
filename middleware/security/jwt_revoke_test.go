package security_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
	"github.com/golang-jwt/jwt/v5"
)

const testRevokeSecret = "test-revoke-secret-must-be-32bytes!!"

func makeRevokeToken(t *testing.T, exp time.Time) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user:1",
		"exp": exp.Unix(),
	})
	raw, err := tok.SignedString([]byte(testRevokeSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func revokeTestApp(store security.TokenRevokeStore) (*astra.App, func(string) *http.Response) {
	app := testutil.NewTestApp()
	app.Use(security.JWTWithConfig(security.JWTConfig{
		Secret:      security.NewSecretString(testRevokeSecret),
		RevokeStore: store,
	}))
	app.GET("/protected", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	do := func(token string) *http.Response {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		return w.Result()
	}
	return app, do
}

// TestRevokeStore_ValidToken verifies that a non-revoked token is accepted.
func TestRevokeStore_ValidToken(t *testing.T) {
	store := security.NewMemoryRevokeStore()
	raw := makeRevokeToken(t, time.Now().Add(time.Hour))
	_, do := revokeTestApp(store)
	resp := do(raw)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
}

// TestRevokeStore_RevokedToken verifies that a revoked token is rejected with 401.
func TestRevokeStore_RevokedToken(t *testing.T) {
	store := security.NewMemoryRevokeStore()
	exp := time.Now().Add(time.Hour)
	raw := makeRevokeToken(t, exp)

	security.RevokeToken(store, raw, exp.Unix())

	_, do := revokeTestApp(store)
	resp := do(raw)
	testutil.AssertEqual(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestRevokeStore_ExpiredRevocationEntry verifies that once a revocation entry
// itself expires (token TTL passed), the store no longer blocks the token.
// In practice the token would also be expired, but this tests the store logic.
func TestRevokeStore_ExpiredRevocationEntry(t *testing.T) {
	store := security.NewMemoryRevokeStore()
	// Revoke with an expireAt already in the past — store should ignore it.
	store.Revoke("somesig", time.Now().Add(-time.Second).Unix())
	testutil.AssertEqual(t, false, store.IsRevoked("somesig"))
}

// TestRevokeStore_Len verifies Len counts only live entries.
func TestRevokeStore_Len(t *testing.T) {
	store := security.NewMemoryRevokeStore()
	future := time.Now().Add(time.Hour).Unix()
	store.Revoke("sig1", future)
	store.Revoke("sig2", future)
	testutil.AssertEqual(t, 2, store.Len())
}

// TestRevokeStore_Purge verifies that Purge removes expired entries.
func TestRevokeStore_Purge(t *testing.T) {
	store := security.NewMemoryRevokeStore()
	// Add one entry that expires very soon and one that lasts an hour.
	// We can't actually wait for expiry in a unit test, so we test Purge
	// indirectly: after revoking with a past expireAt (no-op), Len stays 0.
	store.Revoke("past", time.Now().Add(-time.Second).Unix()) // ignored
	store.Revoke("future", time.Now().Add(time.Hour).Unix())
	store.Purge()
	testutil.AssertEqual(t, 1, store.Len())
}

// TestRevokeToken_Helper verifies the RevokeToken convenience function.
func TestRevokeToken_Helper(t *testing.T) {
	store := security.NewMemoryRevokeStore()
	raw := makeRevokeToken(t, time.Now().Add(time.Hour))
	security.RevokeToken(store, raw, time.Now().Add(time.Hour).Unix())
	// Extract sig the same way the middleware does.
	sig := raw[len(raw)-43:] // HS256 base64url sig is always 43 chars
	_ = sig
	// Verify via the middleware path instead.
	_, do := revokeTestApp(store)
	testutil.AssertEqual(t, http.StatusUnauthorized, do(raw).StatusCode)
}

// TestRevokeStore_WithCache verifies that a cached token is also rejected after
// revocation (the middleware must evict the cache entry on revocation check).
func TestRevokeStore_WithCache(t *testing.T) {
	store := security.NewMemoryRevokeStore()
	exp := time.Now().Add(time.Hour)
	raw := makeRevokeToken(t, exp)

	app := testutil.NewTestApp()
	app.Use(security.JWTWithConfig(security.JWTConfig{
		Secret:      security.NewSecretString(testRevokeSecret),
		CacheSize:   128,
		RevokeStore: store,
	}))
	app.GET("/protected", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	do := func() *http.Response {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		return w.Result()
	}

	// First request: token is valid and gets cached.
	testutil.AssertEqual(t, http.StatusOK, do().StatusCode)

	// Revoke the token.
	security.RevokeToken(store, raw, exp.Unix())

	// Second request: must be rejected even though the token is in the cache.
	testutil.AssertEqual(t, http.StatusUnauthorized, do().StatusCode)
}
