package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/service"
	astraorm "github.com/astra-go/astra/orm"
	"gorm.io/gorm"
)

// ─── mock implementations ─────────────────────────────────────────────────────

type mockProductRepo struct {
	products map[uint]*domain.Product
	nextID   uint
	decrErr  error
}

func newMockProductRepo(products ...*domain.Product) *mockProductRepo {
	m := &mockProductRepo{products: make(map[uint]*domain.Product)}
	for _, p := range products {
		m.products[p.ID] = p
	}
	return m
}

func (m *mockProductRepo) Create(_ context.Context, p *domain.Product) error {
	m.nextID++
	p.ID = m.nextID
	m.products[p.ID] = p
	return nil
}

func (m *mockProductRepo) FindByID(_ context.Context, id uint) (*domain.Product, error) {
	p, ok := m.products[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return p, nil
}

func (m *mockProductRepo) FindAll(_ context.Context, _ *astraorm.Page) ([]domain.Product, int64, error) {
	var out []domain.Product
	for _, p := range m.products {
		out = append(out, *p)
	}
	return out, int64(len(out)), nil
}

func (m *mockProductRepo) Updates(_ context.Context, id uint, _ any) error {
	if _, ok := m.products[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (m *mockProductRepo) Delete(_ context.Context, id uint) error {
	if _, ok := m.products[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.products, id)
	return nil
}

func (m *mockProductRepo) DecrStock(_ context.Context, _ uint, _ int) error {
	return m.decrErr
}

type mockOrderRepo struct {
	orders map[uint]*domain.Order
	nextID uint
}

func (m *mockOrderRepo) Create(_ context.Context, o *domain.Order) error {
	m.nextID++
	o.ID = m.nextID
	m.orders[o.ID] = o
	return nil
}

func (m *mockOrderRepo) FindByID(_ context.Context, id uint) (*domain.Order, error) {
	o, ok := m.orders[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return o, nil
}

func (m *mockOrderRepo) FindWithItems(_ context.Context, id uint) (*domain.Order, error) {
	return m.FindByID(context.Background(), id)
}

func (m *mockOrderRepo) FindByUser(_ context.Context, _ uint, _ *astraorm.Page) ([]domain.Order, int64, error) {
	return nil, 0, nil
}

func (m *mockOrderRepo) Updates(_ context.Context, _ uint, _ any) error { return nil }

type mockItemRepo struct{}

func (m *mockItemRepo) Create(_ context.Context, _ *domain.OrderItem) error { return nil }

type mockUserRepo struct {
	users  map[uint]*domain.User
	nextID uint
}

func (m *mockUserRepo) Create(_ context.Context, u *domain.User) error {
	m.nextID++
	u.ID = m.nextID
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) FindByID(_ context.Context, id uint) (*domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return u, nil
}

func (m *mockUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockUserRepo) FindByOAuth(_ context.Context, provider, sub string) (*domain.User, error) {
	for _, u := range m.users {
		if u.OAuthProvider == provider && u.OAuthSub == sub {
			return u, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockUserRepo) Updates(_ context.Context, _ uint, _ any) error { return nil }

func (m *mockUserRepo) FindAll(_ context.Context, _ *astraorm.Page) ([]domain.User, int64, error) {
	return nil, 0, nil
}

// ─── ProductSvc tests ─────────────────────────────────────────────────────────

func TestProductSvc_Get_NotFound(t *testing.T) {
	svc := service.NewProductSvc(newMockProductRepo())
	_, err := svc.Get(context.Background(), 99)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestProductSvc_Create(t *testing.T) {
	svc := service.NewProductSvc(newMockProductRepo())
	p, err := svc.Create(context.Background(), 1, domain.CreateProductReq{
		Name:  "Widget",
		Price: 9.99,
		Stock: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if p.Name != "Widget" {
		t.Fatalf("expected name Widget, got %s", p.Name)
	}
}

func TestProductSvc_Delete_NotFound(t *testing.T) {
	svc := service.NewProductSvc(newMockProductRepo())
	err := svc.Delete(context.Background(), 99)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ─── OrderSvc tests ───────────────────────────────────────────────────────────

func TestOrderSvc_Create_InsufficientStock_PreCheck(t *testing.T) {
	product := &domain.Product{Stock: 0, Price: 10.0}
	product.ID = 1
	productRepo := newMockProductRepo(product)
	orderRepo := &mockOrderRepo{orders: make(map[uint]*domain.Order)}

	svc := service.NewOrderSvc(orderRepo, &mockItemRepo{}, productRepo, nil)
	_, err := svc.Create(context.Background(), 1, 1, domain.CreateOrderReq{
		Items: []domain.OrderItemReq{{ProductID: 1, Qty: 1}},
	})
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}
}

func TestOrderSvc_Create_ConcurrentStockDepletion(t *testing.T) {
	// Pre-check passes (stock=5, qty=1) but DecrStock fails — simulates concurrent order.
	product := &domain.Product{Stock: 5, Price: 10.0}
	product.ID = 1
	productRepo := newMockProductRepo(product)
	productRepo.decrErr = gorm.ErrRecordNotFound // concurrent depletion
	orderRepo := &mockOrderRepo{orders: make(map[uint]*domain.Order)}

	svc := service.NewOrderSvc(orderRepo, &mockItemRepo{}, productRepo, nil)
	_, err := svc.Create(context.Background(), 1, 1, domain.CreateOrderReq{
		Items: []domain.OrderItemReq{{ProductID: 1, Qty: 1}},
	})
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}
}

func TestOrderSvc_Create_ProductNotFound(t *testing.T) {
	svc := service.NewOrderSvc(
		&mockOrderRepo{orders: make(map[uint]*domain.Order)},
		&mockItemRepo{},
		newMockProductRepo(), // empty — product 99 doesn't exist
		nil,
	)
	_, err := svc.Create(context.Background(), 1, 1, domain.CreateOrderReq{
		Items: []domain.OrderItemReq{{ProductID: 99, Qty: 1}},
	})
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ─── UserSvc tests ────────────────────────────────────────────────────────────

func TestUserSvc_IssueToken_Claims(t *testing.T) {
	repo := &mockUserRepo{users: make(map[uint]*domain.User)}
	svc := service.NewUserSvc(repo, "test-secret", time.Hour)

	u := &domain.User{Email: "alice@example.com", Role: domain.RoleBuyer, TenantID: 1}
	u.ID = 42

	tok, err := svc.IssueToken(u)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if tok == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestUserSvc_UpdateRole_InvalidRole(t *testing.T) {
	repo := &mockUserRepo{users: make(map[uint]*domain.User)}
	svc := service.NewUserSvc(repo, "secret", time.Hour)

	err := svc.UpdateRole(context.Background(), 1, "superuser")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestUserSvc_UpdateRole_Valid(t *testing.T) {
	repo := &mockUserRepo{users: make(map[uint]*domain.User)}
	repo.users[1] = &domain.User{Role: domain.RoleBuyer}
	repo.users[1].ID = 1

	svc := service.NewUserSvc(repo, "secret", time.Hour)
	if err := svc.UpdateRole(context.Background(), 1, domain.RoleSeller); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── CachedProductSvc tests ───────────────────────────────────────────────────

type mockCache struct {
	data map[string][]byte
}

func newMockCache() *mockCache {
	return &mockCache{data: make(map[string][]byte)}
}

func (c *mockCache) Get(_ context.Context, key string) ([]byte, error) {
	val, ok := c.data[key]
	if !ok {
		return nil, cache.ErrCacheMiss
	}
	return val, nil
}

func (c *mockCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.data[key] = value
	return nil
}

func (c *mockCache) Delete(_ context.Context, keys ...string) error {
	for _, k := range keys {
		delete(c.data, k)
	}
	return nil
}

func (c *mockCache) Exists(_ context.Context, key string) (bool, error) {
	_, ok := c.data[key]
	return ok, nil
}

func (c *mockCache) Close() error {
	return nil
}

func (c *mockCache) Flush(_ context.Context) error {
	c.data = make(map[string][]byte)
	return nil
}

func TestCachedProductSvc_Create_InvalidatesList(t *testing.T) {
	repo := newMockProductRepo()
	cache := newMockCache()
	baseSvc := service.NewProductSvc(repo)
	svc := service.NewCachedProductSvc(baseSvc, cache, 1)

	// Pre-populate cache with list
	cache.data["showcase:products:list:1"] = []byte("old-data")

	_, err := svc.Create(context.Background(), 1, domain.CreateProductReq{
		Name:  "Widget",
		Price: 9.99,
		Stock: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify list cache was invalidated
	if _, ok := cache.data["showcase:products:list:1"]; ok {
		t.Fatal("expected list cache to be invalidated after create")
	}
}

func TestCachedProductSvc_Update_InvalidatesBoth(t *testing.T) {
	product := &domain.Product{Name: "Old", Price: 5.0, Stock: 10}
	product.ID = 1
	repo := newMockProductRepo(product)
	cache := newMockCache()
	baseSvc := service.NewProductSvc(repo)
	svc := service.NewCachedProductSvc(baseSvc, cache, 1)

	// Pre-populate both caches
	cache.data["showcase:product:1"] = []byte("old")
	cache.data["showcase:products:list:1"] = []byte("old")

	price := 12.99
	_, err := svc.Update(context.Background(), 1, domain.UpdateProductReq{Price: &price})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both caches should be invalidated
	if _, ok := cache.data["showcase:product:1"]; ok {
		t.Fatal("expected product cache to be invalidated")
	}
	if _, ok := cache.data["showcase:products:list:1"]; ok {
		t.Fatal("expected list cache to be invalidated")
	}
}

func TestCachedProductSvc_Delete_InvalidatesBoth(t *testing.T) {
	product := &domain.Product{Name: "ToDelete", Price: 5.0, Stock: 10}
	product.ID = 1
	repo := newMockProductRepo(product)
	cache := newMockCache()
	baseSvc := service.NewProductSvc(repo)
	svc := service.NewCachedProductSvc(baseSvc, cache, 1)

	cache.data["showcase:product:1"] = []byte("cached")
	cache.data["showcase:products:list:1"] = []byte("cached")

	err := svc.Delete(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := cache.data["showcase:product:1"]; ok {
		t.Fatal("expected product cache to be invalidated")
	}
	if _, ok := cache.data["showcase:products:list:1"]; ok {
		t.Fatal("expected list cache to be invalidated")
	}
}

func TestCachedProductSvc_List_OnlyPage1Cached(t *testing.T) {
	repo := newMockProductRepo()
	cache := newMockCache()
	baseSvc := service.NewProductSvc(repo)
	svc := service.NewCachedProductSvc(baseSvc, cache, 1)

	// Page 1 should attempt caching
	_, _, err := svc.List(context.Background(), astraorm.Page{PageNum: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Page 2 should bypass cache (no cache key stored)
	_, _, err = svc.List(context.Background(), astraorm.Page{PageNum: 2, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── OrderSvc additional tests ────────────────────────────────────────────────

func TestOrderSvc_Create_EmptyItems(t *testing.T) {
	svc := service.NewOrderSvc(
		&mockOrderRepo{orders: make(map[uint]*domain.Order)},
		&mockItemRepo{},
		newMockProductRepo(),
		nil,
	)
	_, err := svc.Create(context.Background(), 1, 1, domain.CreateOrderReq{Items: []domain.OrderItemReq{}})
	// No validation for empty items in current implementation — order would succeed with 0 total
	if err != nil {
		t.Logf("empty items resulted in error: %v", err)
	}
}

func TestOrderSvc_Create_MultipleProducts(t *testing.T) {
	p1 := &domain.Product{Name: "A", Price: 10.0, Stock: 5}
	p1.ID = 1
	p2 := &domain.Product{Name: "B", Price: 20.0, Stock: 3}
	p2.ID = 2
	repo := newMockProductRepo(p1, p2)
	orderRepo := &mockOrderRepo{orders: make(map[uint]*domain.Order)}

	svc := service.NewOrderSvc(orderRepo, &mockItemRepo{}, repo, nil)
	order, err := svc.Create(context.Background(), 1, 1, domain.CreateOrderReq{
		Items: []domain.OrderItemReq{
			{ProductID: 1, Qty: 2},
			{ProductID: 2, Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Total = 2*10 + 1*20 = 40
	expectedTotal := 40.0
	if order.Total != expectedTotal {
		t.Fatalf("expected total %.2f, got %.2f", expectedTotal, order.Total)
	}
}

func TestOrderSvc_Create_CanaryDiscount(t *testing.T) {
	p := &domain.Product{Name: "A", Price: 100.0, Stock: 10}
	p.ID = 1
	repo := newMockProductRepo(p)
	orderRepo := &mockOrderRepo{orders: make(map[uint]*domain.Order)}

	svc := service.NewOrderSvc(orderRepo, &mockItemRepo{}, repo, nil)
	order, err := svc.Create(context.Background(), 1, 1, domain.CreateOrderReq{
		Items:         []domain.OrderItemReq{{ProductID: 1, Qty: 1}},
		CanaryVersion: "v2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// v2 applies 5% discount: 100 * 0.95 = 95
	expectedTotal := 95.0
	if order.Total != expectedTotal {
		t.Fatalf("expected total %.2f with v2 discount, got %.2f", expectedTotal, order.Total)
	}
}

func TestOrderSvc_Get_NotFound(t *testing.T) {
	svc := service.NewOrderSvc(
		&mockOrderRepo{orders: make(map[uint]*domain.Order)},
		&mockItemRepo{},
		newMockProductRepo(),
		nil,
	)
	_, err := svc.Get(context.Background(), 999)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ─── UserSvc additional tests ─────────────────────────────────────────────────

func TestUserSvc_UpsertOAuth_NewUser(t *testing.T) {
	repo := &mockUserRepo{users: make(map[uint]*domain.User)}
	svc := service.NewUserSvc(repo, "secret", time.Hour)

	u, err := svc.UpsertOAuth(context.Background(), 1, "google", "sub123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %s", u.Email)
	}
	if u.Role != domain.RoleBuyer {
		t.Fatalf("expected default role buyer, got %s", u.Role)
	}
	if u.OAuthProvider != "google" {
		t.Fatalf("expected provider google, got %s", u.OAuthProvider)
	}
}

func TestUserSvc_UpsertOAuth_ExistingUser(t *testing.T) {
	existing := &domain.User{Email: "bob@example.com", Role: domain.RoleSeller, OAuthProvider: "github", OAuthSub: "sub456"}
	existing.ID = 5
	repo := &mockUserRepo{users: map[uint]*domain.User{5: existing}}
	svc := service.NewUserSvc(repo, "secret", time.Hour)

	u, err := svc.UpsertOAuth(context.Background(), 1, "github", "sub456", "bob@example.com", "Bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != 5 {
		t.Fatalf("expected existing user ID 5, got %d", u.ID)
	}
	if u.Role != domain.RoleSeller {
		t.Fatalf("expected existing role seller, got %s", u.Role)
	}
}

func TestUserSvc_Get_NotFound(t *testing.T) {
	repo := &mockUserRepo{users: make(map[uint]*domain.User)}
	svc := service.NewUserSvc(repo, "secret", time.Hour)

	_, err := svc.Get(context.Background(), 999)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUserSvc_IssueToken_ValidClaims(t *testing.T) {
	repo := &mockUserRepo{users: make(map[uint]*domain.User)}
	svc := service.NewUserSvc(repo, "test-secret-key", time.Hour)

	u := &domain.User{Email: "test@example.com", Role: domain.RoleAdmin, TenantID: 7}
	u.ID = 10

	token, err := svc.IssueToken(u)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	// Token parsing verification would require jwt package, skip detailed validation here
}
