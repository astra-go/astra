// Package orm — dialect.go provides convenience constructors for MySQL and
// PostgreSQL GORM connections, with optional connection pool configuration.
//
// # MySQL
//
//	db, err := orm.MySQL("user:pass@tcp(localhost:3306)/mydb?parseTime=True")
//
// # PostgreSQL
//
//	db, err := orm.Postgres("host=localhost user=postgres dbname=mydb sslmode=disable")
//
// # With custom pool
//
//	pool := orm.PoolConfig{MaxOpen: 50, MaxIdle: 10}
//	db, err := orm.MySQL(dsn, pool)
//
// # Switching at runtime
//
// Use the driver-agnostic Open:
//
//	db, err := orm.Open("mysql", dsn)     // or "postgres"
package orm

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DefaultGORMConfig is the base *gorm.Config applied by the dialect helpers.
// Override individual fields by building your own *gorm.Config and calling
// gorm.Open directly.
var DefaultGORMConfig = &gorm.Config{
	Logger: logger.Default.LogMode(logger.Silent),
}

// MySQL opens a MySQL / MariaDB connection via GORM and applies the optional
// pool configuration.
//
// DSN format: "user:pass@tcp(host:port)/dbname?parseTime=True&loc=Local"
func MySQL(dsn string, pool ...PoolConfig) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), DefaultGORMConfig)
	if err != nil {
		return nil, fmt.Errorf("orm: open mysql: %w", err)
	}
	return applyPool(db, pool)
}

// Postgres opens a PostgreSQL connection via GORM and applies the optional
// pool configuration.
//
// DSN formats:
//   - "host=localhost user=postgres password=secret dbname=mydb sslmode=disable"
//   - "postgresql://user:pass@localhost:5432/mydb"
func Postgres(dsn string, pool ...PoolConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), DefaultGORMConfig)
	if err != nil {
		return nil, fmt.Errorf("orm: open postgres: %w", err)
	}
	return applyPool(db, pool)
}

// Open is a driver-agnostic helper that delegates to MySQL or Postgres based
// on driver ("mysql" | "postgres" | "postgresql").
func Open(driver, dsn string, pool ...PoolConfig) (*gorm.DB, error) {
	switch driver {
	case "mysql":
		return MySQL(dsn, pool...)
	case "postgres", "postgresql":
		return Postgres(dsn, pool...)
	default:
		return nil, fmt.Errorf("orm: unsupported driver %q (use \"mysql\" or \"postgres\")", driver)
	}
}

func applyPool(db *gorm.DB, pools []PoolConfig) (*gorm.DB, error) {
	if len(pools) == 0 {
		return db, nil
	}
	if err := SetPool(db, pools[0]); err != nil {
		return nil, err
	}
	return db, nil
}
