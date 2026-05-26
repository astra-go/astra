package grpchandler

import (
	"context"
	"errors"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/pb/inventorypb"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// InventoryServer implements inventorypb.InventoryServiceServer.
// It creates a tenant-scoped ProductRepo per request so that tenant_id from
// the gRPC message drives row-level isolation — the same guarantee the HTTP
// layer gets from JWT claims + TenantRepository.
type InventoryServer struct {
	inventorypb.UnimplementedInventoryServiceServer
	db *gorm.DB
}

func NewInventoryServer(db *gorm.DB) *InventoryServer {
	return &InventoryServer{db: db}
}

func (s *InventoryServer) repoFor(tenantID uint64) *repository.ProductRepo {
	return repository.NewProductRepo(s.db, uint(tenantID))
}

// GetStock returns the current stock level for a single product.
func (s *InventoryServer) GetStock(ctx context.Context, req *inventorypb.GetStockRequest) (*inventorypb.StockResponse, error) {
	if req.TenantId == 0 || req.ProductId == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id and product_id are required")
	}
	var p domain.Product
	err := s.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", req.ProductId, req.TenantId).
		First(&p).Error
	if err != nil {
		return nil, toGRPCErr(err, "get stock")
	}
	return &inventorypb.StockResponse{ProductId: req.ProductId, Stock: uint32(p.Stock)}, nil
}

// BatchGetStock returns stock levels for multiple products in one call.
func (s *InventoryServer) BatchGetStock(ctx context.Context, req *inventorypb.BatchGetStockRequest) (*inventorypb.BatchStockResponse, error) {
	if req.TenantId == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	if len(req.ProductIds) == 0 {
		return &inventorypb.BatchStockResponse{}, nil
	}
	var products []domain.Product
	err := s.db.WithContext(ctx).
		Where("id IN ? AND tenant_id = ? AND deleted_at IS NULL", req.ProductIds, req.TenantId).
		Find(&products).Error
	if err != nil {
		return nil, toGRPCErr(err, "batch get stock")
	}
	resp := &inventorypb.BatchStockResponse{
		Items: make([]*inventorypb.StockResponse, 0, len(products)),
	}
	for _, p := range products {
		resp.Items = append(resp.Items, &inventorypb.StockResponse{
			ProductId: uint64(p.ID),
			Stock:     uint32(p.Stock),
		})
	}
	return resp, nil
}

// DecrStock atomically decrements stock and returns the updated level.
func (s *InventoryServer) DecrStock(ctx context.Context, req *inventorypb.DecrStockRequest) (*inventorypb.StockResponse, error) {
	if req.TenantId == 0 || req.ProductId == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id and product_id are required")
	}
	if req.Quantity == 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be > 0")
	}
	repo := s.repoFor(req.TenantId)
	if err := repo.DecrStock(ctx, uint(req.ProductId), int(req.Quantity)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient stock or product not found")
		}
		return nil, toGRPCErr(err, "decr stock")
	}
	p, err := repo.FindByID(ctx, uint(req.ProductId))
	if err != nil {
		return nil, toGRPCErr(err, "get updated stock")
	}
	return &inventorypb.StockResponse{ProductId: req.ProductId, Stock: uint32(p.Stock)}, nil
}

// ListLowStock streams products whose stock is at or below the threshold.
func (s *InventoryServer) ListLowStock(req *inventorypb.ListLowStockRequest, stream grpc.ServerStreamingServer[inventorypb.StockItem]) error {
	if req.TenantId == 0 {
		return status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	products, err := s.repoFor(req.TenantId).FindLowStock(stream.Context(), int(req.Threshold))
	if err != nil {
		return toGRPCErr(err, "list low stock")
	}
	for _, p := range products {
		if err := stream.Send(&inventorypb.StockItem{
			ProductId:   uint64(p.ID),
			ProductName: p.Name,
			Stock:       uint32(p.Stock),
		}); err != nil {
			return err
		}
	}
	return nil
}

// toGRPCErr maps internal errors to gRPC status codes.
func toGRPCErr(err error, op string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status.Errorf(codes.NotFound, "%s: not found", op)
	}
	return status.Errorf(codes.Internal, "%s: %v", op, err)
}
