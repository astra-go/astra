// Package db handles database initialisation and schema migration.
package db

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/astra-go/astra/examples/showcase/internal/domain"
	astraorm "github.com/astra-go/astra/orm"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config holds the Postgres connection parameters.
type Config struct {
	DSN         string
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
}

// Open opens a Postgres connection, applies the connection pool, and returns
// the *gorm.DB. Call Migrate after Open to ensure the schema is up to date.
func Open(cfg Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}

	poolCfg := astraorm.DefaultPoolConfig
	if cfg.MaxOpen > 0 {
		poolCfg.MaxOpen = cfg.MaxOpen
	}
	if cfg.MaxIdle > 0 {
		poolCfg.MaxIdle = cfg.MaxIdle
	}
	if cfg.MaxLifetime > 0 {
		poolCfg.MaxLifetime = cfg.MaxLifetime
	}
	if err := astraorm.SetPool(db, poolCfg); err != nil {
		return nil, fmt.Errorf("db: set pool: %w", err)
	}
	return db, nil
}

// Migrate runs AutoMigrate for all showcase entities.
// Safe to call on every startup — GORM only adds missing columns/indexes.
func Migrate(db *gorm.DB) error {
	slog.Info("running database migrations")
	err := db.AutoMigrate(
		&domain.Tenant{},
		&domain.User{},
		&domain.Product{},
		&domain.Order{},
		&domain.OrderItem{},
		&domain.AuditLog{},
	)
	if err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	slog.Info("database migrations complete")
	return nil
}

// Seed inserts a default tenant and admin user when the database is empty.
// Idempotent — skips insertion if the default tenant already exists.
func Seed(db *gorm.DB) error {
	var count int64
	db.Model(&domain.Tenant{}).Count(&count)
	if count > 0 {
		return nil
	}

	tenant := &domain.Tenant{Name: "demo", Plan: domain.PlanPro}
	if err := db.Create(tenant).Error; err != nil {
		return fmt.Errorf("db: seed tenant: %w", err)
	}

	admin := &domain.User{
		TenantID: tenant.ID,
		Email:    "admin@demo.local",
		Name:     "Demo Admin",
		Role:     domain.RoleAdmin,
	}
	if err := db.Create(admin).Error; err != nil {
		return fmt.Errorf("db: seed admin: %w", err)
	}

	products := []domain.Product{
		{TenantID: tenant.ID, Name: "Widget Pro", Price: 29.99, Stock: 100, Category: "widgets"},
		{TenantID: tenant.ID, Name: "Gadget X",   Price: 99.99, Stock: 50,  Category: "gadgets"},
		{TenantID: tenant.ID, Name: "Doohickey",  Price: 9.99,  Stock: 200, Category: "misc"},
	}
	if err := db.Create(&products).Error; err != nil {
		return fmt.Errorf("db: seed products: %w", err)
	}

	slog.Info("database seeded", slog.String("tenant", tenant.Name), slog.Uint64("tenant_id", uint64(tenant.ID)))
	return nil
}
