// Package domain defines the core business entities for the Showcase application.
// All entities embed orm.Model (UUID primary key + timestamps) and carry a
// TenantID field for row-level multi-tenant isolation.
package domain

import (
	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/timeutil"
)

// ─── Tenant ───────────────────────────────────────────────────────────────────

// Plan represents the subscription tier of a tenant.
type Plan string

const (
	PlanFree    Plan = "free"
	PlanPro     Plan = "pro"
	PlanEnterprise Plan = "enterprise"
)

// Tenant is the top-level isolation boundary. Every other entity belongs to
// exactly one tenant.
type Tenant struct {
	orm.Model
	Name string `json:"name" gorm:"not null;uniqueIndex"`
	Plan Plan   `json:"plan" gorm:"not null;default:'free'"`
}

// ─── User ─────────────────────────────────────────────────────────────────────

// Role controls what a user can do within their tenant.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleSeller Role = "seller"
	RoleBuyer  Role = "buyer"
)

// User represents an authenticated principal. OAuth2 users are identified by
// (OAuthProvider, OAuthSub); password users have a hashed password instead.
type User struct {
	orm.Model
	TenantID      uint   `json:"tenant_id"      gorm:"not null;index"`
	Email         string `json:"email"          gorm:"not null;uniqueIndex"`
	Name          string `json:"name"           gorm:"not null"`
	Role          Role   `json:"role"           gorm:"not null;default:'buyer'"`
	OAuthProvider string `json:"oauth_provider" gorm:"size:32"`
	OAuthSub      string `json:"oauth_sub"      gorm:"size:255;index"`
	// PasswordHash is empty for OAuth-only users.
	PasswordHash string `json:"-" gorm:"size:255"`

	Tenant Tenant `json:"-" gorm:"foreignKey:TenantID"`
}

// ─── Product ──────────────────────────────────────────────────────────────────

// Product is a sellable item within a tenant's catalogue.
type Product struct {
	orm.SoftDeleteModel
	TenantID uint    `json:"tenant_id" gorm:"not null;index"`
	Name     string  `json:"name"      gorm:"not null"`
	Price    float64 `json:"price"     gorm:"not null;check:price >= 0"`
	Stock    int     `json:"stock"     gorm:"not null;default:0;check:stock >= 0"`
	Category string  `json:"category"  gorm:"size:100;index"`

	Tenant Tenant `json:"-" gorm:"foreignKey:TenantID"`
}

// ─── Order ────────────────────────────────────────────────────────────────────

// OrderStatus tracks the lifecycle of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusCompleted OrderStatus = "completed"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// Order is a purchase made by a buyer.
type Order struct {
	orm.Model
	TenantID uint        `json:"tenant_id" gorm:"not null;index"`
	UserID   uint        `json:"user_id"   gorm:"not null;index"`
	Total    float64     `json:"total"     gorm:"not null"`
	Status   OrderStatus `json:"status"    gorm:"not null;default:'pending';index"`

	Tenant Tenant      `json:"-"     gorm:"foreignKey:TenantID"`
	User   User        `json:"user"  gorm:"foreignKey:UserID"`
	Items  []OrderItem `json:"items" gorm:"foreignKey:OrderID"`
}

// OrderItem is a single line in an order.
type OrderItem struct {
	orm.Model
	OrderID   uint    `json:"order_id"   gorm:"not null;index"`
	ProductID uint    `json:"product_id" gorm:"not null;index"`
	Qty       int     `json:"qty"        gorm:"not null;check:qty > 0"`
	Price     float64 `json:"price"      gorm:"not null"` // snapshot at purchase time

	Product Product `json:"product" gorm:"foreignKey:ProductID"`
}

// ─── AuditLog ─────────────────────────────────────────────────────────────────

// AuditLog records sensitive mutations for compliance.
type AuditLog struct {
	ID        uint          `json:"id"         gorm:"primaryKey;autoIncrement"`
	TenantID  uint          `json:"tenant_id"  gorm:"not null;index"`
	UserID    uint          `json:"user_id"    gorm:"not null;index"`
	Action    string        `json:"action"     gorm:"not null;size:64"`
	Resource  string        `json:"resource"   gorm:"not null;size:64"`
	ResourceID uint         `json:"resource_id"`
	Detail    string        `json:"detail"     gorm:"type:text"`
	CreatedAt timeutil.Time `json:"created_at"`
}
