// Package service contains the business logic for the Showcase application.
// Services depend on repositories (via interfaces) and are unaware of HTTP.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	astraorm "github.com/astra-go/astra/orm"
	"gorm.io/gorm"
)

// ─── Errors ───────────────────────────────────────────────────────────────────

var (
	ErrNotFound      = astra.NewHTTPError(http.StatusNotFound, "record not found")
	ErrConflict      = astra.NewHTTPError(http.StatusConflict, "conflict")
	ErrInsufficientStock = astra.NewHTTPError(http.StatusConflict, "insufficient stock")
	ErrForbidden     = astra.NewHTTPError(http.StatusForbidden, "access denied")
)

func mapGORMErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// ─── ProductSvc ───────────────────────────────────────────────────────────────

type productRepo interface {
	Create(ctx context.Context, p *domain.Product) error
	FindByID(ctx context.Context, id uint) (*domain.Product, error)
	FindAll(ctx context.Context, p *astraorm.Page) ([]domain.Product, int64, error)
	Updates(ctx context.Context, id uint, values any) error
	Delete(ctx context.Context, id uint) error
	DecrStock(ctx context.Context, productID uint, qty int) error
}

type ProductSvc struct {
	repo productRepo
}

func NewProductSvc(repo productRepo) *ProductSvc {
	return &ProductSvc{repo: repo}
}

func (s *ProductSvc) Create(ctx context.Context, tenantID uint, req domain.CreateProductReq) (*domain.Product, error) {
	p := &domain.Product{
		TenantID: tenantID,
		Name:     req.Name,
		Price:    req.Price,
		Stock:    req.Stock,
		Category: req.Category,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("product create: %w", err)
	}
	return p, nil
}

func (s *ProductSvc) Get(ctx context.Context, id uint) (*domain.Product, error) {
	p, err := s.repo.FindByID(ctx, id)
	return p, mapGORMErr(err)
}

func (s *ProductSvc) List(ctx context.Context, page astraorm.Page) ([]domain.Product, int64, error) {
	return s.repo.FindAll(ctx, &page)
}

func (s *ProductSvc) Update(ctx context.Context, id uint, req domain.UpdateProductReq) (*domain.Product, error) {
	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.Stock != nil {
		updates["stock"] = *req.Stock
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if len(updates) == 0 {
		return s.Get(ctx, id)
	}
	if err := s.repo.Updates(ctx, id, updates); err != nil {
		return nil, mapGORMErr(err)
	}
	return s.Get(ctx, id)
}

func (s *ProductSvc) Delete(ctx context.Context, id uint) error {
	return mapGORMErr(s.repo.Delete(ctx, id))
}
