package handler

import (
	"net/http"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=100"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *astra.Ctx) error {
	var req registerRequest
	if err := c.BindJSON(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	result, err := h.authService.Register(c.Request().Context(), service.RegisterRequest{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"token": result.Token,
		"user":  result.User,
	})
}

func (h *AuthHandler) Login(c *astra.Ctx) error {
	var req loginRequest
	if err := c.BindJSON(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	result, err := h.authService.Login(c.Request().Context(), service.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"token": result.Token,
		"user":  result.User,
	})
}
