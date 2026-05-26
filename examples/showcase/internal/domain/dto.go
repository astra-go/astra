// Package domain — request/response DTOs used by the HTTP layer.
// Kept separate from entities to avoid coupling transport concerns to the DB model.
package domain

// ─── Auth ─────────────────────────────────────────────────────────────────────

type LoginResp struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"` // seconds
}

// ─── Product ──────────────────────────────────────────────────────────────────

type CreateProductReq struct {
	Name     string  `json:"name"     validate:"required,max=255"`
	Price    float64 `json:"price"    validate:"gte=0"`
	Stock    int     `json:"stock"    validate:"gte=0"`
	Category string  `json:"category" validate:"max=100"`
}

type UpdateProductReq struct {
	Name     string  `json:"name,omitempty"`
	Price    *float64 `json:"price,omitempty"`
	Stock    *int    `json:"stock,omitempty"`
	Category string  `json:"category,omitempty"`
}

// ─── Order ────────────────────────────────────────────────────────────────────

type CreateOrderReq struct {
	Items []OrderItemReq `json:"items" validate:"required,min=1,dive"`
	// CanaryVersion is populated by the handler, not the JSON body.
	// "v2" activates the 5 % loyalty discount in OrderSvc.
	CanaryVersion string `json:"-"`
}

type OrderItemReq struct {
	ProductID uint `json:"product_id" validate:"required"`
	Qty       int  `json:"qty"        validate:"required,min=1"`
}

// ─── User (admin) ─────────────────────────────────────────────────────────────

type UpdateRoleReq struct {
	Role Role `json:"role" validate:"required,oneof=admin seller buyer"`
}
