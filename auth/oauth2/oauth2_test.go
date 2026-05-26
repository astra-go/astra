package oauth2_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	goauth2 "golang.org/x/oauth2"

	astraoauth2 "github.com/astra-go/astra/auth/oauth2"
	"github.com/astra-go/astra/testutil"
)

// noRedirectClient is an HTTP client that does not follow redirects.
// Returned as-is so callers can inspect the 3xx response directly.
var noRedirectClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// ─── LoginHandler ─────────────────────────────────────────────────────────────

func TestLoginHandler_RedirectsToAuthURL(t *testing.T) {
	// Fake OAuth2 provider — we only need a valid URL for the AuthURL; we never
	// actually make a request to it because we stop redirect-following.
	fakeProv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeProv.Close()

	app := testutil.NewTestApp()
	cfg := astraoauth2.Config{
		ClientID:    "client-id",
		RedirectURL: "http://localhost/callback",
		Scopes:      []string{"openid"},
		Endpoint: goauth2.Endpoint{
			AuthURL:  fakeProv.URL + "/auth",
			TokenURL: fakeProv.URL + "/token",
		},
	}
	app.GET("/login", astraoauth2.LoginHandler(cfg))
	s := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL()+"/login", nil)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Error("expected Location header on redirect")
	}
}

func TestLoginHandler_PKCE_LocationContainsCodeChallenge(t *testing.T) {
	fakeProv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer fakeProv.Close()

	app := testutil.NewTestApp()
	cfg := astraoauth2.Config{
		ClientID: "client-id",
		PKCE:     true,
		Endpoint: goauth2.Endpoint{
			AuthURL:  fakeProv.URL + "/auth",
			TokenURL: fakeProv.URL + "/token",
		},
	}
	app.GET("/login", astraoauth2.LoginHandler(cfg))
	s := testutil.NewServer(t, app)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL()+"/login", nil)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatal("expected Location header")
	}
	// PKCE S256 adds code_challenge to the redirect URL.
	if !containsSubstring(loc, "code_challenge") {
		t.Errorf("expected code_challenge in Location %q", loc)
	}
}

// ─── CallbackHandler ─────────────────────────────────────────────────────────

func TestCallbackHandler_ProviderError_Returns400(t *testing.T) {
	app := testutil.NewTestApp()
	cfg := astraoauth2.Config{
		ClientID: "client-id",
		Endpoint: goauth2.Endpoint{
			AuthURL:  "http://localhost/auth",
			TokenURL: "http://localhost/token",
		},
	}
	app.GET("/callback", astraoauth2.CallbackHandler(cfg))
	s := testutil.NewServer(t, app)

	resp := s.GET("/callback?error=access_denied&error_description=user+denied")
	if resp.Status() != http.StatusBadRequest {
		t.Errorf("expected 400 for provider error, got %d", resp.Status())
	}
}

func TestCallbackHandler_InvalidState_Returns400(t *testing.T) {
	app := testutil.NewTestApp()
	cfg := astraoauth2.Config{
		ClientID: "client-id",
		Endpoint: goauth2.Endpoint{
			AuthURL:  "http://localhost/auth",
			TokenURL: "http://localhost/token",
		},
	}
	app.GET("/callback", astraoauth2.CallbackHandler(cfg))
	s := testutil.NewServer(t, app)

	// No state cookie set → state verification must fail.
	resp := s.GET("/callback?code=authcode&state=bad-state")
	if resp.Status() != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid state, got %d", resp.Status())
	}
}

// ─── FetchUserInfo ────────────────────────────────────────────────────────────

func TestFetchUserInfo_EmptyURL_ReturnsError(t *testing.T) {
	cfg := astraoauth2.Config{}
	_, err := astraoauth2.FetchUserInfo(context.Background(), cfg, &goauth2.Token{})
	if err == nil {
		t.Fatal("expected error when UserInfoURL is empty")
	}
}

func TestFetchUserInfo_MockServer_ReturnsUserInfo(t *testing.T) {
	wantInfo := map[string]any{
		"sub":   "user-123",
		"email": "user@example.com",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wantInfo)
	}))
	defer srv.Close()

	cfg := astraoauth2.Config{
		UserInfoURL: srv.URL + "/userinfo",
		Endpoint: goauth2.Endpoint{
			AuthURL:  srv.URL + "/auth",
			TokenURL: srv.URL + "/token",
		},
	}
	// Use a valid token with a future expiry so oauth2 client won't try to refresh.
	tok := &goauth2.Token{
		AccessToken: "test-token",
		Expiry:      time.Now().Add(time.Hour),
	}

	info, err := astraoauth2.FetchUserInfo(context.Background(), cfg, tok)
	testutil.AssertNoError(t, err)

	if info["sub"] != "user-123" {
		t.Errorf("info[sub] = %v, want user-123", info["sub"])
	}
}

func TestFetchUserInfo_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := astraoauth2.Config{
		UserInfoURL: srv.URL + "/userinfo",
	}
	tok := &goauth2.Token{
		AccessToken: "bad-token",
		Expiry:      time.Now().Add(time.Hour),
	}

	_, err := astraoauth2.FetchUserInfo(context.Background(), cfg, tok)
	if err == nil {
		t.Error("expected error from non-200 UserInfo response")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
