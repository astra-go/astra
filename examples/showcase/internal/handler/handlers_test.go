package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/handler"
	"github.com/astra-go/astra/examples/showcase/internal/service"
	astraorm "github.com/astra-go/astra/orm"
)

// ─── stub services ────────────────────────────────────────────────────────────

type stubProductSvc struct {
	product *domain.Product
	err     error
}

func (s *stubProductSvc) Create(_ context.Context, _ uint, req domain.CreateProductReq) (*domain.Product, error) {
	if s.err != nil {
		return nil, s.err
	}
	p := &domain.Product{Name: req.Name, Price: req.Price, Stock: req.Stock}
	p.ID = 1
	return p, nil
}

func (s *stubProductSvc) Get(_ context.Context, _ uint) (*domain.Product, error) {
	return s.product, s.err
}

func (s *stubProductSvc) List(_ context.Context, _ astraorm.Page) ([]domain.Product, int64, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	if s.product != nil {
		return []domain.Product{*s.product}, 1, nil
	}
	return []domain.Product{}, 0, nil
}

func (s *stubProductSvc) Update(_ context.Context, _ uint, _ domain.UpdateProductReq) (*domain.Product, error) {
	return s.product, s.err
}

func (s *stubProductSvc) Delete(_ context.Context, _ uint) error {
	return s.err
}

type stubOrderSvc struct {
	order *domain.Order
	err   error
}

func (s *stubOrderSvc) Create(_ context.Context, _, _ uint, _ domain.CreateOrderReq) (*domain.Order, error) {
	return s.order, s.err
}

func (s *stubOrderSvc) Get(_ context.Context, _ uint) (*domain.Order, error) {
	return s.order, s.err
}

func (s *stubOrderSvc) ListByUser(_ context.Context, _ uint, _ astraorm.Page) ([]domain.Order, int64, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	if s.order != nil {
		return []domain.Order{*s.order}, 1, nil
	}
	return []domain.Order{}, 0, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newApp(t *testing.T) *astra.App {
	t.Helper()
	return astra.New()
}

func do(app *astra.App, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}

// ─── ProductHandler tests ─────────────────────────────────────────────────────

func TestProductHandler_Get_OK(t *testing.T) {
	p := &domain.Product{Name: "Widget", Price: 9.99, Stock: 10}
	p.ID = 42

	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{product: p})
	app.GET("/products/:id", h.Get)

	w := do(app, http.MethodGet, "/products/42", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", resp)
	}
	if data["name"] != "Widget" {
		t.Fatalf("expected name Widget, got %v", data["name"])
	}
}

func TestProductHandler_Get_NotFound(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{err: service.ErrNotFound})
	app.GET("/products/:id", h.Get)

	w := do(app, http.MethodGet, "/products/99", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProductHandler_Get_BadID(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{})
	app.GET("/products/:id", h.Get)

	w := do(app, http.MethodGet, "/products/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestProductHandler_Create_OK(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{})
	app.POST("/products", h.Create)

	body := map[string]any{"name": "Gadget", "price": 19.99, "stock": 5}
	w := do(app, http.MethodPost, "/products", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProductHandler_Delete_OK(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{})
	app.DELETE("/products/:id", h.Delete)

	w := do(app, http.MethodDelete, "/products/1", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestProductHandler_List_OK(t *testing.T) {
	p := &domain.Product{Name: "Widget", Price: 9.99, Stock: 10}
	p.ID = 1

	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{product: p})
	app.GET("/products", h.List)

	w := do(app, http.MethodGet, "/products", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── OrderHandler tests ───────────────────────────────────────────────────────

func TestOrderHandler_Get_OK(t *testing.T) {
	o := &domain.Order{Total: 29.99, Status: domain.OrderStatusPending}
	o.ID = 7

	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{order: o})
	app.GET("/orders/:id", h.Get)

	w := do(app, http.MethodGet, "/orders/7", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOrderHandler_Get_NotFound(t *testing.T) {
	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{err: service.ErrNotFound})
	app.GET("/orders/:id", h.Get)

	w := do(app, http.MethodGet, "/orders/999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestOrderHandler_Create_OK(t *testing.T) {
	o := &domain.Order{Total: 10.0, Status: domain.OrderStatusPending}
	o.ID = 1

	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{order: o})
	app.POST("/orders", h.Create)

	body := map[string]any{
		"items": []map[string]any{{"product_id": 1, "qty": 1}},
	}
	w := do(app, http.MethodPost, "/orders", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOrderHandler_Create_InsufficientStock(t *testing.T) {
	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{err: service.ErrInsufficientStock})
	app.POST("/orders", h.Create)

	body := map[string]any{
		"items": []map[string]any{{"product_id": 1, "qty": 100}},
	}
	w := do(app, http.MethodPost, "/orders", body)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestOrderHandler_Create_CanaryDiscount(t *testing.T) {
	// Verify that the canary_version context key is forwarded to the service.
	var capturedReq domain.CreateOrderReq
	app := newApp(t)

	// Middleware that sets canary_version before the handler runs.
	app.Use(func(c *astra.Ctx) error {
		c.Set("canary_version", "v2")
		c.Next()
		return nil
	})

	svc := &captureOrderSvc{}
	h := handler.NewOrderHandler(svc)
	app.POST("/orders", h.Create)

	body := map[string]any{
		"items": []map[string]any{{"product_id": 1, "qty": 1}},
	}
	w := do(app, http.MethodPost, "/orders", body)
	_ = capturedReq
	// Service returns zero-value order (nil) → handler returns 201 with empty data or error.
	// We only care that the canary version was forwarded — check via svc.
	if svc.lastReq.CanaryVersion != "v2" {
		t.Fatalf("expected CanaryVersion v2, got %q (status %d)", svc.lastReq.CanaryVersion, w.Code)
	}
}

// captureOrderSvc records the last CreateOrderReq it received.
type captureOrderSvc struct {
	lastReq domain.CreateOrderReq
}

func (s *captureOrderSvc) Create(_ context.Context, _, _ uint, req domain.CreateOrderReq) (*domain.Order, error) {
	s.lastReq = req
	o := &domain.Order{Total: 9.5, Status: domain.OrderStatusPending}
	o.ID = 1
	return o, nil
}

func (s *captureOrderSvc) Get(_ context.Context, _ uint) (*domain.Order, error) {
	return nil, service.ErrNotFound
}

func (s *captureOrderSvc) ListByUser(_ context.Context, _ uint, _ astraorm.Page) ([]domain.Order, int64, error) {
	return nil, 0, nil
}

// ─── ProductHandler additional tests ──────────────────────────────────────────

func TestProductHandler_Update_OK(t *testing.T) {
	p := &domain.Product{Name: "Updated", Price: 15.99, Stock: 20}
	p.ID = 1

	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{product: p})
	app.PUT("/products/:id", h.Update)

	body := map[string]any{"name": "Updated", "price": 15.99}
	w := do(app, http.MethodPut, "/products/1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProductHandler_Update_NotFound(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{err: service.ErrNotFound})
	app.PUT("/products/:id", h.Update)

	body := map[string]any{"name": "Ghost"}
	w := do(app, http.MethodPut, "/products/999", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProductHandler_Update_BadID(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{})
	app.PUT("/products/:id", h.Update)

	w := do(app, http.MethodPut, "/products/invalid", map[string]any{"name": "Test"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestProductHandler_Delete_NotFound(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{err: service.ErrNotFound})
	app.DELETE("/products/:id", h.Delete)

	w := do(app, http.MethodDelete, "/products/999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProductHandler_Create_ValidationError(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{})
	app.POST("/products", h.Create)

	// Missing required fields
	body := map[string]any{"name": ""}
	w := do(app, http.MethodPost, "/products", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for validation error, got %d", w.Code)
	}
}

func TestProductHandler_List_EmptyResult(t *testing.T) {
	app := newApp(t)
	h := handler.NewProductHandler(&stubProductSvc{})
	app.GET("/products", h.List)

	w := do(app, http.MethodGet, "/products", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %v", resp)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty array, got %d items", len(data))
	}
}

// ─── OrderHandler additional tests ────────────────────────────────────────────

func TestOrderHandler_Get_BadID(t *testing.T) {
	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{})
	app.GET("/orders/:id", h.Get)

	w := do(app, http.MethodGet, "/orders/invalid", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestOrderHandler_Create_ProductNotFound(t *testing.T) {
	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{err: service.ErrNotFound})
	app.POST("/orders", h.Create)

	body := map[string]any{
		"items": []map[string]any{{"product_id": 999, "qty": 1}},
	}
	w := do(app, http.MethodPost, "/orders", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestOrderHandler_Create_InvalidPayload(t *testing.T) {
	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{})
	app.POST("/orders", h.Create)

	// Invalid JSON
	w := do(app, http.MethodPost, "/orders", nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid payload, got %d", w.Code)
	}
}

func TestOrderHandler_ListByUser_OK(t *testing.T) {
	o := &domain.Order{Total: 50.0, Status: domain.OrderStatusPending}
	o.ID = 1

	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{order: o})
	app.GET("/orders", h.List)

	w := do(app, http.MethodGet, "/orders", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOrderHandler_ListByUser_EmptyResult(t *testing.T) {
	app := newApp(t)
	h := handler.NewOrderHandler(&stubOrderSvc{})
	app.GET("/orders", h.List)

	w := do(app, http.MethodGet, "/orders", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %v", resp)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty array, got %d items", len(data))
	}
}
