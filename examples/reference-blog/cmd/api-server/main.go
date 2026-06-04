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
	"github.com/astra-go/astra/cache/redis"
	bloggrpc "github.com/astra-go/astra/examples/reference-blog/internal/grpc"
	"github.com/astra-go/astra/mq"
	"github.com/astra-go/astra/otel"
	"github.com/astra-go/astra/otel/example"
	"github.com/astra-go/astra/examples/reference-blog/internal/handler"
	"github.com/astra-go/astra/examples/reference-blog/internal/middleware"
	"github.com/astra-go/astra/examples/reference-blog/internal/repository"
	"github.com/astra-go/astra/examples/reference-blog/internal/service"
	"github.com/astra-go/astra/search/elastic"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Config ─────────────────────────────────────────────────────────────────
	dbDSN := getEnv("DATABASE_DSN", "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	kafkaBrokers := getEnv("KAFKA_BROKERS", "localhost:9092")
	esAddrs := getEnv("ELASTICSEARCH_ADDRESSES", "http://localhost:9200")
	jaegerEndpoint := getEnv("JAEGER_ENDPOINT", "http://localhost:14268/api/traces")
	serverPort := getEnv("SERVER_PORT", "8080")
	jwtSecret := getEnv("JWT_SECRET", "reference-blog-jwt-secret-change-in-production")
	serverMode := getEnv("SERVER_MODE", "debug")

	// ── Observability ───────────────────────────────────────────────────────────
	otelShutdown, err := otel.Init(ctx,
		otel.WithServiceName("reference-blog/api-server"),
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
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// ── Redis ───────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Error("connect redis", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()
	cacheClient := redis.NewCache(rdb)

	// ── Kafka Producer ──────────────────────────────────────────────────────────
	producer, err := mq.NewProducer("kafka", mq.ProducerOptions{
		Brokers:   parseBrokers(kafkaBrokers),
		ClientID:  "api-server",
		Compress:  true,
		RequiredAcks: mq.WaitForAll,
	})
	if err != nil {
		slog.Error("create kafka producer", "err", err)
		os.Exit(1)
	}
	defer producer.Close()

	// ── Elasticsearch ───────────────────────────────────────────────────────────
	searcher := elastic.NewSearcher(esAddrs, "", "")

	// ── Repositories ────────────────────────────────────────────────────────────
	userRepo := repository.NewUserRepository(db)
	articleRepo := repository.NewArticleRepository(db)

	// ── Services ────────────────────────────────────────────────────────────────
	authSvc := service.NewAuthService(userRepo, jwtSecret, 15*time.Minute)
	notificationSvc := service.NewNotificationService(producer)
	searchSvc := service.NewSearchService(searcher)
	articleSvc := service.NewArticleService(articleRepo, cacheClient, notificationSvc, searchSvc)
	commentSvc := service.NewCommentService(nil, notificationSvc) // Comment CRUD via gRPC

	// ── Handlers ────────────────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authSvc)
	articleHandler := handler.NewArticleHandler(articleSvc)
	searchHandler := handler.NewSearchHandler(searchSvc)

	// Comment handler delegates to gRPC (via comment-service)
	grpcCommentAddr := getEnv("GRPC_COMMENT_SERVICE_ADDR", "localhost:9090")
	commentClient, err := bloggrpc.NewCommentClient(grpcCommentAddr)
	if err != nil {
		slog.Warn("failed to connect to comment-service via gRPC, comment features unavailable", "addr", grpcCommentAddr, "err", err)
	} else {
		slog.Info("connected to comment-service", "addr", grpcCommentAddr)
		defer commentClient.Close()
	}
	commentHandler := handler.NewCommentHandler(nil, commentClient)

	// ── Middleware ───────────────────────────────────────────────────────────────
	authMw := middleware.NewAuthMiddleware(authSvc)

	// ── App ─────────────────────────────────────────────────────────────────────
	app := astra.New(astra.WithMode(serverMode))

	// Health check
	app.GET("/health", func(c *astra.Ctx) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Prometheus metrics (if otel enabled)
	if example.IsEnabled() {
		app.Use(example.MetricsMiddleware())
	}

	// Auth routes (public)
	api := app.Group("/api/v1")
	api.POST("/auth/register", authHandler.Register)
	api.POST("/auth/login", authHandler.Login)

	// Protected routes
	protected := api.Group("", authMw.Authenticate)

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

func parseBrokers(s string) []string {
	var brokers []string
	for _, b := range splitComma(s) {
		b = trim(b)
		if b != "" {
			brokers = append(brokers, b)
		}
	}
	return brokers
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
