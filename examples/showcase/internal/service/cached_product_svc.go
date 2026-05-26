package service

import (
	"context"
	"fmt"
	"time"

	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	astraorm "github.com/astra-go/astra/orm"
)

const productCacheTTL = 5 * time.Minute

// CachedProductSvc wraps ProductSvc with a read-through cache layer.
// Cache keys are scoped per tenant to prevent cross-tenant cache poisoning.
type CachedProductSvc struct {
	inner    *ProductSvc
	cache    cache.Cache
	tenantID uint
}

func NewCachedProductSvc(inner *ProductSvc, c cache.Cache, tenantID uint) *CachedProductSvc {
	return &CachedProductSvc{inner: inner, cache: c, tenantID: tenantID}
}

func (s *CachedProductSvc) Create(ctx context.Context, tenantID uint, req domain.CreateProductReq) (*domain.Product, error) {
	p, err := s.inner.Create(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Delete(ctx, listKey(tenantID))
	return p, nil
}

func (s *CachedProductSvc) Get(ctx context.Context, id uint) (*domain.Product, error) {
	var p domain.Product
	err := cache.GetOrSet(ctx, s.cache, productKey(id), &p, productCacheTTL, func() (any, error) {
		return s.inner.Get(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *CachedProductSvc) List(ctx context.Context, page astraorm.Page) ([]domain.Product, int64, error) {
	// Only cache page 1 (most common); other pages go straight to DB.
	if page.PageNum != 1 {
		return s.inner.List(ctx, page)
	}
	type listResult struct {
		Items []domain.Product `json:"items"`
		Total int64            `json:"total"`
	}
	var result listResult
	err := cache.GetOrSet(ctx, s.cache, listKey(s.tenantID), &result, productCacheTTL, func() (any, error) {
		items, total, err := s.inner.List(ctx, page)
		if err != nil {
			return nil, err
		}
		return &listResult{Items: items, Total: total}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	return result.Items, result.Total, nil
}

func (s *CachedProductSvc) Update(ctx context.Context, id uint, req domain.UpdateProductReq) (*domain.Product, error) {
	p, err := s.inner.Update(ctx, id, req)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Delete(ctx, productKey(id), listKey(s.tenantID))
	return p, nil
}

func (s *CachedProductSvc) Delete(ctx context.Context, id uint) error {
	if err := s.inner.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.cache.Delete(ctx, productKey(id), listKey(s.tenantID))
	return nil
}

// ─── key helpers ──────────────────────────────────────────────────────────────

func productKey(id uint) string {
	return fmt.Sprintf("showcase:product:%d", id)
}

func listKey(tenantID uint) string {
	return fmt.Sprintf("showcase:products:list:%d", tenantID)
}
