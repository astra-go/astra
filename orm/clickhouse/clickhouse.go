// Package clickhouse provides a ClickHouse-backed GORM database adapter for Astra.
//
// ClickHouse is a column-oriented database optimised for analytical workloads,
// high-throughput inserts, and time-series data. This package wraps
// gorm.io/driver/clickhouse and exposes a convenient Open function that returns
// a fully configured *gorm.DB.
//
// # Usage
//
//	import (
//	    "github.com/astra-go/astra/orm/clickhouse"
//	    "gorm.io/gorm"
//	)
//
//	db, err := clickhouse.Open(clickhouse.Config{
//	    DSN: "clickhouse://default:@localhost:9000/logs",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create table (ClickHouse-specific DDL via raw SQL)
//	db.Exec(`
//	    CREATE TABLE IF NOT EXISTS events (
//	        event_date  Date,
//	        event_time  DateTime,
//	        user_id     UInt64,
//	        action      String
//	    ) ENGINE = MergeTree()
//	    ORDER BY (event_date, user_id)
//	`)
//
//	// Batch insert
//	type Event struct {
//	    EventDate time.Time `gorm:"column:event_date"`
//	    UserID    uint64
//	    Action    string
//	}
//	db.Create(&[]Event{{...}, {...}})
//
// # DSN format
//
//	clickhouse://[user[:password]@]host[:port]/database[?param=value&...]
//
// Examples:
//
//	clickhouse://default:@localhost:9000/mydb
//	clickhouse://user:pass@ch-cluster:9000/analytics?dial_timeout=5s&read_timeout=30s
package clickhouse

import (
	"fmt"
	"time"

	gormclickhouse "gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

const (
	defaultMaxOpenConns    = 5
	defaultMaxIdleConns    = 2
	defaultConnMaxLifetime = time.Hour
)

// Config configures the ClickHouse GORM connection.
type Config struct {
	// DSN is the ClickHouse connection string.
	// Format: clickhouse://[user[:password]@]host[:port]/database[?params]
	DSN string

	// MaxOpenConns is the maximum number of open connections.
	// ClickHouse connections are heavier than MySQL/Postgres — keep this low.
	// Default: 5.
	MaxOpenConns int

	// MaxIdleConns is the maximum number of idle connections in the pool.
	// Default: 2.
	MaxIdleConns int

	// ConnMaxLifetime is the maximum lifetime of a pooled connection.
	// Default: 1h.
	ConnMaxLifetime time.Duration
}

func (c *Config) setDefaults() {
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = defaultMaxOpenConns
	}
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = defaultMaxIdleConns
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = defaultConnMaxLifetime
	}
}

// Open creates a *gorm.DB connected to ClickHouse.
//
// Additional gorm.Option values (e.g. gorm.Config{Logger: ...}) can be passed
// as extra arguments.
func Open(cfg Config, opts ...gorm.Option) (*gorm.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("clickhouse: DSN is required")
	}
	cfg.setDefaults()

	db, err := gorm.Open(gormclickhouse.Open(cfg.DSN), opts...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("clickhouse: get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	return db, nil
}
