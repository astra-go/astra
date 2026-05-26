package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	astraorm "github.com/astra-go/astra/orm"
)

// userRepo is the minimal interface UserSvc needs.
type userRepo interface {
	Create(ctx context.Context, u *domain.User) error
	FindByID(ctx context.Context, id uint) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByOAuth(ctx context.Context, provider, sub string) (*domain.User, error)
	Updates(ctx context.Context, id uint, values any) error
	FindAll(ctx context.Context, p *astraorm.Page) ([]domain.User, int64, error)
}

// UserSvc handles user management and JWT issuance.
type UserSvc struct {
	repo      userRepo
	jwtSecret []byte
	jwtTTL    time.Duration
}

func NewUserSvc(repo userRepo, jwtSecret string, jwtTTL time.Duration) *UserSvc {
	return &UserSvc{
		repo:      repo,
		jwtSecret: []byte(jwtSecret),
		jwtTTL:    jwtTTL,
	}
}

// IssueToken creates a signed JWT for the given user.
func (s *UserSvc) IssueToken(u *domain.User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":       fmt.Sprintf("%d", u.ID),
		"user_id":   u.ID,
		"tenant_id": u.TenantID,
		"role":      string(u.Role),
		"email":     u.Email,
		"iat":       now.Unix(),
		"exp":       now.Add(s.jwtTTL).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.jwtSecret)
}

// UpsertOAuth finds or creates a user identified by the OAuth provider + subject.
// On first login the user is created with the buyer role.
func (s *UserSvc) UpsertOAuth(ctx context.Context, tenantID uint, provider, sub, email, name string) (*domain.User, error) {
	u, err := s.repo.FindByOAuth(ctx, provider, sub)
	if err == nil {
		return u, nil
	}

	// New user — create with buyer role.
	u = &domain.User{
		TenantID:      tenantID,
		Email:         email,
		Name:          name,
		Role:          domain.RoleBuyer,
		OAuthProvider: provider,
		OAuthSub:      sub,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("user upsert: %w", err)
	}
	return u, nil
}

// Get returns a user by ID.
func (s *UserSvc) Get(ctx context.Context, id uint) (*domain.User, error) {
	u, err := s.repo.FindByID(ctx, id)
	return u, mapGORMErr(err)
}

// UpdateRole changes a user's role. Only admins should call this endpoint.
func (s *UserSvc) UpdateRole(ctx context.Context, userID uint, role domain.Role) error {
	switch role {
	case domain.RoleAdmin, domain.RoleSeller, domain.RoleBuyer:
	default:
		return astra.NewHTTPError(http.StatusBadRequest, "invalid role")
	}
	return mapGORMErr(s.repo.Updates(ctx, userID, map[string]any{"role": string(role)}))
}
