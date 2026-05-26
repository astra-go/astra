package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
