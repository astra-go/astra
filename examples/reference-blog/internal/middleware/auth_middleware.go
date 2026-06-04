package middleware

import (
	"net/http"
	"strings"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
)

type AuthMiddleware struct {
	authService *service.AuthService
}

func NewAuthMiddleware(authService *service.AuthService) *AuthMiddleware {
	return &AuthMiddleware{authService: authService}
}

func (m *AuthMiddleware) Authenticate(next astra.HandlerFunc) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		auth := c.Request().Header.Get("Authorization")
		if auth == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid authorization header"})
		}

		claims, err := m.authService.ValidateToken(parts[1])
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		return next(c)
	}
}

func RequireRole(role string) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		val, ok := c.Get("role")
		if !ok {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
		}
		userRole, ok := val.(string)
		if !ok || userRole != role {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
		}
		return c.Next()
	}
}
