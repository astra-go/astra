package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/astra-go/astra/examples/showcase/internal/pb/inventorypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := os.Getenv("GRPC_ADDR")
	if addr == "" {
		addr = "localhost:9091"
	}
	// ":port" means local server — prepend localhost
	if len(addr) > 0 && addr[0] == ':' {
		addr = "localhost" + addr
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "grpc connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := inventorypb.NewInventoryServiceClient(conn)

	// Each RPC gets its own context to avoid shared deadline expiry.
	newCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 15*time.Second)
	}

	const tenantID = 1

	// 5-1. GetStock
	fmt.Println("\n▶ 5-1. gRPC GetStock (product_id=1)")
	ctx, cancel := newCtx()
	resp, err := client.GetStock(ctx, &inventorypb.GetStockRequest{TenantId: tenantID, ProductId: 1})
	cancel()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
	} else {
		fmt.Printf("  product_id=%d  stock=%d\n", resp.ProductId, resp.Stock)
	}

	// 5-2. BatchGetStock
	fmt.Println("\n▶ 5-2. gRPC BatchGetStock (product_ids=1,2,3)")
	ctx, cancel = newCtx()
	batch, err := client.BatchGetStock(ctx, &inventorypb.BatchGetStockRequest{
		TenantId: tenantID, ProductIds: []uint64{1, 2, 3},
	})
	cancel()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
	} else {
		for _, item := range batch.Items {
			fmt.Printf("  product_id=%d  stock=%d\n", item.ProductId, item.Stock)
		}
	}

	// 5-3. ListLowStock (streaming)
	fmt.Println("\n▶ 5-3. gRPC ListLowStock (threshold=10)")
	ctx, cancel = newCtx()
	stream, err := client.ListLowStock(ctx, &inventorypb.ListLowStockRequest{TenantId: tenantID, Threshold: 10})
	if err != nil {
		fmt.Printf("  error: %v\n", err)
	} else {
		for {
			item, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Printf("  stream error: %v\n", err)
				break
			}
			fmt.Printf("  product_id=%d  name=%s  stock=%d\n", item.ProductId, item.ProductName, item.Stock)
		}
	}
	cancel()

	// 5-4. DecrStock
	fmt.Println("\n▶ 5-4. gRPC DecrStock (product_id=1, qty=1)")
	ctx, cancel = newCtx()
	decrResp, err := client.DecrStock(ctx, &inventorypb.DecrStockRequest{
		TenantId: tenantID, ProductId: 1, Quantity: 1,
	})
	cancel()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
	} else {
		fmt.Printf("  product_id=%d  stock=%d\n", decrResp.ProductId, decrResp.Stock)
	}

	// 5-5. 租户隔离
	fmt.Println("\n▶ 5-5. gRPC 租户隔离 (tenant_id=999, 预期 not found)")
	ctx, cancel = newCtx()
	_, err = client.GetStock(ctx, &inventorypb.GetStockRequest{TenantId: 1, ProductId: 1})
	cancel()
	if err != nil {
		fmt.Printf("  预期错误: %v\n", err)
	} else {
		fmt.Println("  ✗ 应该返回错误但没有！")
	}
}
