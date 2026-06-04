package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/astra-go/astra/otel"
	"github.com/astra-go/astra/orm"
	"github.com/astra-go/astra/examples/reference-blog/internal/proto"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Config ─────────────────────────────────────────────────────────────────
	dbDSN := getEnv("DATABASE_DSN", "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable")
	grpcPort := getEnv("GRPC_PORT", "9090")
	jaegerEndpoint := getEnv("JAEGER_ENDPOINT", "http://localhost:14268/api/traces")

	// ── Observability ───────────────────────────────────────────────────────────
	otelShutdown, err := otel.Init(ctx,
		otel.WithServiceName("reference-blog/comment-service"),
		otel.WithJaegerEndpoint(jaegerEndpoint),
	)
	if err != nil {
		slog.Warn("otel init failed, continuing without tracing", "err", err)
	} else {
		defer otelShutdown(context.Background())
	}

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

	// ── Repository & Service ────────────────────────────────────────────────────
	commentRepo := repository.NewCommentRepository(db)
	commentSvc := service.NewCommentServiceServer(commentRepo)
	commentHandler := &grpcCommentHandler{svc: commentSvc}

	// ── gRPC Server ─────────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		slog.Error("listen tcp", "err", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			otel.GRPCUnaryServerInterceptor(),
		),
	)
	proto.RegisterCommentServiceServer(grpcServer, commentHandler)
	reflection.Register(grpcServer) // enable grpcurl / grpcox

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

// ── gRPC Handler (implements proto.CommentServiceServer) ────────────────────

type grpcCommentHandler struct {
	proto.UnimplementedCommentServiceServer
	svc *service.CommentServiceServer
}

func (h *grpcCommentHandler) CreateComment(ctx context.Context, req *proto.CreateCommentRequest) (*proto.CommentResponse, error) {
	var parentID *uint
	if req.ParentId != nil && *req.ParentId != 0 {
		pid := uint(*req.ParentId)
		parentID = &pid
	}

	comment, err := h.svc.Create(ctx, service.CreateCommentServerRequest{
		ArticleID: uint(req.ArticleId),
		UserID:    uint(req.UserId),
		ParentID:  parentID,
		Content:   req.Content,
	})
	if err != nil {
		return nil, err
	}
	return commentToProto(comment), nil
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
	page := orm.Page{PageNum: pageNum, PageSize: pageSize, Offset: (pageNum - 1) * pageSize}

	comments, total, err := h.svc.ListByArticle(ctx, uint(req.ArticleId), page)
	if err != nil {
		return nil, err
	}

	resp := &proto.ListCommentsResponse{
		Comments: make([]*proto.CommentResponse, 0, len(comments)),
		Total:    int64(total),
		Page:     int32(page.PageNum),
		PageSize: int32(page.PageSize),
	}
	for _, c := range comments {
		resp.Comments = append(resp.Comments, commentToProto(c))
	}
	return resp, nil
}

func (h *grpcCommentHandler) DeleteComment(ctx context.Context, req *proto.DeleteCommentRequest) (*proto.Empty, error) {
	err := h.svc.Delete(ctx, uint(req.Id), uint(req.UserId))
	return &proto.Empty{}, err
}

func (h *grpcCommentHandler) LikeComment(ctx context.Context, req *proto.LikeCommentRequest) (*proto.CommentResponse, error) {
	comment, err := h.svc.Like(ctx, uint(req.Id))
	if err != nil {
		return nil, err
	}
	return commentToProto(comment), nil
}

func commentToProto(c *service.CommentServerResponse) *proto.CommentResponse {
	resp := &proto.CommentResponse{
		Id:        uint32(c.ID),
		ArticleId: uint32(c.ArticleID),
		UserId:    uint32(c.UserID),
		Content:   c.Content,
		LikeCount: c.LikeCount,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	if c.ParentID != nil {
		pid := uint32(*c.ParentID)
		resp.ParentId = &pid
	}
	return resp
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
