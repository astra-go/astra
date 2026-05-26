package middleware_test

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	sec "github.com/astra-go/astra/middleware/security"
	"github.com/astra-go/astra/testutil"
	"github.com/golang-jwt/jwt/v5"
)

// ─── Recovery ─────────────────────────────────────────────────────────────────

func TestRecovery_PanicReturns500(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Recovery())
	app.GET("/panic", func(_ *astra.Ctx) error {
		panic("something went wrong")
	})
	s := testutil.NewServer(t, app)

	s.GET("/panic").AssertStatus(http.StatusInternalServerError)
}

func TestRecovery_PanicString_ReturnsJSON(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Recovery())
	app.GET("/panic", func(_ *astra.Ctx) error {
		panic("kaboom")
	})
	s := testutil.NewServer(t, app)

	s.GET("/panic").
		AssertStatus(http.StatusInternalServerError).
		AssertBodyContains("Internal Server Error")
}

func TestRecovery_NoPanic_PassesThrough(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Recovery())
	app.GET("/ok", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "healthy")
	})
	s := testutil.NewServer(t, app)

	s.GET("/ok").AssertStatus(http.StatusOK).AssertBodyContains("healthy")
}

// ─── CORS ─────────────────────────────────────────────────────────────────────

func TestCORS_AllowedOrigin_SetsHeaders(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
		AllowMethods: []string{http.MethodGet, http.MethodPost},
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/", map[string]string{"Origin": "https://example.com"}).
		AssertStatus(http.StatusOK).
		AssertHeader("Access-Control-Allow-Origin", "https://example.com")
}

func TestCORS_DisallowedOrigin_NoHeaders(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"https://example.com"},
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	resp := s.GET("/", map[string]string{"Origin": "https://evil.com"})
	resp.AssertStatus(http.StatusOK)
	if got := resp.Header("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no CORS header for disallowed origin, got %q", got)
	}
}

func TestCORS_Preflight_Returns204(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CORS())
	app.POST("/api", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	// In Astra, middleware is bundled with routes, so OPTIONS must be registered
	// for the CORS middleware to intercept the preflight request.
	app.OPTIONS("/api", func(c *astra.Ctx) error { return nil })
	s := testutil.NewServer(t, app)

	s.Do(http.MethodOptions, "/api", nil, map[string]string{
		"Origin":                        "https://example.com",
		"Access-Control-Request-Method": "POST",
	}).AssertStatus(http.StatusNoContent)
}

func TestCORS_NoOrigin_SkipsMiddleware(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.CORS())
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	// Request without Origin header — CORS middleware should not interfere.
	s.GET("/").AssertStatus(http.StatusOK)
}

// ─── RequestID ────────────────────────────────────────────────────────────────

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.RequestID())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", middleware.GetRequestID(c))
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/")
	resp.AssertStatus(http.StatusOK)
	id := resp.BodyString()
	if len(id) == 0 {
		t.Error("expected a non-empty request ID in the body")
	}
	if got := resp.Header("X-Request-ID"); got == "" {
		t.Error("expected X-Request-ID response header")
	}
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	const fixed = "fixed-id-1234"
	app := testutil.NewTestApp()
	app.Use(middleware.RequestID())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", middleware.GetRequestID(c))
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/", map[string]string{"X-Request-ID": fixed})
	resp.AssertStatus(http.StatusOK).AssertBodyContains(fixed)
	testutil.AssertEqual(t, fixed, resp.Header("X-Request-ID"))
}

// ─── JWT ──────────────────────────────────────────────────────────────────────

func TestJWT(t *testing.T) {
	const secret = "test-jwt-secret-key"

	validToken, err := sec.GenerateJWT(jwt.MapClaims{
		"sub": "user:42",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, secret)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}

	expiredToken, err := sec.GenerateJWT(jwt.MapClaims{
		"sub": "user:42",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}, secret)
	if err != nil {
		t.Fatalf("GenerateJWT (expired): %v", err)
	}

	app := testutil.NewTestApp()
	app.Use(sec.JWT(secret))
	app.GET("/", func(c *astra.Ctx) error {
		claims := sec.GetClaims(c)
		sub, _ := claims.GetSubject()
		return c.String(http.StatusOK, "%s", sub)
	})
	s := testutil.NewServer(t, app)

	tests := []struct {
		name   string
		auth   string
		status int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"invalid token", "Bearer not-a-jwt", http.StatusUnauthorized},
		{"wrong secret", "Bearer eyJhbGciOiJIUzI1NiJ9.e30.wrong", http.StatusUnauthorized},
		{"expired token", "Bearer " + expiredToken, http.StatusUnauthorized},
		{"valid token", "Bearer " + validToken, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{}
			if tc.auth != "" {
				headers["Authorization"] = tc.auth
			}
			s.GET("/", headers).AssertStatus(tc.status)
		})
	}
}

func TestJWT_ClaimsAccessible(t *testing.T) {
	const secret = "my-secret"
	token, _ := sec.GenerateJWT(jwt.MapClaims{
		"sub":     "alice",
		"role":    "admin",
		"exp":     time.Now().Add(time.Hour).Unix(),
	}, secret)

	app := testutil.NewTestApp()
	app.Use(sec.JWT(secret))
	app.GET("/me", func(c *astra.Ctx) error {
		claims := sec.GetClaims(c)
		sub, _ := claims.GetSubject()
		role, _ := claims.Extra["role"].(string)
		return c.String(http.StatusOK, "%s:%s", sub, role)
	})
	s := testutil.NewServer(t, app)

	s.GET("/me", map[string]string{"Authorization": "Bearer " + token}).
		AssertStatus(http.StatusOK).
		AssertBodyContains("alice:admin")
}

func TestJWT_TokenFromQuery(t *testing.T) {
	const secret = "qsecret"
	token, _ := sec.GenerateJWT(jwt.MapClaims{
		"sub": "bob",
		"exp": time.Now().Add(time.Hour).Unix(),
	}, secret)

	app := testutil.NewTestApp()
	app.Use(sec.JWTWithConfig(sec.JWTConfig{
		Secret:      sec.NewSecretString(secret),
		TokenLookup: "query:token",
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/?token="+token).AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusUnauthorized)
}

// jwtServer builds a test server with a JWT-protected GET / route.
func jwtServer(t *testing.T, cfg sec.JWTConfig) *testutil.Server {
	t.Helper()
	app := testutil.NewTestApp()
	app.Use(sec.JWTWithConfig(cfg))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	return testutil.NewServer(t, app)
}

// makeToken generates an HMAC HS256 token whose exp is now + offset.
// A negative offset produces an already-expired token.
func makeToken(t *testing.T, secret string, expOffset time.Duration) string {
	t.Helper()
	tok, err := sec.GenerateJWT(jwt.MapClaims{
		"sub": "u1",
		"exp": time.Now().Add(expOffset).Unix(),
	}, secret)
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}
	return tok
}

// ─── JWT leeway ───────────────────────────────────────────────────────────────

// TestJWT_Leeway_DefaultAcceptsWithin5s verifies that the default leeway (5s)
// allows a token that expired up to 5 seconds ago.
func TestJWT_Leeway_DefaultAcceptsWithin5s(t *testing.T) {
	const secret = "leeway-secret"
	tok := makeToken(t, secret, -3*time.Second) // expired 3s ago — within default 5s
	jwtServer(t, sec.JWTConfig{Secret: sec.NewSecretString(secret)}).
		GET("/", map[string]string{"Authorization": "Bearer " + tok}).
		AssertStatus(http.StatusOK)
}

// TestJWT_Leeway_DefaultRejectsBeyond5s verifies that a token expired more
// than 5 seconds ago is rejected under the default leeway.
func TestJWT_Leeway_DefaultRejectsBeyond5s(t *testing.T) {
	const secret = "leeway-secret"
	tok := makeToken(t, secret, -10*time.Second) // expired 10s ago — beyond 5s default
	jwtServer(t, sec.JWTConfig{Secret: sec.NewSecretString(secret)}).
		GET("/", map[string]string{"Authorization": "Bearer " + tok}).
		AssertStatus(http.StatusUnauthorized)
}

// TestJWT_Leeway_CustomAcceptsWithinWindow verifies that an explicit Leeway is
// honoured: a token expired 8 seconds ago is accepted when Leeway = 10s.
func TestJWT_Leeway_CustomAcceptsWithinWindow(t *testing.T) {
	const secret = "leeway-secret"
	tok := makeToken(t, secret, -8*time.Second)
	jwtServer(t, sec.JWTConfig{
		Secret: sec.NewSecretString(secret),
		Leeway: 10 * time.Second,
	}).
		GET("/", map[string]string{"Authorization": "Bearer " + tok}).
		AssertStatus(http.StatusOK)
}

// TestJWT_StrictLeeway_RejectsJustExpired verifies that StrictJWTLeeway
// rejects a token that expired even 1 second ago.
func TestJWT_StrictLeeway_RejectsJustExpired(t *testing.T) {
	const secret = "leeway-secret"
	tok := makeToken(t, secret, -1*time.Second)
	jwtServer(t, sec.JWTConfig{
		Secret: sec.NewSecretString(secret),
		Leeway: sec.StrictJWTLeeway,
	}).
		GET("/", map[string]string{"Authorization": "Bearer " + tok}).
		AssertStatus(http.StatusUnauthorized)
}

// TestJWT_StrictLeeway_AcceptsValid verifies that a still-valid token is
// always accepted regardless of leeway setting.
func TestJWT_StrictLeeway_AcceptsValid(t *testing.T) {
	const secret = "leeway-secret"
	tok := makeToken(t, secret, time.Hour)
	jwtServer(t, sec.JWTConfig{
		Secret: sec.NewSecretString(secret),
		Leeway: sec.StrictJWTLeeway,
	}).
		GET("/", map[string]string{"Authorization": "Bearer " + tok}).
		AssertStatus(http.StatusOK)
}

// ─── RateLimit ────────────────────────────────────────────────────────────────

func TestRateLimit_AllowsWithinBurst(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RateLimitWithConfig(sec.RateLimitConfig{
		Rate:    100,
		Burst:   5,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed-key" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	for i := 0; i < 5; i++ {
		s.GET("/").AssertStatus(http.StatusOK)
	}
}

func TestRateLimit_RejectsWhenExhausted(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.RateLimitWithConfig(sec.RateLimitConfig{
		Rate:    0.001, // near-zero refill rate
		Burst:   1,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed-key" },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)           // first: uses the one token
	s.GET("/").AssertStatus(http.StatusTooManyRequests) // second: bucket empty
}

func TestRateLimit_IndependentKeys(t *testing.T) {
	key := "user-a"
	app := testutil.NewTestApp()
	app.Use(sec.RateLimitWithConfig(sec.RateLimitConfig{
		Rate:    0.001,
		Burst:   1,
		KeyFunc: func(_ *astra.Ctx) string { return key },
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)

	// Switch key — new bucket, should allow again.
	key = "user-b"
	s.GET("/").AssertStatus(http.StatusOK)
}

// TestRateLimit_CleanupStopsOnContextCancel verifies that the internal cleanup
// goroutine exits when the supplied context is cancelled.  Without the fix
// every RateLimitWithConfig call in tests would leak a goroutine.
func TestRateLimit_CleanupStopsOnContextCancel(t *testing.T) {
	before := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	app := testutil.NewTestApp()
	app.Use(sec.RateLimitWithConfig(sec.RateLimitConfig{
		Rate:    100,
		Burst:   5,
		Context: ctx,
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })

	// Cancel the context — cleanup goroutine must exit.
	cancel()

	// Give the scheduler a moment to observe the cancellation.
	time.Sleep(20 * time.Millisecond)
	runtime.GC()

	after := runtime.NumGoroutine()
	// Allow ±2 goroutines for test infrastructure noise; the key assertion is
	// that we did not add a net goroutine.
	if after > before+2 {
		t.Errorf("goroutine leak: before=%d after=%d (expected <= %d)", before, after, before+2)
	}
}

// TestNewRateLimiter_StopFuncExits verifies that the stop function returned by
// NewRateLimiter cancels the cleanup goroutine without requiring a manual context.
func TestNewRateLimiter_StopFuncExits(t *testing.T) {
	before := runtime.NumGoroutine()

	_, stop := sec.NewRateLimiter(100, 5)

	// Stop the cleanup goroutine immediately.
	stop()

	time.Sleep(20 * time.Millisecond)
	runtime.GC()

	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("goroutine leak after stop: before=%d after=%d", before, after)
	}
}

// TestNewRateLimiter_StillServesAfterStop verifies that the middleware continues
// to serve requests correctly after stop() is called (the store is still alive;
// only the cleanup goroutine exits).
func TestNewRateLimiter_StillServesAfterStop(t *testing.T) {
	mw, stop := sec.NewRateLimiter(100, 10)
	defer stop()

	app := testutil.NewTestApp()
	app.Use(mw)
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	stop() // stop cleanup goroutine early

	// Requests must still succeed.
	for i := 0; i < 5; i++ {
		s.GET("/").AssertStatus(http.StatusOK)
	}
}

// ─── SlidingWindow ────────────────────────────────────────────────────────────

func TestSlidingWindow_AllowsWithinLimit_WithContext(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   5,
		Window:  time.Second,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed" },
		Context: context.Background(),
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	for i := 0; i < 5; i++ {
		s.GET("/").AssertStatus(http.StatusOK)
	}
}

func TestSlidingWindow_RejectsWhenExhausted_WithContext(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   1,
		Window:  time.Second,
		KeyFunc: func(_ *astra.Ctx) string { return "fixed" },
		Context: context.Background(),
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/").AssertStatus(http.StatusOK)
	s.GET("/").AssertStatus(http.StatusTooManyRequests)
}

// TestSlidingWindow_CleanupStopsOnContextCancel verifies that the internal
// cleanup goroutine exits when the supplied context is cancelled.
func TestSlidingWindow_CleanupStopsOnContextCancel(t *testing.T) {
	before := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	app := testutil.NewTestApp()
	app.Use(sec.SlidingWindowWithConfig(sec.SlidingWindowConfig{
		Limit:   100,
		Window:  time.Second,
		Context: ctx,
	}))
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })

	cancel()
	time.Sleep(20 * time.Millisecond)
	runtime.GC()

	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("goroutine leak: before=%d after=%d", before, after)
	}
}

// TestNewSlidingWindow_StopFuncExits verifies the stop function cancels the
// cleanup goroutine.
func TestNewSlidingWindow_StopFuncExits(t *testing.T) {
	before := runtime.NumGoroutine()

	_, stop := sec.NewSlidingWindow(100, time.Second)
	stop()

	time.Sleep(20 * time.Millisecond)
	runtime.GC()

	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("goroutine leak after stop: before=%d after=%d", before, after)
	}
}

// TestNewSlidingWindow_StillServesAfterStop verifies requests succeed after
// stop() is called (only the cleanup goroutine exits; the store stays alive).
func TestNewSlidingWindow_StillServesAfterStop(t *testing.T) {
	mw, stop := sec.NewSlidingWindow(100, time.Second)
	defer stop()

	app := testutil.NewTestApp()
	app.Use(mw)
	app.GET("/", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	stop()

	for i := 0; i < 5; i++ {
		s.GET("/").AssertStatus(http.StatusOK)
	}
}

// ─── RouteQuotaMiddleware ─────────────────────────────────────────────────────

func TestRouteQuota_PerRouteLimitEnforced(t *testing.T) {
	mw, stop := sec.NewRouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/slow", Limit: 1, Window: time.Second},
		},
		DefaultLimit:  100,
		DefaultWindow: time.Second,
		KeyFunc:       func(_ *astra.Ctx) string { return "fixed" },
	})
	defer stop()

	app := testutil.NewTestApp()
	app.Use(mw)
	app.GET("/slow", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	app.GET("/fast", func(c *astra.Ctx) error { return c.String(http.StatusOK, "ok") })
	s := testutil.NewServer(t, app)

	s.GET("/slow").AssertStatus(http.StatusOK)
	s.GET("/slow").AssertStatus(http.StatusTooManyRequests)
	// /fast is governed by the higher default limit — still allowed.
	s.GET("/fast").AssertStatus(http.StatusOK)
}

// TestRouteQuota_CleanupStopsOnContextCancel verifies all cleanup goroutines
// (one per route + one default) exit when the context is cancelled.
func TestRouteQuota_CleanupStopsOnContextCancel(t *testing.T) {
	before := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	_ = sec.RouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/a", Limit: 10, Window: time.Second},
			{Prefix: "/b", Limit: 20, Window: time.Second},
		},
		DefaultLimit:  100,
		DefaultWindow: time.Second,
		Context:       ctx,
	})

	cancel()
	time.Sleep(20 * time.Millisecond)
	runtime.GC()

	after := runtime.NumGoroutine()
	// 3 goroutines were spawned (2 routes + 1 default); all should have exited.
	if after > before+2 {
		t.Errorf("goroutine leak: before=%d after=%d", before, after)
	}
}

// TestNewRouteQuotaMiddleware_StopFuncExits verifies the returned stop function
// terminates all cleanup goroutines.
func TestNewRouteQuotaMiddleware_StopFuncExits(t *testing.T) {
	before := runtime.NumGoroutine()

	_, stop := sec.NewRouteQuotaMiddleware(sec.RouteQuotaConfig{
		Routes: []sec.RouteQuota{
			{Prefix: "/x", Limit: 10, Window: time.Second},
		},
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})
	stop()

	time.Sleep(20 * time.Millisecond)
	runtime.GC()

	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("goroutine leak after stop: before=%d after=%d", before, after)
	}
}

// ─── CSRF ─────────────────────────────────────────────────────────────────────

func TestCSRF_SafeMethodsPass(t *testing.T) {
	ts := newCSRFServer()
	defer ts.Close()

	client := &http.Client{}
	// In Astra, only registered methods pass through middleware;
	// GET is registered for /data so it's the safe-method to test.
	for _, method := range []string{http.MethodGet} {
		req, _ := http.NewRequestWithContext(context.Background(), method, ts.URL+"/data", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: want 200, got %d", method, resp.StatusCode)
		}
	}
}

func TestCSRF_PostWithoutToken_Returns403(t *testing.T) {
	ts := newCSRFServer()
	defer ts.Close()

	client := &http.Client{}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/submit", nil)
	resp, _ := client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", resp.StatusCode)
	}
}

func TestCSRF_PostWithValidDoubleSubmit_Passes(t *testing.T) {
	ts := newCSRFServer()
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Step 1: GET — retrieves CSRF token from response body; cookie set automatically.
	resp1, _ := client.Get(ts.URL + "/token")
	body, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	token := strings.TrimSpace(string(body))

	if token == "" {
		t.Fatal("expected non-empty CSRF token from GET /token")
	}

	// Step 2: POST — cookie jar sends the CSRF cookie; also set X-CSRF-Token header.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/submit", nil)
	req.Header.Set("X-CSRF-Token", token)
	resp2, _ := client.Do(req)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp2.StatusCode)
	}
}

func TestCSRF_GetCSRFToken_NonEmpty(t *testing.T) {
	ts := newCSRFServer()
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/token")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(strings.TrimSpace(string(body))) == 0 {
		t.Error("GetCSRFToken should return a non-empty token")
	}
}

// newCSRFServer creates an httptest.Server with CSRF middleware applied.
func newCSRFServer() *httptest.Server {
	app := testutil.NewTestApp()
	app.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		Secret:       []byte("super-secret-key-for-testing-32b"),
		CookieSecure: false, // httptest runs on HTTP; Secure cookies won't be sent.
	}))
	app.GET("/token", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", middleware.GetCSRFToken(c))
	})
	app.GET("/data", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "data")
	})
	app.POST("/submit", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "submitted")
	})
	return httptest.NewServer(app)
}

// ─── Timeout ──────────────────────────────────────────────────────────────────

func TestTimeout_FastHandler_Passes(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Timeout(100 * time.Millisecond))
	app.GET("/fast", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "fast")
	})
	s := testutil.NewServer(t, app)

	s.GET("/fast").AssertStatus(http.StatusOK).AssertBodyContains("fast")
}

func TestTimeout_SlowHandler_Returns504(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Timeout(5 * time.Millisecond))
	app.GET("/slow", func(c *astra.Ctx) error {
		// Respect context cancellation so the timeout middleware can detect it.
		select {
		case <-c.Request().Context().Done():
			return nil // don't write; let timeout middleware respond
		case <-time.After(200 * time.Millisecond):
			return c.String(http.StatusOK, "too late")
		}
	})
	s := testutil.NewServer(t, app)

	s.GET("/slow").AssertStatus(http.StatusGatewayTimeout)
}

// ─── Compress ─────────────────────────────────────────────────────────────────

func TestCompress_LargeResponse_IsGzipped(t *testing.T) {
	largeBody := strings.Repeat("Hello Astra! ", 200) // ~2600 bytes > 1024 MinSize

	app := testutil.NewTestApp()
	app.Use(middleware.Compress())
	app.GET("/big", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", largeBody)
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/big", map[string]string{"Accept-Encoding": "gzip"})
	resp.AssertStatus(http.StatusOK).AssertHeaderContains("Content-Encoding", "gzip")

	// Verify the body is valid gzip.
	r, err := gzip.NewReader(strings.NewReader(resp.BodyString()))
	if err != nil {
		t.Fatalf("body is not valid gzip: %v", err)
	}
	decoded, _ := io.ReadAll(r)
	r.Close()
	if string(decoded) != largeBody {
		t.Errorf("decoded body mismatch")
	}
}

func TestCompress_SmallResponse_NotCompressed(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Compress())
	app.GET("/small", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "hi")
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/small", map[string]string{"Accept-Encoding": "gzip"})
	resp.AssertStatus(http.StatusOK)
	if got := resp.Header("Content-Encoding"); got == "gzip" {
		t.Errorf("small response should not be gzip-compressed, but got Content-Encoding: %s", got)
	}
}

func TestCompress_NoAcceptEncoding_NotCompressed(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Compress())
	app.GET("/", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", strings.Repeat("x", 2000))
	})
	s := testutil.NewServer(t, app)

	// No Accept-Encoding header — no compression.
	resp := s.GET("/")
	if got := resp.Header("Content-Encoding"); got == "gzip" {
		t.Errorf("no Accept-Encoding sent, but response was compressed")
	}
}

func TestCompress_ExcludedExtension_NotCompressed(t *testing.T) {
	app := testutil.NewTestApp()
	app.Use(middleware.Compress())
	app.GET("/image.png", func(c *astra.Ctx) error {
		return c.String(http.StatusOK, "%s", strings.Repeat("x", 2000))
	})
	s := testutil.NewServer(t, app)

	resp := s.GET("/image.png", map[string]string{"Accept-Encoding": "gzip"})
	if got := resp.Header("Content-Encoding"); got == "gzip" {
		t.Errorf(".png should not be compressed, but got Content-Encoding: gzip")
	}
}
