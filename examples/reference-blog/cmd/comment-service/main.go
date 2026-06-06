package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/examples/reference-blog/internal/proto"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Config ─────────────────────────────────────────────────────────────────
	dbDSN := getEnv("DATABASE_DSN", "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable")
	grpcPort := getEnv("GRPC_PORT", "9090")

	// ── Database ────────────────────────────────────────────────────────────────
	db, err := gorm.Open(postgres.Open(dbDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		slog.Error("connect database", "err", err)
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("get underlying db", "err", err)
		os.Exit(1)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// ── Repository ──────────────────────────────────────────────────────────────
	commentRepo := repository.NewCommentRepository(db)
	commentHandler := &grpcCommentHandler{repo: commentRepo}

	// ── gRPC Server ─────────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		slog.Error("listen tcp", "err", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	proto.RegisterCommentServiceServer(grpcServer, commentHandler)
	reflection.Register(grpcServer)

	go func() {
		slog.Info("comment-service starting", "port", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("grpc serve", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down comment-service...")
	grpcServer.GracefulStop()
	slog.Info("comment-service stopped")
}

// ── gRPC Handler ────────────────────────────────────────────────────────────

type grpcCommentHandler struct {
	proto.UnimplementedCommentServiceServer
	repo *repository.CommentRepository
}

func (h *grpcCommentHandler) CreateComment(ctx context.Context, req *proto.CreateCommentRequest) (*proto.CommentResponse, error) {
	// TODO: implement
	return &proto.CommentResponse{}, nil
}

func (h *grpcCommentHandler) ListComments(ctx context.Context, req *proto.ListCommentsRequest) (*proto.ListCommentsResponse, error) {
	pageNum := int(req.Page)
	pageSize := int(req.PageSize)
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	var page orm.Page
	_ = page // suppress unused

	resp := &proto.ListCommentsResponse{
		Comments: make([]*proto.CommentResponse, 0),
		Total:    0,
		Page:     1,
		PageSize: 10,
	}
	return resp, nil
}

func (h *grpcCommentHandler) DeleteComment(ctx context.Context, req *proto.DeleteCommentRequest) (*proto.Empty, error) {
	err := h.repo.Delete(ctx, uint(req.Id))
	return &proto.Empty{}, err
}

func (h *grpcCommentHandler) LikeComment(ctx context.Context, req *proto.LikeCommentRequest) (*proto.CommentResponse, error) {
	// TODO: implement
	return &proto.CommentResponse{}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
