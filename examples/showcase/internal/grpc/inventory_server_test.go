package grpchandler_test

import (
	"context"
	"testing"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	grpchandler "github.com/astra-go/astra/examples/showcase/internal/grpc"
	"github.com/astra-go/astra/examples/showcase/internal/pb/inventorypb"
	"github.com/glebarez/sqlite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&domain.Tenant{}, &domain.Product{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedProduct(t *testing.T, db *gorm.DB, tenantID uint, name string, stock int, price float64) *domain.Product {
	t.Helper()
	p := &domain.Product{TenantID: tenantID, Name: name, Stock: stock, Price: price}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return p
}

func TestInventoryServer_GetStock(t *testing.T) {
	db := openTestDB(t)
	p := seedProduct(t, db, 1, "Widget", 42, 9.99)

	srv := grpchandler.NewInventoryServer(db)
	resp, err := srv.GetStock(context.Background(), &inventorypb.GetStockRequest{
		TenantId: 1, ProductId: uint64(p.ID),
	})
	if err != nil {
		t.Fatalf("GetStock: %v", err)
	}
	if resp.Stock != 42 {
		t.Fatalf("expected stock 42, got %d", resp.Stock)
	}
}

func TestInventoryServer_GetStock_NotFound(t *testing.T) {
	srv := grpchandler.NewInventoryServer(openTestDB(t))
	_, err := srv.GetStock(context.Background(), &inventorypb.GetStockRequest{
		TenantId: 1, ProductId: 999,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestInventoryServer_GetStock_TenantIsolation(t *testing.T) {
	db := openTestDB(t)
	p := seedProduct(t, db, 2, "Widget", 10, 5.0) // tenant 2

	srv := grpchandler.NewInventoryServer(db)
	// tenant 1 should not see tenant 2's product
	_, err := srv.GetStock(context.Background(), &inventorypb.GetStockRequest{
		TenantId: 1, ProductId: uint64(p.ID),
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound for cross-tenant access, got %v", err)
	}
}

func TestInventoryServer_GetStock_MissingArgs(t *testing.T) {
	srv := grpchandler.NewInventoryServer(openTestDB(t))
	_, err := srv.GetStock(context.Background(), &inventorypb.GetStockRequest{TenantId: 0, ProductId: 1})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestInventoryServer_BatchGetStock(t *testing.T) {
	db := openTestDB(t)
	p1 := seedProduct(t, db, 1, "A", 10, 1.0)
	p2 := seedProduct(t, db, 1, "B", 20, 2.0)
	seedProduct(t, db, 2, "C", 30, 3.0) // different tenant — must not appear

	srv := grpchandler.NewInventoryServer(db)
	resp, err := srv.BatchGetStock(context.Background(), &inventorypb.BatchGetStockRequest{
		TenantId:   1,
		ProductIds: []uint64{uint64(p1.ID), uint64(p2.ID), 9999}, // 9999 is missing
	})
	if err != nil {
		t.Fatalf("BatchGetStock: %v", err)
	}
	// missing product is skipped, not an error
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
}

func TestInventoryServer_DecrStock(t *testing.T) {
	db := openTestDB(t)
	p := seedProduct(t, db, 1, "Widget", 10, 9.99)

	srv := grpchandler.NewInventoryServer(db)
	resp, err := srv.DecrStock(context.Background(), &inventorypb.DecrStockRequest{
		TenantId: 1, ProductId: uint64(p.ID), Quantity: 3,
	})
	if err != nil {
		t.Fatalf("DecrStock: %v", err)
	}
	if resp.Stock != 7 {
		t.Fatalf("expected stock 7, got %d", resp.Stock)
	}
}

func TestInventoryServer_DecrStock_Insufficient(t *testing.T) {
	db := openTestDB(t)
	p := seedProduct(t, db, 1, "Widget", 2, 9.99)

	srv := grpchandler.NewInventoryServer(db)
	_, err := srv.DecrStock(context.Background(), &inventorypb.DecrStockRequest{
		TenantId: 1, ProductId: uint64(p.ID), Quantity: 5,
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
}

func TestInventoryServer_ListLowStock(t *testing.T) {
	db := openTestDB(t)
	seedProduct(t, db, 1, "Low-A", 3, 1.0)
	seedProduct(t, db, 1, "Low-B", 5, 2.0)
	seedProduct(t, db, 1, "High", 100, 3.0)
	seedProduct(t, db, 2, "Other-tenant", 1, 1.0) // different tenant

	srv := grpchandler.NewInventoryServer(db)
	stream := &collectStream{ctx: context.Background()}
	err := srv.ListLowStock(&inventorypb.ListLowStockRequest{TenantId: 1, Threshold: 5}, stream)
	if err != nil {
		t.Fatalf("ListLowStock: %v", err)
	}
	if len(stream.items) != 2 {
		t.Fatalf("expected 2 low-stock items, got %d", len(stream.items))
	}
}

// collectStream is a test double for grpc.ServerStreamingServer[StockItem].
type collectStream struct {
	ctx   context.Context
	items []*inventorypb.StockItem
}

func (s *collectStream) Send(item *inventorypb.StockItem) error {
	s.items = append(s.items, item)
	return nil
}
func (s *collectStream) Context() context.Context          { return s.ctx }
func (s *collectStream) SetHeader(md metadata.MD) error    { return nil }
func (s *collectStream) SendHeader(md metadata.MD) error   { return nil }
func (s *collectStream) SetTrailer(md metadata.MD)         {}
func (s *collectStream) SendMsg(m any) error               { return nil }
func (s *collectStream) RecvMsg(m any) error               { return nil }
