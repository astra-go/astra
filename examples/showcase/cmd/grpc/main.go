// cmd/grpc is the dual-stack HTTP+gRPC entry point for the Showcase application.
// HTTP :8081 — health check
// gRPC :9091 — InventoryService
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/astra-go/astra"
	grpcserver "github.com/astra-go/astra/grpc"
	"github.com/astra-go/astra/examples/showcase/internal/db"
	grpchandler "github.com/astra-go/astra/examples/showcase/internal/grpc"
	"github.com/astra-go/astra/examples/showcase/internal/handler"
	"github.com/astra-go/astra/examples/showcase/internal/pb/inventorypb"
	"github.com/astra-go/astra/middleware"
	astraotel "github.com/astra-go/astra/otel"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present — ignored in production where env vars are injected externally.
	_ = godotenv.Load()

	ctx := context.Background()

	// ── OTel tracing ──────────────────────────────────────────────────────────
	otelShutdown, err := astraotel.Setup(ctx, astraotel.Config{
		ServiceName:    "showcase-grpc",
		ServiceVersion: "0.1.0",
		OTLPEndpoint:   getenv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		Insecure:       true,
		SampleRatio:    1.0,
		EnableStdout:   getenv("OTEL_STDOUT", "") != "",
	})
	if err != nil {
		slog.Warn("otel setup failed, tracing disabled", slog.String("err", err.Error()))
	} else {
		defer otelShutdown(context.Background())
	}

	// ── Database ──────────────────────────────────────────────────────────────
	dsn := getenv("DATABASE_URL", "postgres://showcase:showcase@localhost:5432/showcase?sslmode=disable")
	database, err := db.Open(db.Config{DSN: dsn, MaxOpen: 25, MaxIdle: 5, MaxLifetime: time.Hour})
	if err != nil {
		slog.Error("db open failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
	if err := db.Migrate(database); err != nil {
		slog.Error("migration failed", slog.String("err", err.Error()))
		os.Exit(1)
	}

	// ── Repository ────────────────────────────────────────────────────────────
	// InventoryServer creates per-request tenant-scoped repos from the raw DB.
	// No fixed tenantID here — tenant isolation is driven by the gRPC request.

	// ── HTTP app ──────────────────────────────────────────────────────────────
	app := astra.New(astra.WithShutdownTimeout(15))
	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recovery())
	app.GET("/health", handler.HealthHandler)

	// ── Dual-stack server ─────────────────────────────────────────────────────
	srv := grpcserver.New(app,
		grpcserver.WithHTTPAddr(getenv("HTTP_ADDR", ":8081")),
		grpcserver.WithGRPCAddr(getenv("GRPC_ADDR", ":9091")),
		grpcserver.WithTimeout(30*time.Second),
		grpcserver.WithUnaryInterceptors(
			grpcserver.UnaryInterceptorRecovery(),
			grpcserver.UnaryInterceptorTracing(),
			grpcserver.UnaryInterceptorLogger(),
		),
		grpcserver.WithStreamInterceptors(
			grpcserver.StreamInterceptorRecovery(),
			grpcserver.StreamInterceptorTracing(),
			grpcserver.StreamInterceptorLogger(),
		),
	)

	// ── Register gRPC services ────────────────────────────────────────────────
	inventorypb.RegisterInventoryServiceServer(srv.GRPC, grpchandler.NewInventoryServer(database))

	slog.Info("showcase grpc server starting",
		slog.String("http", getenv("HTTP_ADDR", ":8081")),
		slog.String("grpc", getenv("GRPC_ADDR", ":9091")),
	)
	if err := srv.Run(); err != nil {
		slog.Error("server error", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
