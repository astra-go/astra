// Package oauth2 provides OAuth2 / OIDC client integration for Astra.
//
// It wraps golang.org/x/oauth2 and adds:
//   - PKCE (S256) support for public clients
//   - OIDC UserInfo fetch
//   - Cookie-backed CSRF-safe state store (HMAC-SHA256, http-only, SameSite=Lax)
//   - Pluggable OnSuccess callback for token persistence and redirect
//
// # Authorization Code flow
//
//	import (
//	    "golang.org/x/oauth2"
//	    "golang.org/x/oauth2/google"
//	    astraoauth2 "github.com/astra-go/astra/auth/oauth2"
//	)
//
//	cfg := astraoauth2.Config{
//	    ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
//	    ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
//	    RedirectURL:  "https://myapp.com/auth/callback",
//	    Scopes:       []string{"openid", "email", "profile"},
//	    Endpoint:     google.Endpoint,
//	    PKCE:         true,
//	    UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
//	    OnSuccess: func(c *astra.Ctx, tok *oauth2.Token, info map[string]any) error {
//	        // store token in session, redirect to /dashboard
//	        return c.Redirect(http.StatusFound, "/dashboard")
//	    },
//	}
//
//	app.GET("/auth/login",    astraoauth2.LoginHandler(cfg))
//	app.GET("/auth/callback", astraoauth2.CallbackHandler(cfg))
//
// # Token refresh
//
//	newTok, err := astraoauth2.RefreshToken(ctx, cfg, expiredToken)
//
// # OIDC UserInfo
//
//	info, err := astraoauth2.FetchUserInfo(ctx, cfg, token)
//	// info["sub"], info["email"], info["name"] …
package oauth2

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	goauth2 "golang.org/x/oauth2"

	"github.com/astra-go/astra"
)

const (
	stateCookieName    = "_oauth2_state"
	verifierCookieName = "_oauth2_pkce_verifier"
	cookieMaxAge       = 10 * 60 // 10 minutes
)

// StateStore persists and validates the anti-CSRF state parameter.
type StateStore interface {
	Save(c *astra.Ctx, state string) error
	Verify(c *astra.Ctx, state string) error
}

// Config configures an OAuth2 / OIDC client flow.
type Config struct {
	// ClientID and ClientSecret are the OAuth2 application credentials.
	ClientID     string
	ClientSecret string

	// RedirectURL must exactly match the URL registered with the provider.
	RedirectURL string

	// Scopes requested from the provider.
	// Include "openid" for OIDC flows.
	Scopes []string

	// Endpoint contains the provider's authorization and token URLs.
	// Use a constant from golang.org/x/oauth2/<provider> or build manually.
	Endpoint goauth2.Endpoint

	// PKCE enables the Proof Key for Code Exchange extension (RFC 7636).
	// Recommended for all new public client integrations.
	PKCE bool

	// UserInfoURL is the OIDC UserInfo endpoint.
	// When set, CallbackHandler fetches user info and passes it to OnSuccess.
	UserInfoURL string

	// StateStore persists and validates the anti-CSRF state parameter.
	// Default: a secure cookie-backed store using HMAC-SHA256.
	StateStore StateStore

	// StateKey is the HMAC-SHA256 key used to sign state cookies in the default
	// cookie-backed StateStore. Must be exactly 32 bytes.
	// If empty, a random key is generated at startup — cookies will be invalidated
	// on server restart and are unsuitable for multi-instance deployments.
	StateKey []byte

	// OnSuccess is called after a successful token exchange.
	// tok is the new token; info is the OIDC UserInfo map (nil if UserInfoURL
	// is empty). Typically used to store the token in a session and redirect.
	// Required — if nil, CallbackHandler returns a 500 error.
	OnSuccess func(c *astra.Ctx, tok *goauth2.Token, info map[string]any) error
}

func (cfg *Config) oauthConfig() *goauth2.Config {
	return &goauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
		Endpoint:     cfg.Endpoint,
	}
}

// LoginHandler redirects the user to the authorization server.
//
// It generates a cryptographically random state, persists it via StateStore,
// and, when PKCE is enabled, generates a code_verifier / code_challenge pair
// (also persisted via a cookie).
func LoginHandler(cfg Config) astra.HandlerFunc {
	if cfg.StateStore == nil {
		cfg.StateStore = newCookieStateStore(cfg.StateKey)
	}

	return func(c *astra.Ctx) error {
		state, err := randomBase64(32)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, astra.Map{"error": "state generation failed"})
		}
		if err := cfg.StateStore.Save(c, state); err != nil {
			return c.JSON(http.StatusInternalServerError, astra.Map{"error": "state save failed"})
		}

		authCfg := cfg.oauthConfig()
		opts := []goauth2.AuthCodeOption{goauth2.AccessTypeOnline}

		if cfg.PKCE {
			verifier := goauth2.GenerateVerifier()
			setCookie(c.Writer(), verifierCookieName, verifier, cookieMaxAge)
			opts = append(opts, goauth2.S256ChallengeOption(verifier))
		}

		url := authCfg.AuthCodeURL(state, opts...)
		return c.Redirect(http.StatusFound, url)
	}
}

// CallbackHandler handles the authorization code callback.
//
// It validates the state, exchanges the code for a token, optionally fetches
// OIDC UserInfo, and calls cfg.OnSuccess.
func CallbackHandler(cfg Config) astra.HandlerFunc {
	if cfg.StateStore == nil {
		cfg.StateStore = newCookieStateStore(cfg.StateKey)
	}

	return func(c *astra.Ctx) error {
		q := c.Request().URL.Query()

		// Check provider error
		if errParam := q.Get("error"); errParam != "" {
			return c.JSON(http.StatusBadRequest, astra.Map{
				"error":             errParam,
				"error_description": q.Get("error_description"),
			})
		}

		// Validate state
		state := q.Get("state")
		if err := cfg.StateStore.Verify(c, state); err != nil {
			return c.JSON(http.StatusBadRequest, astra.Map{"error": "invalid state: " + err.Error()})
		}

		authCfg := cfg.oauthConfig()
		code := q.Get("code")

		opts := []goauth2.AuthCodeOption{}
		if cfg.PKCE {
			verifier, err := readCookie(c.Request(), verifierCookieName)
			if err != nil {
				return c.JSON(http.StatusBadRequest, astra.Map{"error": "PKCE verifier missing"})
			}
			deleteCookie(c.Writer(), verifierCookieName)
			opts = append(opts, goauth2.VerifierOption(verifier))
		}

		tok, err := authCfg.Exchange(c.Request().Context(), code, opts...)
		if err != nil {
			return c.JSON(http.StatusBadRequest, astra.Map{"error": "token exchange failed: " + err.Error()})
		}

		var userInfo map[string]any
		if cfg.UserInfoURL != "" {
			userInfo, err = FetchUserInfo(c.Request().Context(), cfg, tok)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, astra.Map{"error": "userinfo fetch failed: " + err.Error()})
			}
		}

		if cfg.OnSuccess == nil {
			return c.JSON(http.StatusInternalServerError, astra.Map{"error": "OnSuccess not configured"})
		}
		return cfg.OnSuccess(c, tok, userInfo)
	}
}

// RefreshToken exchanges a refresh token for a new access token.
func RefreshToken(ctx context.Context, cfg Config, tok *goauth2.Token) (*goauth2.Token, error) {
	ts := cfg.oauthConfig().TokenSource(ctx, tok)
	newTok, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("oauth2: refresh token: %w", err)
	}
	return newTok, nil
}

// FetchUserInfo fetches the OIDC UserInfo endpoint.
// cfg.UserInfoURL must be set.
func FetchUserInfo(ctx context.Context, cfg Config, tok *goauth2.Token) (map[string]any, error) {
	if cfg.UserInfoURL == "" {
		return nil, fmt.Errorf("oauth2: UserInfoURL is empty")
	}
	client := cfg.oauthConfig().Client(ctx, tok)
	client.Timeout = 10 * time.Second

	resp, err := client.Get(cfg.UserInfoURL)
	if err != nil {
		return nil, fmt.Errorf("oauth2: userinfo GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oauth2: userinfo returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("oauth2: userinfo read: %w", err)
	}

	var info map[string]any
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("oauth2: userinfo unmarshal: %w", err)
	}
	return info, nil
}

// ─── Cookie-based StateStore ──────────────────────────────────────────────────

// cookieStateStore stores the state in an HMAC-SHA256-signed http-only cookie.
type cookieStateStore struct{ key []byte }

func newCookieStateStore(key []byte) *cookieStateStore {
	if len(key) == 0 {
		slog.Warn("oauth2: StateKey not set; state cookies will be invalidated on restart and are unsuitable for multi-instance deployments")
		key = make([]byte, 32)
		_, _ = rand.Read(key)
	}
	return &cookieStateStore{key: key}
}

func (s *cookieStateStore) Save(c *astra.Ctx, state string) error {
	signed := s.sign(state)
	setCookie(c.Writer(), stateCookieName, signed, cookieMaxAge)
	return nil
}

func (s *cookieStateStore) Verify(c *astra.Ctx, state string) error {
	stored, err := readCookie(c.Request(), stateCookieName)
	if err != nil {
		return fmt.Errorf("oauth2: state cookie missing")
	}
	deleteCookie(c.Writer(), stateCookieName)
	if !s.verify(state, stored) {
		return fmt.Errorf("oauth2: state mismatch")
	}
	return nil
}

func (s *cookieStateStore) sign(value string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(value))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return value + "." + sig
}

func (s *cookieStateStore) verify(value, stored string) bool {
	expected := s.sign(value)
	return hmac.Equal([]byte(expected), []byte(stored))
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func randomBase64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

func readCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func deleteCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:   name,
		MaxAge: -1,
		Path:   "/",
	})
}
