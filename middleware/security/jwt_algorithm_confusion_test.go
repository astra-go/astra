package security_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// Must be ≥ MinJWTKeyLength (32 bytes)
	hmacSecret = "test-secret-key-must-be-32-bytes-long!"
)

// generateECKey creates a new ECDSA P-256 key for testing.
func generateECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	return key
}

// makeTokenWithAlg creates a token string with a specific signing method.
func makeTokenWithAlg(t *testing.T, method jwt.SigningMethod, claims jwt.Claims, secret any) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	var signed string
	var err error
	switch m := method.(type) {
	case *jwt.SigningMethodHMAC:
		signed, err = tok.SignedString([]byte(secret.(string)))
	case *jwt.SigningMethodECDSA:
		signed, err = tok.SignedString(secret.(*ecdsa.PrivateKey))
	default:
		t.Fatalf("unsupported signing method: %v", m)
	}
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// forgeNoneToken forges a token with alg:none.
func forgeNoneToken(t *testing.T, claims jwt.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	raw, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("forge none token: %v", err)
	}
	return raw
}

// assertBodyContains asserts the response body contains the given substring.
func assertBodyContains(t *testing.T, body string, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("expected body to contain %q, got: %s", substr, body)
	}
}

// ---------------------- Test Cases ----------------------

// TestJWT_AlgorithmNoneRejected verifies that alg:none tokens are rejected.
func TestJWT_AlgorithmNoneRejected(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(security.JWTWithConfig(security.JWTConfig{
		Secret: security.NewSecretString(hmacSecret),
	}))
	app.GET("/protected", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", "ok")
	})

	// Forge a token with alg: none
	claims := jwt.MapClaims{
		"sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	noneToken := forgeNoneToken(t, claims)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+noneToken)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	testutil.AssertEqual(t, http.StatusUnauthorized, w.Code)
	assertBodyContains(t, w.Body.String(), "unexpected signing method")
}

// TestJWT_KeyConfusion_HMACWithRSA verifies that using an RSA public key
// as HMAC secret (Key Confusion attack) is rejected.
func TestJWT_KeyConfusion_HMACWithRSA(t *testing.T) {
	// Setup: server expects ECDSA-signed tokens
	ecKey := generateECKey(t)
	pubPEM := pemEncodeECPublicKey(t, &ecKey.PublicKey)

	app := testutil.NewTestApp()
	app.Use(security.JWTWithConfig(security.JWTConfig{
		KeyFunc: security.ECPublicKey(pubPEM),
	}))
	app.GET("/protected", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", "ok")
	})

	// Attacker crafts a token signed with HMAC using the public key as secret
	//
	// In a real attack:
	//   1. Attacker gets the server's public key (often in /jwks.json)
	//   2. Attacker signs a token with HS256 using the public key as secret
	//   3. Server's KeyFunc returns the public key for HMAC validation
	//      → signature verifies because both sides use the same bytes
	//
	// Our fix: ECPublicKey wraps keyFunc to check t.Method is *jwt.SigningMethodECDSA.
	maliciousClaims := jwt.MapClaims{
		"sub": "attacker",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	// Sign with HMAC using the public key bytes as "secret"
	pubKeyBytes := []byte(fmt.Sprintf("%v", &ecKey.PublicKey))
	maliciousToken := makeTokenWithAlg(t, jwt.SigningMethodHS256, maliciousClaims, string(pubKeyBytes))

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+maliciousToken)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	// Must be 401 (not 200)
	testutil.AssertEqual(t, http.StatusUnauthorized, w.Code)
	assertBodyContains(t, w.Body.String(), "unexpected signing method")
}

// TestJWT_ValidHMACTokenAccepted verifies that valid HMAC tokens still work.
func TestJWT_ValidHMACTokenAccepted(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(security.JWTWithConfig(security.JWTConfig{
		Secret: security.NewSecretString(hmacSecret),
	}))
	app.GET("/protected", func(c *astra.Ctx) error {
		claims := security.GetClaims(c)
		if claims == nil {
			return c.NoContent(http.StatusInternalServerError)
		}
		return c.String(http.StatusOK, "%s", claims.Subject)
	})

	// Create a valid HMAC token
	claims := jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	validToken := makeTokenWithAlg(t, jwt.SigningMethodHS256, claims, hmacSecret)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	testutil.AssertEqual(t, http.StatusOK, w.Code)
	testutil.AssertEqual(t, "user123", w.Body.String())
}

// TestJWT_ValidECDSATokenAccepted verifies that valid ECDSA tokens work.
func TestJWT_ValidECDSATokenAccepted(t *testing.T) {
	ecKey := generateECKey(t)
	pubPEM := pemEncodeECPublicKey(t, &ecKey.PublicKey)

	app := testutil.NewTestApp()
	app.Use(security.JWTWithConfig(security.JWTConfig{
		KeyFunc: security.ECPublicKey(pubPEM),
	}))
	app.GET("/protected", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", "ok")
	})

	claims := jwt.MapClaims{
		"sub": "user456",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	validToken := makeTokenWithAlg(t, jwt.SigningMethodES256, claims, ecKey)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	testutil.AssertEqual(t, http.StatusOK, w.Code)
	testutil.AssertEqual(t, "ok", w.Body.String())
}

// ---------------------- Helpers ----------------------

// pemEncodeECPublicKey encodes an ECDSA public key to PEM format.
func pemEncodeECPublicKey(t *testing.T, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal EC public key: %v", err)
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}
	buf := new(bytes.Buffer)
	if err := pem.Encode(buf, block); err != nil {
		t.Fatalf("encode PEM: %v", err)
	}
	return buf.Bytes()
}
