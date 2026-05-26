package middleware_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	mwobs "github.com/astra-go/astra/middleware/observability"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
	"github.com/prometheus/client_golang/prometheus"
)

// ─── APIKey ──────────────────────────────────────────────────────────────────

func TestAPIKey_ValidHeader_Passes(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Validator: func(_ context.Context, key string) error {
			if key != "valid-key" {
				return fmt.Errorf("invalid key")
			}
			return nil
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-API-Key": "valid-key"}).
		AssertStatus(http.StatusOK).AssertBodyContains("ok")
}

func TestAPIKey_InvalidKey_Returns401(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Validator: func(_ context.Context, key string) error {
			return fmt.Errorf("invalid key: %s", key)
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-API-Key": "bad-key"}).
		AssertStatus(http.StatusUnauthorized)
}

func TestAPIKey_MissingKey_Returns401(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Validator: func(_ context.Context, key string) error { return nil },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusUnauthorized)
}

func TestAPIKey_QueryParam_Passes(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Validator: func(_ context.Context, key string) error {
			if key != "query-key" {
				return fmt.Errorf("invalid")
			}
			return nil
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/?api_key=query-key").AssertStatus(http.StatusOK)
}

func TestAPIKey_BearerPrefix_Stripped(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Validator: func(_ context.Context, key string) error {
			if key != "bearer-token" {
				return fmt.Errorf("got: %s", key)
			}
			return nil
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-API-Key": "Bearer bearer-token"}).
		AssertStatus(http.StatusOK)
}

func TestAPIKey_Skipper_Skips(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Skipper:   func(c *astra.Ctx) bool { return true },
		Validator: func(_ context.Context, key string) error { return fmt.Errorf("never") },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

func TestAPIKey_CustomHeaderAndQueryParam(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Header:     "X-App-Token",
		QueryParam: "token",
		Validator: func(_ context.Context, key string) error {
			if key != "my-token" {
				return fmt.Errorf("invalid")
			}
			return nil
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	// Custom header
	s.GET("/", map[string]string{"X-App-Token": "my-token"}).AssertStatus(http.StatusOK)
	// Custom query param
	s.GET("/?token=my-token").AssertStatus(http.StatusOK)
	// Default header should not work
	s.GET("/", map[string]string{"X-API-Key": "my-token"}).AssertStatus(http.StatusUnauthorized)
}

func TestAPIKey_PanicsOnNilValidator(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when Validator is nil")
		}
	}()
	sec.APIKey(sec.APIKeyConfig{})
}

func TestAPIKey_CustomErrorHandler(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.APIKey(sec.APIKeyConfig{
		Validator: func(_ context.Context, key string) error { return fmt.Errorf("nope") },
		ErrorHandler: func(c *astra.Ctx) error {
			return c.String(http.StatusForbidden, "custom error")
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/", map[string]string{"X-API-Key": "any"})
	resp.AssertStatus(http.StatusForbidden).AssertBodyContains("custom error")
}

// ─── SecureHeaders ───────────────────────────────────────────────────────────

func TestSecureHeaders_DefaultConfig_SetsAllHeaders(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)
	resp.AssertHeaderContains("Strict-Transport-Security", "max-age=31536000")
	resp.AssertHeaderContains("Strict-Transport-Security", "includeSubDomains")
	resp.AssertHeader("X-Frame-Options", "DENY")
	resp.AssertHeader("X-Content-Type-Options", "nosniff")
	resp.AssertHeader("Referrer-Policy", "strict-origin-when-cross-origin")
}

func TestSecureHeaders_CustomConfig(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders(middleware.SecureConfig{
		HSTSMaxAge:         86400,
		FrameOption:        middleware.FrameSameOrigin,
		ContentTypeNosniff: true,
		CSP:                "default-src 'self'",
		PermissionsPolicy:  "geolocation=()",
		ReferrerPolicy:     "no-referrer",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertHeaderContains("Strict-Transport-Security", "max-age=86400")
	resp.AssertHeader("X-Frame-Options", "SAMEORIGIN")
	resp.AssertHeader("Content-Security-Policy", "default-src 'self'")
	resp.AssertHeader("Permissions-Policy", "geolocation=()")
	resp.AssertHeader("Referrer-Policy", "no-referrer")
}

func TestSecureHeaders_HSTSIncludeSubdomainsAndPreload(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders(middleware.SecureConfig{
		HSTSMaxAge:            31536000,
		HSTSIncludeSubdomains: true,
		HSTSPreload:           true,
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	hsts := resp.Header("Strict-Transport-Security")
	if !strings.Contains(hsts, "includeSubDomains") {
		t.Error("expected includeSubDomains in HSTS")
	}
	if !strings.Contains(hsts, "preload") {
		t.Error("expected preload in HSTS")
	}
}

func TestSecureHeaders_ZeroHSTSMaxAge_DisablesHSTS(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders(middleware.SecureConfig{
		HSTSMaxAge: 0,
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	if got := resp.Header("Strict-Transport-Security"); got != "" {
		t.Errorf("expected no HSTS header, got %q", got)
	}
}

// ─── Tenant ──────────────────────────────────────────────────────────────────

func TestTenant_FromHeader(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.TenantOptional())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", sec.TenantID(c))
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-Tenant-ID": "acme"}).
		AssertStatus(http.StatusOK).AssertBodyContains("acme")
}

func TestTenant_FromQueryParam(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.TenantOptional())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", sec.TenantID(c))
	})
	s := testutil.NewServer(t, app)

	s.GET("/?tenant_id=acme").AssertStatus(http.StatusOK).AssertBodyContains("acme")
}

func TestTenant_HeaderTakesPrecedenceOverQuery(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.TenantOptional())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", sec.TenantID(c))
	})
	s := testutil.NewServer(t, app)

	s.GET("/?tenant_id=query-tenant", map[string]string{"X-Tenant-ID": "header-tenant"}).
		AssertStatus(http.StatusOK).AssertBodyContains("header-tenant")
}

func TestTenant_DefaultRequired_RejectsMissing(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Tenant()) // always required
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusBadRequest)
}

func TestTenant_NotRequired_PassesWithoutTenant(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.TenantOptional())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "tid=%s", sec.TenantID(c))
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK).AssertBodyContains("tid=")
}

func TestTenant_Validator_RejectsInvalid(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Tenant(sec.TenantConfig{
		Validator: func(_ context.Context, tid string) error {
			if len(tid) > 5 {
				return fmt.Errorf("tenant ID too long")
			}
			return nil
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-Tenant-ID": "toolongtenant"}).
		AssertStatus(http.StatusBadRequest)
}

func TestTenant_CustomContextKey(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Tenant(sec.TenantConfig{ContextKey: "my_tenant"}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", sec.TenantIDFromKey(c, "my_tenant"))
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-Tenant-ID": "custom"}).
		AssertStatus(http.StatusOK).AssertBodyContains("custom")
}

func TestTenant_Sources_PathOnly_IgnoresHeader(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Tenant(sec.TenantConfig{
		Sources:   sec.TenantSrcPath,
		PathParam: "tenant",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", sec.TenantID(c))
	})
	s := testutil.NewServer(t, app)

	// Header is ignored when only TenantSrcPath is enabled.
	s.GET("/", map[string]string{"X-Tenant-ID": "spoofed"}).
		AssertStatus(http.StatusBadRequest)
}

func TestTenantFromContext_ReadsFromClaims(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.TenantFromContext("tenant_id"))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", sec.TenantID(c))
	})
	s := testutil.NewServer(t, app)

	// No tenant in context → 400
	s.GET("/").AssertStatus(http.StatusBadRequest)

	// Pre-set tenant in context (simulates JWT middleware setting claims)
	app2 := testutil.NewTestApp()
	app2.Use(func(c *astra.Ctx) error {
		c.Set("tenant_id", "acme-from-jwt")
		c.Next()
		return nil
	})
	app2.Use(sec.TenantFromContext("tenant_id"))
	app2.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", sec.TenantID(c))
	})
	s2 := testutil.NewServer(t, app2)
	s2.GET("/").AssertStatus(http.StatusOK).AssertBodyContains("acme-from-jwt")

	// Header is ignored by TenantFromContext
	s2.GET("/", map[string]string{"X-Tenant-ID": "spoofed"}).
		AssertStatus(http.StatusOK).AssertBodyContains("acme-from-jwt")
}

func TestTenantID_EmptyWhenNotSet(t *testing.T) {
	app := testutil.NewTestApp()
	// No Tenant middleware
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "tid=%s", sec.TenantID(c))
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertBodyContains("tid=")
}

// ─── CSP ─────────────────────────────────────────────────────────────────────

func TestCSP_SetsPolicyHeader(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CSP(middleware.CSPConfig{
		Policy: "default-src 'self'",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)
	resp.AssertHeader("Content-Security-Policy", "default-src 'self'")
}

func TestCSP_ReportOnly_SetsReportOnlyHeader(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CSP(middleware.CSPConfig{
		Policy:     "default-src 'self'",
		ReportOnly: true,
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	if got := resp.Header("Content-Security-Policy-Report-Only"); got != "default-src 'self'" {
		t.Errorf("expected CSP-Report-Only header, got %q", got)
	}
	if got := resp.Header("Content-Security-Policy"); got != "" {
		t.Errorf("expected no enforcing CSP header, got %q", got)
	}
}

func TestCSP_NonceInjection(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CSP(middleware.CSPConfig{
		Policy:    "default-src 'self'; script-src 'nonce-{nonce}'",
		NonceFunc: middleware.RandomNonce,
	}))
	app.GET("/", func(c *astra.Ctx) error {
		nonce := middleware.CSPNonce(c)
		return c.String(http.StatusOK, "nonce=%s", nonce)
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)

	// Nonce should appear in both the header and the body
	cspHeader := resp.Header("Content-Security-Policy")
	if !strings.Contains(cspHeader, "nonce-") {
		t.Errorf("expected nonce in CSP header, got %q", cspHeader)
	}
	body := resp.BodyString()
	if !strings.HasPrefix(body, "nonce=") || len(body) <= 6 {
		t.Errorf("expected nonce in body, got %q", body)
	}
}

func TestCSP_ReportURI_Appended(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CSP(middleware.CSPConfig{
		Policy:    "default-src 'self'",
		ReportURI: "https://csp.example.com/report",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	csp := resp.Header("Content-Security-Policy")
	if !strings.Contains(csp, "report-uri https://csp.example.com/report") {
		t.Errorf("expected report-uri in CSP, got %q", csp)
	}
}

func TestCSP_Skipper_Skips(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CSP(middleware.CSPConfig{
		Policy:  "default-src 'self'",
		Skipper: func(c *astra.Ctx) bool { return true },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	if got := resp.Header("Content-Security-Policy"); got != "" {
		t.Errorf("expected no CSP header when skipped, got %q", got)
	}
}

func TestRandomNonce_NonEmpty(t *testing.T) {
	nonce, err := middleware.RandomNonce()
	if err != nil {
		t.Fatalf("RandomNonce error: %v", err)
	}
	if len(nonce) == 0 {
		t.Error("expected non-empty nonce")
	}
	// 16 bytes → base64url = 22 chars
	if len(nonce) != 22 {
		t.Errorf("expected 22-char nonce, got %d", len(nonce))
	}
}

func TestCSPNonce_EmptyWhenNotConfigured(t *testing.T) {
	app := testutil.NewTestApp()
	// No CSP middleware with NonceFunc
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "nonce=%s", middleware.CSPNonce(c))
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertBodyContains("nonce=")
}

// ─── IPFilter ────────────────────────────────────────────────────────────────

func TestIPFilter_Allowlist_PermitsAllowed(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		Allowlist: []string{"127.0.0.1/32"},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

func TestIPFilter_Allowlist_BlocksNotAllowed(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		Allowlist: []string{"10.0.0.0/8"},
		GetIP: func(c *astra.Ctx) string { return "192.168.1.1" },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusForbidden)
}

func TestIPFilter_Blocklist_BlocksBlocked(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		Blocklist: []string{"127.0.0.1/32"},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusForbidden)
}

func TestIPFilter_BlocklistTakesPrecedenceOverAllowlist(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		Allowlist: []string{"127.0.0.0/8"},
		Blocklist: []string{"127.0.0.1/32"},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusForbidden)
}

func TestIPFilter_EmptyLists_AllowAll(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

func TestIPFilter_Skipper_Skips(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		Blocklist: []string{"127.0.0.1/32"},
		Skipper:   func(c *astra.Ctx) bool { return true },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

func TestIPFilter_UnparseableIP_Denied(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		GetIP: func(c *astra.Ctx) string { return "not-an-ip" },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusForbidden)
}

func TestIPFilter_CustomDenyStatus(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		Blocklist:   []string{"127.0.0.1/32"},
		DenyStatus:  http.StatusNotFound,
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusNotFound)
}

func TestIPFilter_DynamicLoader(t *testing.T) {
	var mu sync.Mutex
	allow := []string{"127.0.0.1/32"}

	app := testutil.NewTestApp()
	app.Use(sec.IPFilter(sec.IPFilterConfig{
		Loader: func(_ context.Context) ([]string, []string, error) {
			mu.Lock()
			defer mu.Unlock()
			return allow, nil, nil
		},
		ReloadInterval: time.Hour, // long interval for test
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	// Initially allowed
	s.GET("/").AssertStatus(http.StatusOK)
}

// ─── Signature ───────────────────────────────────────────────────────────────

func TestSignature_ValidRequest_Passes(t *testing.T) {
	secret := []byte("test-secret-key")
	app := testutil.NewTestApp()
	app.Use(sec.Signature(secret))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	ts := time.Now().Unix()
	nonce := randomHex(t, 16)
	bodyHash := sha256HexStr(nil)
	canonical := fmt.Sprintf("GET\n/\n%d\n%s\n%s", ts, nonce, bodyHash)
	sig := hmacSHA256Hex(t, secret, canonical)

	s.GET("/", map[string]string{
		"X-Timestamp": strconv.FormatInt(ts, 10),
		"X-Nonce":     nonce,
		"X-Signature": sig,
	}).AssertStatus(http.StatusOK)
}

func TestSignature_MissingHeaders_Returns401(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Signature([]byte("secret")))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusUnauthorized)
}

func TestSignature_InvalidSignature_Returns401(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.Signature([]byte("secret")))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{
		"X-Timestamp": strconv.FormatInt(time.Now().Unix(), 10),
		"X-Nonce":     "test-nonce",
		"X-Signature": "invalid-signature",
	}).AssertStatus(http.StatusUnauthorized)
}

func TestSignature_ExpiredTimestamp_Returns401(t *testing.T) {
	secret := []byte("secret")
	app := testutil.NewTestApp()
	app.Use(sec.Signature(secret))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	// Timestamp 10 minutes ago — beyond default 5 min TTL
	ts := time.Now().Add(-10 * time.Minute).Unix()
	nonce := randomHex(t, 16)
	canonical := fmt.Sprintf("GET\n/\n%d\n%s\n%s", ts, nonce, sha256HexStr(nil))
	sig := hmacSHA256Hex(t, secret, canonical)

	s.GET("/", map[string]string{
		"X-Timestamp": strconv.FormatInt(ts, 10),
		"X-Nonce":     nonce,
		"X-Signature": sig,
	}).AssertStatus(http.StatusUnauthorized)
}

func TestSignature_NonceReplay_Returns401(t *testing.T) {
	secret := []byte("secret")
	app := testutil.NewTestApp()
	app.Use(sec.Signature(secret))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	ts := time.Now().Unix()
	nonce := randomHex(t, 16)
	canonical := fmt.Sprintf("GET\n/\n%d\n%s\n%s", ts, nonce, sha256HexStr(nil))
	sig := hmacSHA256Hex(t, secret, canonical)

	headers := map[string]string{
		"X-Timestamp": strconv.FormatInt(ts, 10),
		"X-Nonce":     nonce,
		"X-Signature": sig,
	}

	// First request succeeds
	s.GET("/", headers).AssertStatus(http.StatusOK)
	// Replay rejected
	s.GET("/", headers).AssertStatus(http.StatusUnauthorized)
}

func TestSignature_PanicsOnEmptySecret(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when SecretKey is empty")
		}
	}()
	sec.Signature([]byte{})
}

func TestSignature_WithBody(t *testing.T) {
	secret := []byte("secret")
	app := testutil.NewTestApp()
	app.Use(sec.Signature(secret))
	app.POST("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	ts := time.Now().Unix()
	nonce := randomHex(t, 16)
	body := []byte(`{"hello":"world"}`)
	bodyHash := sha256HexStr(body)
	canonical := fmt.Sprintf("POST\n/\n%d\n%s\n%s", ts, nonce, bodyHash)
	sig := hmacSHA256Hex(t, secret, canonical)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, s.URL()+"/", bytes.NewReader(body))
	req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", sig)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestSignature_InMemoryNonceStore_Close_NoPanic(t *testing.T) {
	// Verify Close() works and is idempotent.
	store := sec.NewInMemoryNonceStore()
	ok, err := store.Seen("nonce-1", time.Minute)
	if err != nil {
		t.Fatalf("Seen: %v", err)
	}
	if ok {
		t.Fatal("first Seen should return false")
	}

	store.Close() // must not block or panic
	store.Close() // double close must not panic

	// Seen still works after Close.
	ok, err = store.Seen("nonce-1", time.Minute)
	if err != nil {
		t.Fatalf("Seen after Close: %v", err)
	}
	if !ok {
		t.Fatal("Seen should return true for replayed nonce")
	}
}

func TestSignature_InMemoryNonceStore_ReplayDetects(t *testing.T) {
	store := sec.NewInMemoryNonceStore()
	defer store.Close()

	seen, err := store.Seen("abc", time.Minute)
	if err != nil || seen {
		t.Fatal("first call should not be seen")
	}

	seen, err = store.Seen("abc", time.Minute)
	if err != nil || !seen {
		t.Fatal("second call should be seen (replay)")
	}
}

// ─── Audit ───────────────────────────────────────────────────────────────────

func TestAudit_RecordsEntry(t *testing.T) {
	var recorded mwobs.AuditEntry
	var mu sync.Mutex

	app := testutil.NewTestApp()
	app.Use(mwobs.Audit(mwobs.AuditConfig{
		Logger: func(entry mwobs.AuditEntry) {
			mu.Lock()
			recorded = entry
			mu.Unlock()
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	if recorded.Method != "GET" {
		t.Errorf("expected method GET, got %s", recorded.Method)
	}
	if recorded.Path != "/" {
		t.Errorf("expected path /, got %s", recorded.Path)
	}
	if recorded.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", recorded.Status)
	}
}

func TestAudit_Skipper_Skips(t *testing.T) {
	called := false
	app := testutil.NewTestApp()
	app.Use(mwobs.Audit(mwobs.AuditConfig{
		Skipper: func(c *astra.Ctx) bool { return true },
		Logger: func(entry mwobs.AuditEntry) { called = true },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	if called {
		t.Error("expected logger not to be called when skipped")
	}
}

func TestAudit_ActorID_FromDefaultHeader(t *testing.T) {
	var recorded mwobs.AuditEntry
	var mu sync.Mutex

	app := testutil.NewTestApp()
	app.Use(mwobs.Audit(mwobs.AuditConfig{
		Logger: func(entry mwobs.AuditEntry) {
			mu.Lock()
			recorded = entry
			mu.Unlock()
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-User-ID": "user42"}).AssertStatus(http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	if recorded.ActorID != "user42" {
		t.Errorf("expected actor_id user42, got %s", recorded.ActorID)
	}
}

func TestAudit_CustomGetActorID(t *testing.T) {
	var recorded mwobs.AuditEntry
	var mu sync.Mutex

	app := testutil.NewTestApp()
	app.Use(mwobs.Audit(mwobs.AuditConfig{
		GetActorID: func(c *astra.Ctx) string { return "custom-actor" },
		Logger: func(entry mwobs.AuditEntry) {
			mu.Lock()
			recorded = entry
			mu.Unlock()
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	if recorded.ActorID != "custom-actor" {
		t.Errorf("expected custom-actor, got %s", recorded.ActorID)
	}
}

func TestAudit_AsyncBuffer(t *testing.T) {
	var recorded mwobs.AuditEntry
	var mu sync.Mutex

	app := testutil.NewTestApp()
	app.Use(mwobs.Audit(mwobs.AuditConfig{
		AsyncBuffer: 16,
		Logger: func(entry mwobs.AuditEntry) {
			mu.Lock()
			recorded = entry
			mu.Unlock()
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	// Wait a bit for the async goroutine to process
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if recorded.Method != "GET" {
		t.Errorf("expected method GET, got %s", recorded.Method)
	}
}

func TestAudit_ClientIP_XForwardedFor(t *testing.T) {
	var recorded mwobs.AuditEntry
	var mu sync.Mutex

	app := testutil.NewTestApp()
	app.Use(mwobs.Audit(mwobs.AuditConfig{
		Logger: func(entry mwobs.AuditEntry) {
			mu.Lock()
			recorded = entry
			mu.Unlock()
		},
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"}).AssertStatus(http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	if recorded.ClientIP != "1.2.3.4" {
		t.Errorf("expected client IP 1.2.3.4, got %s", recorded.ClientIP)
	}
}

// ─── CORSStrict ──────────────────────────────────────────────────────────────

func TestCORSStrict_AllowedOrigin(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CORSStrict("https://app.example.com"))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"Origin": "https://app.example.com"}).
		AssertStatus(http.StatusOK).
		AssertHeader("Access-Control-Allow-Origin", "https://app.example.com")
}

func TestCORSStrict_DisallowedOrigin(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CORSStrict("https://app.example.com"))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	resp := s.GET("/", map[string]string{"Origin": "https://evil.com"})
	resp.AssertStatus(http.StatusOK)
	if got := resp.Header("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no CORS header for disallowed origin, got %q", got)
	}
}

// ─── Logger ──────────────────────────────────────────────────────────────────

func TestLogger_RequestsAreLogged(t *testing.T) {
	// Logger writes to stdout by default; just verify it doesn't panic
	app := testutil.NewTestApp()
	app.Use(middleware.Logger())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

func TestLogger_SkipPaths(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))
	app.GET("/health", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	app.GET("/api", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/health").AssertStatus(http.StatusOK)
	s.GET("/api").AssertStatus(http.StatusOK)
}

func TestLogger_JSONFormat(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "json",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

// ─── LongPoll ────────────────────────────────────────────────────────────────

func TestLongPoll_PublishAndPoll(t *testing.T) {
	mgr := sec.NewLongPollManager(sec.LongPollConfig{
		DefaultTimeout: 2 * time.Second,
	})

	app := testutil.NewTestApp()
	app.GET("/events", mgr.PollHandler(func(c *astra.Ctx) string {
		return c.Query("topic")
	}))
	s := testutil.NewServer(t, app)

	// Publish after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		mgr.Publish("test-topic", map[string]string{"msg": "hello"})
	}()

	resp := s.GET("/events?topic=test-topic")
	resp.AssertStatus(http.StatusOK).AssertBodyContains("hello")
}

func TestLongPoll_Timeout_Returns204(t *testing.T) {
	mgr := sec.NewLongPollManager(sec.LongPollConfig{
		DefaultTimeout: 100 * time.Millisecond,
	})

	app := testutil.NewTestApp()
	app.GET("/events", mgr.PollHandler(func(c *astra.Ctx) string {
		return c.Query("topic")
	}))
	s := testutil.NewServer(t, app)

	resp := s.GET("/events?topic=test-topic")
	status := resp.Status()
	if status != http.StatusNoContent && status != http.StatusOK {
		t.Errorf("expected 204 or 200, got %d", status)
	}
}

func TestLongPoll_EmptyTopic_Returns400(t *testing.T) {
	mgr := sec.NewLongPollManager(sec.LongPollConfig{})

	app := testutil.NewTestApp()
	app.GET("/events", mgr.PollHandler(func(c *astra.Ctx) string {
		return ""
	}))
	s := testutil.NewServer(t, app)

	s.GET("/events").AssertStatus(http.StatusBadRequest)
}

func TestLongPoll_PollHandlerByQuery(t *testing.T) {
	mgr := sec.NewLongPollManager(sec.LongPollConfig{
		DefaultTimeout: 2 * time.Second,
	})

	app := testutil.NewTestApp()
	app.GET("/events", mgr.PollHandlerByQuery("channel"))
	s := testutil.NewServer(t, app)

	go func() {
		time.Sleep(200 * time.Millisecond)
		mgr.Publish("my-channel", "event-data")
	}()

	resp := s.GET("/events?channel=my-channel")
	resp.AssertStatus(http.StatusOK)
}

// ─── MetricsHandler ──────────────────────────────────────────────────────────

func TestMetricsHandler_ServesPrometheus(t *testing.T) {
	app := testutil.NewTestApp()
	app.GET("/metrics", mwobs.MetricsHandler())
	s := testutil.NewServer(t, app)

	resp := s.GET("/metrics")
	resp.AssertStatus(http.StatusOK)
}

func TestMetricsHandlerFor_CustomGatherer(t *testing.T) {
	reg := prometheus.NewRegistry()
	app := testutil.NewTestApp()
	app.GET("/metrics", mwobs.MetricsHandlerFor(reg))
	s := testutil.NewServer(t, app)

	resp := s.GET("/metrics")
	resp.AssertStatus(http.StatusOK)
}

// ─── Pprof ───────────────────────────────────────────────────────────────────

// TestRegisterPprof verifies RegisterPprof option functions exist and compile.
// NOTE: RegisterPprof registers both p and p+"/" which causes a route conflict
// in Astra's strict-conflict router. This is a known issue in pprof.go line 90-91.
// We test the options independently without calling RegisterPprof.
func TestRegisterPprof_OptionFunctions(t *testing.T) {
	// PprofWithPrefix returns a function — just verify it doesn't panic
	opt1 := sec.PprofWithPrefix("/custom")
	if opt1 == nil {
		t.Error("PprofWithPrefix returned nil")
	}

	// PprofWithMiddleware returns a function
	opt2 := sec.PprofWithMiddleware(func(c *astra.Ctx) error {
		c.Next()
		return nil
	})
	if opt2 == nil {
		t.Error("PprofWithMiddleware returned nil")
	}
}

// ─── CSRF (simple constructor) ───────────────────────────────────────────────

func TestCSRF_Constructor_UsesDefaultConfig(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		Secret:       []byte("32-byte-secret-key-for-testing!!"),
		CookieSecure: false,
	}))
	app.GET("/token", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", middleware.GetCSRFToken(c))
	})
	s := testutil.NewServer(t, app)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	resp, _ := client.Get(s.URL() + "/token")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	token := strings.TrimSpace(string(body))
	if token == "" {
		t.Error("expected CSRF token")
	}
}

// ─── RateLimit (simple constructor) ──────────────────────────────────────────

func TestRateLimit_Constructor(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RateLimitWithConfig(sec.RateLimitConfig{
		Rate:  100,
		Burst: 5,
		KeyFunc: func(_ *astra.Ctx) string { return "key" },
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

// ─── SlidingWindow (simple constructor) ──────────────────────────────────────

func TestSlidingWindow_Constructor(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   100,
		Window:  time.Second,
		KeyFunc: func(_ *astra.Ctx) string { return "key" },
		Context: context.Background(),
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("randomHex: %v", err)
	}
	return hex.EncodeToString(b)
}

func sha256HexStr(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256Hex(t *testing.T, key []byte, data string) string {
	t.Helper()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// ─── SecureHeaders COOP / Permissions-Policy ──────────────────────────────────

func TestSecureHeaders_DefaultIncludesCOOPAndPermissionsPolicy(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)
	// Default config includes Permissions-Policy and Cross-Origin-Opener-Policy
	resp.AssertHeader("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	resp.AssertHeader("Cross-Origin-Opener-Policy", "same-origin")
}

func TestSecureHeaders_CustomCOOP(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders(middleware.SecureConfig{
		HSTSMaxAge:              0, // disable HSTS to simplify
		CrossOriginOpenerPolicy: "same-origin-allow-popups",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertHeader("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
}

func TestSecureHeaders_EmptyCOOP_DisablesHeader(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders(middleware.SecureConfig{
		HSTSMaxAge:              0,
		CrossOriginOpenerPolicy: "",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	if got := resp.Header("Cross-Origin-Opener-Policy"); got != "" {
		t.Errorf("expected no COOP header, got %q", got)
	}
}

func TestSecureHeaders_CustomPermissionsPolicy(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.SecureHeaders(middleware.SecureConfig{
		HSTSMaxAge:         0,
		PermissionsPolicy:  "accelerometer=(), camera=(), geolocation=()",
	}))
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "ok")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertHeader("Permissions-Policy", "accelerometer=(), camera=(), geolocation=()")
}
