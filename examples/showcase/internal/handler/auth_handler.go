package handler

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"github.com/astra-go/astra"
	astraoauth2 "github.com/astra-go/astra/auth/oauth2"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	goauth2 "golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// oauthUserSvc is the subset of UserSvc needed by AuthHandler.
type oauthUserSvc interface {
	UpsertOAuth(ctx context.Context, tenantID uint, provider, sub, email, name string) (*domain.User, error)
	IssueToken(u *domain.User) (string, error)
}

// AuthHandler wires OAuth2 login/callback and JWT issuance.
type AuthHandler struct {
	svc          oauthUserSvc
	defaultTenant uint // tenant used for OAuth logins (demo: single-tenant)
	googleCfg    astraoauth2.Config
	githubCfg    astraoauth2.Config
}

// NewAuthHandler builds an AuthHandler from environment variables.
// Required env vars:
//
//	GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET
//	GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET
//	APP_BASE_URL  (e.g. http://localhost:8080)
func NewAuthHandler(svc oauthUserSvc, defaultTenantID uint) *AuthHandler {
	base := os.Getenv("APP_BASE_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	stateKey := []byte(os.Getenv("OAUTH2_STATE_KEY"))

	onSuccess := func(provider string) func(*astra.Ctx, *goauth2.Token, map[string]any) error {
		return func(c *astra.Ctx, tok *goauth2.Token, info map[string]any) error {
			sub, _ := info["sub"].(string)
			if sub == "" {
				// GitHub uses "id" (numeric) instead of "sub"
				if id, ok := info["id"].(float64); ok {
					sub = strconv.FormatInt(int64(id), 10)
				}
			}
			email, _ := info["email"].(string)
			name, _ := info["name"].(string)
			if name == "" {
				name, _ = info["login"].(string) // GitHub
			}

			u, err := svc.UpsertOAuth(c.Request().Context(), defaultTenantID, provider, sub, email, name)
			if err != nil {
				return err
			}
			token, err := svc.IssueToken(u)
			if err != nil {
				return err
			}
			return c.JSON(http.StatusOK, domain.LoginResp{Token: token, ExpiresIn: 86400})
		}
	}

	return &AuthHandler{
		svc:           svc,
		defaultTenant: defaultTenantID,
		googleCfg: astraoauth2.Config{
			ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			RedirectURL:  base + "/auth/google/callback",
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
			PKCE:         true,
			UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
			StateKey:     stateKey,
			OnSuccess:    onSuccess("google"),
		},
		githubCfg: astraoauth2.Config{
			ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			RedirectURL:  base + "/auth/github/callback",
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
			UserInfoURL:  "https://api.github.com/user",
			StateKey:     stateKey,
			OnSuccess:    onSuccess("github"),
		},
	}
}

func (h *AuthHandler) GoogleLogin(c *astra.Ctx) error {
	return astraoauth2.LoginHandler(h.googleCfg)(c)
}

func (h *AuthHandler) GoogleCallback(c *astra.Ctx) error {
	return astraoauth2.CallbackHandler(h.googleCfg)(c)
}

func (h *AuthHandler) GithubLogin(c *astra.Ctx) error {
	return astraoauth2.LoginHandler(h.githubCfg)(c)
}

func (h *AuthHandler) GithubCallback(c *astra.Ctx) error {
	return astraoauth2.CallbackHandler(h.githubCfg)(c)
}
