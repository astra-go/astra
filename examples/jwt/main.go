// JWT example: register, login (access + refresh tokens), protected routes.
//
// Token flow:
//   POST /auth/register  → create account
//   POST /auth/login     → { access_token, refresh_token, expires_in }
//   POST /auth/refresh   → new access_token using refresh_token
//   GET  /api/me         → protected: returns current user from claims
package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	sec "github.com/astra-go/astra/middleware/security"
)

const (
	accessSecret  = "access-secret-change-in-prod"
	refreshSecret = "refresh-secret-change-in-prod"
	accessTTL     = 15 * time.Minute
	refreshTTL    = 7 * 24 * time.Hour
)

// ─── User store ────────────────────────────────────────────────────────────────

type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	passwordHash []byte
}

type UserStore struct {
	mu      sync.RWMutex
	users   map[int64]*User
	byEmail map[string]*User
	counter int64
}

func NewUserStore() *UserStore {
	return &UserStore{
		users:   make(map[int64]*User),
		byEmail: make(map[string]*User),
	}
}

func (s *UserStore) Register(name, email, password string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byEmail[email]; exists {
		return nil, fmt.Errorf("email already registered")
	}
	s.counter++
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	u := &User{
		ID:           s.counter,
		Email:        email,
		Name:         name,
		passwordHash: hash,
	}
	s.users[u.ID] = u
	s.byEmail[email] = u
	return u, nil
}

func (s *UserStore) Authenticate(email, password string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byEmail[email]
	if !ok {
		return nil, false
	}
	return u, bcrypt.CompareHashAndPassword(u.passwordHash, []byte(password)) == nil
}

func (s *UserStore) Find(id int64) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

// ─── Token helpers ─────────────────────────────────────────────────────────────

func issueTokens(userID int64) (access, refresh string, err error) {
	now := time.Now()
	access, err = sec.GenerateJWT(jwt.MapClaims{
		"sub":  fmt.Sprintf("%d", userID),
		"type": "access",
		"iat":  now.Unix(),
		"exp":  now.Add(accessTTL).Unix(),
	}, accessSecret)
	if err != nil {
		return
	}
	refresh, err = sec.GenerateJWT(jwt.MapClaims{
		"sub":  fmt.Sprintf("%d", userID),
		"type": "refresh",
		"iat":  now.Unix(),
		"exp":  now.Add(refreshTTL).Unix(),
	}, refreshSecret)
	return
}

func parseToken(tokenStr, secret string) (*sec.Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &sec.Claims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(secret), nil
		},
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(*sec.Claims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

type AuthHandler struct{ store *UserStore }

func (h *AuthHandler) Register(c *astra.Ctx) error {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	if req.Name == "" || req.Email == "" || req.Password == "" {
		return astra.NewHTTPError(http.StatusBadRequest, "name, email, and password are required")
	}
	u, err := h.store.Register(req.Name, req.Email, req.Password)
	if err != nil {
		return astra.NewHTTPError(http.StatusConflict, err.Error())
	}
	return c.JSON(http.StatusCreated, astra.Map{
		"id":    u.ID,
		"name":  u.Name,
		"email": u.Email,
	})
}

func (h *AuthHandler) Login(c *astra.Ctx) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	u, ok := h.store.Authenticate(req.Email, req.Password)
	if !ok {
		return astra.NewHTTPError(http.StatusUnauthorized, "invalid credentials")
	}
	access, refresh, err := issueTokens(u.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{
		"access_token":  access,
		"refresh_token": refresh,
		"expires_in":    int(accessTTL.Seconds()),
		"token_type":    "Bearer",
	})
}

func (h *AuthHandler) Refresh(c *astra.Ctx) error {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BindJSON(&req); err != nil {
		return err
	}
	claims, err := parseToken(req.RefreshToken, refreshSecret)
	if err != nil {
		return astra.NewHTTPError(http.StatusUnauthorized, "invalid refresh token")
	}
	if v, _ := claims.Extra["type"]; v != "refresh" {
		return astra.NewHTTPError(http.StatusUnauthorized, "wrong token type")
	}
	sub, _ := claims.GetSubject()
	var userID int64
	fmt.Sscanf(sub, "%d", &userID)

	access, newRefresh, err := issueTokens(userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{
		"access_token":  access,
		"refresh_token": newRefresh,
		"expires_in":    int(accessTTL.Seconds()),
	})
}

func (h *AuthHandler) Me(c *astra.Ctx) error {
	claims := sec.GetClaims(c)
	sub, _ := claims.GetSubject()
	var userID int64
	fmt.Sscanf(sub, "%d", &userID)
	u, ok := h.store.Find(userID)
	if !ok {
		return astra.NewHTTPError(http.StatusNotFound, "user not found")
	}
	return c.JSON(http.StatusOK, astra.Map{
		"id":    u.ID,
		"name":  u.Name,
		"email": u.Email,
	})
}

// ─── Main ──────────────────────────────────────────────────────────────────────

func main() {
	app := astra.New(astra.WithShutdownTimeout(10))
	app.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.CORSPermissive(),
	)

	store := NewUserStore()
	// seed a demo account
	store.Register("Demo User", "demo@example.com", "password123")

	h := &AuthHandler{store: store}

	auth := app.Group("/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.Refresh)

	api := app.Group("/api")
	api.Use(sec.JWT(accessSecret))
	api.GET("/me", h.Me)

	fmt.Println("JWT server :8080")
	fmt.Println("  POST /auth/register   { name, email, password }")
	fmt.Println("  POST /auth/login      { email, password }")
	fmt.Println("  POST /auth/refresh    { refresh_token }")
	fmt.Println("  GET  /api/me          (Bearer <access_token>)")
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
