// cmd/api is the HTTP + gRPC dual-stack entry point for the Showcase application.
// Wires: ORM + TxMiddleware, Redis Cache, TaskQueue, JWT, Casbin RBAC,
//        OAuth2 (Google + GitHub), OTel tracing, and Canary middleware.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/auth/rbac"
	"github.com/astra-go/astra/examples/showcase/internal/db"
	"github.com/astra-go/astra/examples/showcase/internal/domain"
	"github.com/astra-go/astra/examples/showcase/internal/handler"
	"github.com/astra-go/astra/examples/showcase/internal/repository"
	"github.com/astra-go/astra/examples/showcase/internal/service"
	"github.com/astra-go/astra/middleware"
	"github.com/astra-go/astra/middleware/security"
	astraorm "github.com/astra-go/astra/orm"
	astraotel "github.com/astra-go/astra/otel"
	"github.com/astra-go/astra/taskqueue"
	tqredis "github.com/astra-go/astra/taskqueue/redis"
	cacheredis "github.com/astra-go/astra/cache/redis"
	"github.com/casbin/casbin/v2"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	ctx := context.Background()

	// ── OTel tracing ──────────────────────────────────────────────────────────
	otelShutdown, err := astraotel.Setup(ctx, astraotel.Config{
		ServiceName:    "showcase-api",
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
	if err := db.Seed(database); err != nil {
		slog.Warn("seed skipped", slog.String("err", err.Error()))
	}

	// ── Redis Cache ───────────────────────────────────────────────────────────
	redisCache, err := cacheredis.New(cacheredis.Config{
		Addr:      getenv("REDIS_ADDR", "localhost:6379"),
		Password:  getenv("REDIS_PASSWORD", ""),
		KeyPrefix: "showcase:",
	})
	if err != nil {
		slog.Error("redis cache init failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer redisCache.Close()

	// ── TaskQueue client ──────────────────────────────────────────────────────
	tqBroker, err := tqredis.New(tqredis.Config{
		Addr:     getenv("REDIS_ADDR", "localhost:6379"),
		Password: getenv("REDIS_PASSWORD", ""),
	})
	if err != nil {
		slog.Error("taskqueue broker init failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer tqBroker.Close()
	tqClient := taskqueue.NewClient(tqBroker)
	defer tqClient.Close()

	// ── Casbin RBAC ───────────────────────────────────────────────────────────
	enforcer, err := casbin.NewEnforcer("config/rbac_model.conf", "config/rbac_policy.csv")
	if err != nil {
		slog.Error("casbin init failed", slog.String("err", err.Error()))
		os.Exit(1)
	}

	// ── Repositories (demo: single tenant ID=1) ───────────────────────────────
	const demoTenantID uint = 1
	productRepo := repository.NewProductRepo(database, demoTenantID)
	orderRepo   := repository.NewOrderRepo(database, demoTenantID)
	userRepo    := repository.NewUserRepo(database, demoTenantID)
	orderItemRepo := astraorm.NewRepository[domain.OrderItem](database)

	// ── Services ──────────────────────────────────────────────────────────────
	jwtSecret := getenv("JWT_SECRET", "change-me-in-production")
	userSvc    := service.NewUserSvc(userRepo, jwtSecret, 24*time.Hour)
	productSvc := service.NewCachedProductSvc(
		service.NewProductSvc(productRepo),
		redisCache,
		demoTenantID,
	)
	orderSvc := service.NewOrderSvc(orderRepo, orderItemRepo, productRepo, tqClient)

	// ── Handlers ──────────────────────────────────────────────────────────────
	productH := handler.NewProductHandler(productSvc)
	orderH   := handler.NewOrderHandler(orderSvc)
	adminH   := handler.NewAdminHandler(userSvc)
	authH    := handler.NewAuthHandler(userSvc, demoTenantID)

	// ── HTTP app ──────────────────────────────────────────────────────────────
	app := astra.New(astra.WithShutdownTimeout(15))
	app.Use(
		middleware.RequestID(),
		// Tracing must run before Logger so trace_id is in context when Logger fires.
		// Skip /health to avoid polluting traces with probe noise.
		middleware.Tracing(middleware.WithTracingSkipPaths("/health")),
		middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format:          "json",
			WithTraceContext: true, // appends trace_id + span_id to every log line
			SkipPaths:       []string{"/health"},
		}),
		middleware.Recovery(),
		middleware.CORSPermissive(),
	)

	// Canary rules are evaluated AFTER JWT so user_id is available in context.
	// Rule 1: explicit opt-in via header  X-Canary: true
	// Rule 2: 10 % of users by user_id hash (user_id % 10 == 0)
	// Rule 3: cookie-based opt-in         canary=1
	canaryMW := middleware.Canary([]middleware.CanaryRule{
		{Header: "X-Canary", HeaderRE: "^true$", Version: "v2"},
		{Cookie: "canary", CookieRE: "^1$", Version: "v2"},
		{UserIDKey: "user_id", Modulo: 10, Remainder: 0, Version: "v2"},
	})

	// ── Public routes ─────────────────────────────────────────────────────────
	app.GET("/health", handler.HealthHandler)
	app.GET("/auth/google/login",    authH.GoogleLogin)
	app.GET("/auth/google/callback", authH.GoogleCallback)
	app.GET("/auth/github/login",    authH.GithubLogin)
	app.GET("/auth/github/callback", authH.GithubCallback)

	// ── Protected API ─────────────────────────────────────────────────────────
	jwtMW := security.JWT(jwtSecret)
	rbacMW := rbac.Middleware(rbac.Config{
		Enforcer: enforcer,
		GetSubject: func(c *astra.Ctx) string {
			claims := security.GetClaims(c)
			if claims == nil {
				return ""
			}
			role, _ := claims.Extra["role"].(string)
			return role
		},
		Skipper: func(c *astra.Ctx) bool {
			return c.Request().URL.Path == "/health"
		},
	})

	v1 := app.Group("/api/v1")
	// JWT → RBAC → Canary: canary runs after JWT so user_id is in context.
	v1.Use(jwtMW, rbacMW, canaryMW)

	// Products
	v1.GET("/products",     productH.List)
	v1.POST("/products",    productH.Create)
	v1.GET("/products/:id", productH.Get)
	v1.PUT("/products/:id", productH.Update)
	v1.DELETE("/products/:id", productH.Delete)

	// Orders — wrapped in TxMiddleware for atomic stock decrement
	v1.GET("/orders",     orderH.List)
	v1.GET("/orders/:id", orderH.Get)
	v1.POST("/orders",    astraorm.TxMiddleware(database), orderH.Create)

	// Admin
	admin := v1.Group("/admin")
	admin.Use(handler.RequireRole(domain.RoleAdmin))
	admin.GET("/users/:id",        adminH.GetUser)
	admin.PUT("/users/:id/role",   adminH.UpdateRole)

	// ── Lifecycle ─────────────────────────────────────────────────────────────
	app.OnStart(func(_ context.Context) error {
		slog.Info("showcase api started",
			slog.String("http", getenv("API_ADDR", ":8080")),
			slog.String("redis", getenv("REDIS_ADDR", "localhost:6379")),
		)
		return nil
	})
	app.OnStop(func(_ context.Context) error {
		slog.Info("showcase api stopped")
		return nil
	})

	if err := app.Run(getenv("API_ADDR", ":8080")); err != nil {
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
