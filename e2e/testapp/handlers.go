package testapp

import (
	"net/http"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/middleware"
	"github.com/golang-jwt/jwt/v5"
)

type registerReq struct {
	Username string `json:"username" validate:"required,min=3,max=20"`
	Password string `json:"password" validate:"required,min=6"`
}

type loginReq struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

func registerHandler(store *UserStore) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		var req registerReq
		if err := c.ShouldBind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"code": 400, "message": err.Error()})
		}

		u, err := store.Register(req.Username, req.Password)
		if err != nil {
			if err == ErrUsernameTaken {
				return c.JSON(http.StatusConflict, map[string]any{"code": 409, "message": "username already taken"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]any{"code": 500, "message": "internal error"})
		}

		return c.JSON(http.StatusOK, map[string]any{"id": u.ID, "username": u.Username})
	}
}

func loginHandler(store *UserStore, secret string) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		var req loginReq
		if err := c.ShouldBind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"code": 400, "message": err.Error()})
		}

		u, err := store.Login(req.Username, req.Password)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]any{"code": 401, "message": "invalid credentials"})
		}

		exp := time.Now().Add(time.Hour)
		token, err := middleware.GenerateJWT(jwt.MapClaims{
			"sub": u.ID,
			"exp": exp.Unix(),
		}, secret)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"code": 500, "message": "token generation failed"})
		}

		return c.JSON(http.StatusOK, map[string]any{"token": token, "expires_at": exp.Unix()})
	}
}

func meHandler(store *UserStore) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		claims := middleware.GetClaims(c)
		if claims == nil {
			return c.JSON(http.StatusUnauthorized, map[string]any{"code": 401, "message": "unauthorized"})
		}

		userID := claims.Subject
		u, err := store.GetByID(userID)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]any{"code": 404, "message": "user not found"})
		}

		return c.JSON(http.StatusOK, map[string]any{"id": u.ID, "username": u.Username})
	}
}
