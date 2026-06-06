package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/astra-go/astra"
	bloggrpc "github.com/astra-go/astra/examples/reference-blog/internal/grpc"
	"github.com/astra-go/astra/examples/reference-blog/internal/handler"
	// "github.com/astra-go/astra/examples/reference-blog/internal/middleware"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Config ─────────────────────────────────────────────────────────────────
	dbDSN := getEnv("DATABASE_DSN", "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable")
	serverPort := getEnv("SERVER_PORT", "8080")
	jwtSecret := getEnv("JWT_SECRET", "reference-blog-jwt-secret-change-in-production")
	serverMode := getEnv("SERVER_MODE", "debug")

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
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// ── Repositories ────────────────────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	articleRepo := repository.NewArticleRepository(db)

	// ── Services ────────────────────────────────────────────────────────────────
	authSvc := service.NewAuthService(userRepo, jwtSecret, 15*time.Minute)
	articleSvc := service.NewArticleService(articleRepo, nil, nil, nil)

	// ── Handlers ────────────────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authSvc)
	articleHandler := handler.NewArticleHandler(articleSvc)
	searchHandler := handler.NewSearchHandler(nil)

	grpcCommentAddr := getEnv("GRPC_COMMENT_SERVICE_ADDR", "localhost:9090")
	commentClient, err := bloggrpc.NewCommentClient(grpcCommentAddr)
	if err != nil {
		slog.Warn("failed to connect to comment-service via gRPC", "addr", grpcCommentAddr, "err", err)
	} else {
		slog.Info("connected to comment-service", "addr", grpcCommentAddr)
		defer commentClient.Close()
	}
	commentHandler := handler.NewCommentHandler(nil, commentClient)

	// ── Middleware ───────────────────────────────────────────────────────────────
	// authMw := middleware.NewAuthMiddleware(authSvc)

	// ── App ─────────────────────────────────────────────────────────────────────
	app := astra.New(astra.WithMode(astra.Mode(serverMode)))

	// Health check
	app.GET("/health", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Auth routes (public)
	api := app.Group("/api/v1")
	api.POST("/auth/register", authHandler.Register)
	api.POST("/auth/login", authHandler.Login)

	// Protected routes (no auth for now - TODO fix middleware)
	protected := api.Group("")
	// protected.Use(authMw.Authenticate)

	_ = protected // suppress unused

	// Articles
	protected.POST("/articles", articleHandler.Create)
	protected.GET("/articles", articleHandler.List)
	protected.GET("/articles/:id", articleHandler.GetByID)
	protected.PUT("/articles/:id", articleHandler.Update)
	protected.POST("/articles/:id/publish", articleHandler.Publish)
	protected.DELETE("/articles/:id", articleHandler.Delete)
	protected.POST("/articles/:id/like", articleHandler.Like)
	protected.GET("/articles/author/:author_id", articleHandler.ListByAuthor)

	// Comments (delegated to gRPC comment-service)
	protected.POST("/comments", commentHandler.Create)
	protected.GET("/comments/article/:article_id", commentHandler.ListByArticle)
	protected.DELETE("/comments/:id", commentHandler.Delete)
	protected.POST("/comments/:id/like", commentHandler.Like)

	// Search
	protected.GET("/search", searchHandler.Search)

	// ── Start ───────────────────────────────────────────────────────────────────
	srv := &http.Server{Addr: ":" + serverPort, Handler: app}

	go func() {
		slog.Info("api-server starting", "port", serverPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		slog.Error("server shutdown", "err", err)
	}
	slog.Info("api-server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
