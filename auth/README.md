# Auth — Authentication and Authorization

Provides OAuth2 login and Casbin-based RBAC authorization support.

## Features

- **OAuth2**: Supports Authorization Code flow with PKCE, OIDC UserInfo fetching, Token auto-refresh
- **CSRF Protection**: HMAC-SHA256 signed state stored in HttpOnly Cookie, prevents CSRF attacks
- **RBAC**: Casbin-based middleware-style authorization, supports custom subject/resource/action extractors
- **Pluggable**: Token persistence and post-auth redirect logic are all customizable

## Quick Start

### OAuth2 Login

```go
import (
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    astraoauth2 "github.com/astra-go/astra/auth/oauth2"
)

cfg := astraoauth2.Config{
    ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
    ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
    RedirectURL:  "https://myapp.com/auth/callback",
    Scopes:       []string{"openid", "email", "profile"},
    Endpoint:     google.Endpoint,
    PKCE:         true,
    OnSuccess: func(c *astra.Ctx, tok *oauth2.Token, info map[string]any) error {
        // Store Token here and redirect
        return c.Redirect(http.StatusFound, "/dashboard")
    },
}

app.GET("/auth/login",    astraoauth2.LoginHandler(cfg))
app.GET("/auth/callback", astraoauth2.CallbackHandler(cfg))
```

### RBAC Authorization Middleware

```go
import (
    "github.com/casbin/casbin/v2"
    "github.com/astra-go/astra/auth/rbac"
)

e, _ := casbin.NewEnforcer("model.conf", "policy.csv")
app.Use(rbac.Middleware(rbac.Config{Enforcer: e}))
```

## OAuth2 API

### Config

```go
type Config struct {
    ClientID, ClientSecret string       // OAuth2 app credentials
    RedirectURL            string       // Callback URL
    Scopes                 []string     // Requested permission scopes
    Endpoint               oauth2.Endpoint
    PKCE                   bool         // Enable PKCE (recommended)
    UserInfoURL            string       // OIDC UserInfo endpoint
    OnSuccess              func(*astra.Ctx, *oauth2.Token, map[string]any) error
}
```

### LoginHandler / CallbackHandler

```go
// Initiate OAuth2 login
LoginHandler(cfg Config) astra.HandlerFunc

// Handle callback
CallbackHandler(cfg Config) astra.HandlerFunc
```

### Other Utilities

```go
// Refresh expired token
RefreshToken(ctx, cfg, expiredToken) (*oauth2.Token, error)

// Fetch user info
FetchUserInfo(ctx, cfg, token) (map[string]any, error)
```

## RBAC API

### Config

```go
type Config struct {
    Enforcer    *casbin.Enforcer    // Casbin policy enforcer (required)
    GetSubject  func(*astra.Ctx) string  // Extract subject from request, default: "user_id"
    GetObject   func(*astra.Ctx) string  // Extract resource, default: URL path
    GetAction   func(*astra.Ctx) string  // Extract action, default: HTTP Method
    Skipper     func(*astra.Ctx) bool    // Skip check function
    ErrorHandler astra.HandlerFunc       // Custom 403 response handler
}
```

### Subject Extractor Example (with JWT)

```go
app.Use(rbac.Middleware(rbac.Config{
    Enforcer: e,
    GetSubject: func(c *astra.Ctx) string {
        uid, _ := c.Get("user_id")
        return fmt.Sprintf("%v", uid)
    },
}))
```

## Config

### OAuth2 PKCE Mode

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `PKCE` | `bool` | `false` | Enable S256 PKCE in production recommended |

### RBAC Skipper

```go
// Skip health check routes
Skipper: func(c *astra.Ctx) bool {
    return c.Path() == "/health" || c.Path() == "/ready"
}
```

## Module Dependencies

- `golang.org/x/oauth2` — OAuth2/OIDC core library
- `github.com/casbin/casbin/v2` — RBAC policy engine

## Notes

- OAuth2 callback URL must exactly match registered Redirect URL with Provider
- PKCE mode forces S256 method; doesn't support pure redirect_uri implicit grant
- After Casbin policy file changes, call `e.LoadPolicy()` for hot reload without restart
