package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	astraorm "github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/taskqueue"
	"gorm.io/gorm"
)

// orderRepo is the minimal interface OrderSvc needs from the repository layer.
type orderRepo interface {
	Create(ctx context.Context, o *domain.Order) error
	FindByID(ctx context.Context, id uint) (*domain.Order, error)
	FindWithItems(ctx context.Context, orderID uint) (*domain.Order, error)
	FindByUser(ctx context.Context, userID uint, p *astraorm.Page) ([]domain.Order, int64, error)
	Updates(ctx context.Context, id uint, values any) error
}

type orderItemCreator interface {
	Create(ctx context.Context, item *domain.OrderItem) error
}

// OrderSvc handles order creation with atomic stock reservation.
// The entire create flow runs inside a single DB transaction supplied by
// orm.TxMiddleware — no explicit Begin/Commit here.
type OrderSvc struct {
	orders   orderRepo
	items    orderItemCreator
	products productRepo
	tq       *taskqueue.Client
}

func NewOrderSvc(
	orders orderRepo,
	items orderItemCreator,
	products productRepo,
	tq *taskqueue.Client,
) *OrderSvc {
	return &OrderSvc{orders: orders, items: items, products: products, tq: tq}
}

// Create places an order and atomically decrements stock for each line item.
// Must be called inside a request wrapped by orm.TxMiddleware so that stock
// decrements and order insertion share the same transaction.
func (s *OrderSvc) Create(ctx context.Context, tenantID, userID uint, req domain.CreateOrderReq) (*domain.Order, error) {
	var total float64
	type resolvedItem struct {
		product *domain.Product
		qty     int
	}
	resolved := make([]resolvedItem, 0, len(req.Items))

	// Validate all products exist and have sufficient stock before writing anything.
	for _, item := range req.Items {
		p, err := s.products.FindByID(ctx, item.ProductID)
		if err != nil {
			return nil, fmt.Errorf("product %d: %w", item.ProductID, mapGORMErr(err))
		}
		if p.Stock < item.Qty {
			return nil, ErrInsufficientStock
		}
		total += p.Price * float64(item.Qty)
		resolved = append(resolved, resolvedItem{product: p, qty: item.Qty})
	}

	// Insert the order header.
	// v2 canary: apply 5 % loyalty discount.
	if req.CanaryVersion == "v2" {
		total *= 0.95
	}
	order := &domain.Order{
		TenantID: tenantID,
		UserID:   userID,
		Total:    total,
		Status:   domain.OrderStatusPending,
	}
	if err := s.orders.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("order create: %w", err)
	}

	// Insert line items and atomically decrement stock.
	for _, ri := range resolved {
		if err := s.products.DecrStock(ctx, ri.product.ID, ri.qty); err != nil {
			// DecrStock returns gorm.ErrRecordNotFound on insufficient stock —
			// this can happen under concurrent load even after the pre-check above.
			if err == gorm.ErrRecordNotFound {
				return nil, ErrInsufficientStock
			}
			return nil, fmt.Errorf("decr stock product %d: %w", ri.product.ID, err)
		}
		item := &domain.OrderItem{
			OrderID:   order.ID,
			ProductID: ri.product.ID,
			Qty:       ri.qty,
			Price:     ri.product.Price,
		}
		if err := s.items.Create(ctx, item); err != nil {
			return nil, fmt.Errorf("order item create: %w", err)
		}
	}

	// Enqueue confirmation email (non-fatal — order is already committed).
	if s.tq != nil {
		payload, _ := json.Marshal(map[string]any{"order_id": order.ID, "user_id": userID})
		_, _ = s.tq.EnqueueTask(ctx, "order:confirm-email", payload,
			taskqueue.WithQueue("critical"),
			taskqueue.WithMaxRetries(5),
		)
	}

	// Reload with associations so the caller gets a fully-populated order.
	full, err := s.orders.FindWithItems(ctx, order.ID)
	if err != nil {
		return order, nil // non-fatal: return the bare order rather than failing
	}
	return full, nil
}

func (s *OrderSvc) Get(ctx context.Context, orderID uint) (*domain.Order, error) {
	o, err := s.orders.FindWithItems(ctx, orderID)
	return o, mapGORMErr(err)
}

func (s *OrderSvc) ListByUser(ctx context.Context, userID uint, page astraorm.Page) ([]domain.Order, int64, error) {
	return s.orders.FindByUser(ctx, userID, &page)
}
