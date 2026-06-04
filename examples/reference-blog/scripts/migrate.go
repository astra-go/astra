// scripts/migrate.go — Database migration tool for reference-blog.
// Run with: go run scripts/migrate.go
//
// Supports: postgres (default), mysql, sqlite
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/astra-go/astra/examples/reference-blog/internal/domain"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	dsn := flag.String("dsn", "postgres://bloguser:blogpass@localhost:5432/blog?sslmode=disable", "Database connection string")
	driver := flag.String("driver", "postgres", "Database driver: postgres, mysql, sqlite")
	drop := flag.Bool("drop", false, "Drop existing tables before migrating")
	seed := flag.Bool("seed", false, "Insert seed data")
	flag.Parse()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	db, err := openDB(*driver, *dsn)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}

	// Run auto-migrate
	if *drop {
		slog.Warn("dropping existing tables...")
		_ = db.Migrator().DropTable(&domain.Comment{}, &domain.Article{}, &domain.User{})
	}

	slog.Info("running migrations...")
	if err := db.AutoMigrate(&domain.User{}, &domain.Article{}, &domain.Comment{}); err != nil {
		slog.Error("auto migrate", "err", err)
		os.Exit(1)
	}
	slog.Info("✓ migrations complete")

	if *seed {
		if err := seedData(ctx, db); err != nil {
			slog.Error("seed data", "err", err)
			os.Exit(1)
		}
		slog.Info("✓ seed data inserted")
	}
}

func openDB(driver, dsn string) (*gorm.DB, error) {
	var dial gorm.Dialector
	switch driver {
	case "postgres":
		dial = postgres.Open(dsn)
	case "mysql":
		dial = mysql.Open(dsn)
	case "sqlite":
		dial = sqlite.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := gorm.Open(dial, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying db: %w", err)
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

func seedData(ctx context.Context, db *gorm.DB) error {
	var count int64
	db.WithContext(ctx).Model(&domain.User{}).Count(&count)
	if count > 0 {
		slog.Info("seed data already exists, skipping")
		return nil
	}

	adminBio := "Astra framework maintainer"
	readerBio := "Tech enthusiast and blogger"

	users := []domain.User{
		{Username: "admin", Email: "admin@blog.local", PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", Role: "admin", Bio: &adminBio},
		{Username: "alice", Email: "alice@blog.local", PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", Role: "author", Bio: &readerBio},
	}
	if err := db.WithContext(ctx).Create(&users).Error; err != nil {
		return err
	}
	slog.Info("seeded users", "count", len(users))

	summary := "Getting started with Astra framework"
	tags := "go,aastra,web"
	articles := []domain.Article{
		{Title: "Welcome to Astra", Content: "# Welcome to Astra\n\nAstra is a Go web framework...", Summary: &summary, AuthorID: users[1].ID, Tags: &tags, Status: domain.ArticleStatusPublished},
		{Title: "Building REST APIs", Content: "# Building REST APIs\n\nLet's explore...", Summary: &summary, AuthorID: users[1].ID, Tags: &tags, Status: domain.ArticleStatusPublished},
	}
	if err := db.WithContext(ctx).Create(&articles).Error; err != nil {
		return err
	}
	slog.Info("seeded articles", "count", len(articles))

	return nil
}
