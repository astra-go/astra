// Package handler contains the HTTP handlers for the Showcase application.
// Handlers are thin: they bind/validate input, call a service, and render output.
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/service"
	"github.com/astra-go/astra/middleware/security"
	astraorm "github.com/astra-go/astra/orm"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseUintParam(c *astra.Ctx, name string) (uint, error) {
	n, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		return 0, astra.NewHTTPError(http.StatusBadRequest, "invalid "+name)
	}
	return uint(n), nil
}

// claimsUserID extracts user_id from the JWT claims set by security.JWT middleware.
func claimsUserID(c *astra.Ctx) uint {
	claims := security.GetClaims(c)
	if claims == nil {
		return 0
	}
	if v, ok := claims.Extra["user_id"]; ok {
		switch id := v.(type) {
		case float64:
			return uint(id)
		case uint:
			return id
		}
	}
	return 0
}

// claimsTenantID extracts tenant_id from JWT claims.
func claimsTenantID(c *astra.Ctx) uint {
	claims := security.GetClaims(c)
	if claims == nil {
		return 0
	}
	if v, ok := claims.Extra["tenant_id"]; ok {
		if id, ok := v.(float64); ok {
			return uint(id)
		}
	}
	return 0
}

// ─── ProductHandler ───────────────────────────────────────────────────────────

type productSvc interface {
	Create(ctx context.Context, tenantID uint, req domain.CreateProductReq) (*domain.Product, error)
	Get(ctx context.Context, id uint) (*domain.Product, error)
	List(ctx context.Context, page astraorm.Page) ([]domain.Product, int64, error)
	Update(ctx context.Context, id uint, req domain.UpdateProductReq) (*domain.Product, error)
	Delete(ctx context.Context, id uint) error
}

type ProductHandler struct{ svc productSvc }

func NewProductHandler(svc productSvc) *ProductHandler { return &ProductHandler{svc: svc} }

func (h *ProductHandler) List(c *astra.Ctx) error {
	page := astraorm.ParsePage(c)
	products, total, err := h.svc.List(c.Request().Context(), page)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astraorm.NewPageResponse(page, total, products))
}

func (h *ProductHandler) Create(c *astra.Ctx) error {
	var req domain.CreateProductReq
	if err := c.ShouldBind(&req); err != nil {
		return err
	}
	p, err := h.svc.Create(c.Request().Context(), claimsTenantID(c), req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, astra.Map{"data": p})
}

func (h *ProductHandler) Get(c *astra.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	p, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"data": p})
}

func (h *ProductHandler) Update(c *astra.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	var req domain.UpdateProductReq
	if err := c.ShouldBind(&req); err != nil {
		return err
	}
	p, err := h.svc.Update(c.Request().Context(), id, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"data": p})
}

func (h *ProductHandler) Delete(c *astra.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ─── OrderHandler ─────────────────────────────────────────────────────────────

type orderSvc interface {
	Create(ctx context.Context, tenantID, userID uint, req domain.CreateOrderReq) (*domain.Order, error)
	Get(ctx context.Context, orderID uint) (*domain.Order, error)
	ListByUser(ctx context.Context, userID uint, page astraorm.Page) ([]domain.Order, int64, error)
}

type OrderHandler struct{ svc orderSvc }

func NewOrderHandler(svc orderSvc) *OrderHandler { return &OrderHandler{svc: svc} }

func (h *OrderHandler) Create(c *astra.Ctx) error {
	var req domain.CreateOrderReq
	if err := c.ShouldBind(&req); err != nil {
		return err
	}

	// Canary: v2 checkout applies a 5 % loyalty discount on the total.
	// The canary_version key is set by middleware.Canary (runs after JWT).
	if v, ok := c.Get("canary_version"); ok && v == "v2" {
		req.CanaryVersion = "v2"
	}

	order, err := h.svc.Create(c.Request().Context(), claimsTenantID(c), claimsUserID(c), req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, astra.Map{"data": order})
}

func (h *OrderHandler) Get(c *astra.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	order, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"data": order})
}

func (h *OrderHandler) List(c *astra.Ctx) error {
	page := astraorm.ParsePage(c)
	orders, total, err := h.svc.ListByUser(c.Request().Context(), claimsUserID(c), page)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astraorm.NewPageResponse(page, total, orders))
}

// ─── AdminHandler ─────────────────────────────────────────────────────────────

type userSvc interface {
	Get(ctx context.Context, id uint) (*domain.User, error)
	UpdateRole(ctx context.Context, userID uint, role domain.Role) error
}

type AdminHandler struct{ svc userSvc }

func NewAdminHandler(svc userSvc) *AdminHandler { return &AdminHandler{svc: svc} }

func (h *AdminHandler) GetUser(c *astra.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	u, err := h.svc.Get(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"data": u})
}

func (h *AdminHandler) UpdateRole(c *astra.Ctx) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	var req domain.UpdateRoleReq
	if err := c.ShouldBind(&req); err != nil {
		return err
	}
	if err := h.svc.UpdateRole(c.Request().Context(), id, req.Role); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
}

// ─── HealthHandler ────────────────────────────────────────────────────────────

func HealthHandler(c *astra.Ctx) error {
	return c.JSON(http.StatusOK, astra.Map{"status": "ok"})
}

// ─── RBAC helper ──────────────────────────────────────────────────────────────

// RequireRole returns a middleware that rejects requests whose JWT role claim
// is not in the allowed set.
func RequireRole(roles ...domain.Role) astra.MiddlewareFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[string(r)] = true
	}
	return func(c *astra.Ctx) error {
		claims := security.GetClaims(c)
		if claims == nil {
			return service.ErrForbidden
		}
		role, _ := claims.Extra["role"].(string)
		if !allowed[role] {
			return service.ErrForbidden
		}
		c.Next()
		return nil
	}
}
