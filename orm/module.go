package orm

import (
	"context"
	"fmt"

	"github.com/astra-go/astra"
	"gorm.io/gorm"
)

// Module wraps a *gorm.DB as an astra.Module.
//
// On Install it:
//   - Applies connection-pool settings (via SetPool)
//   - Registers orm.Middleware so every handler can call orm.DB(c)
//   - Registers an OnStop hook that closes the underlying sql.DB
//
// Usage:
//
//	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
//	if err != nil { log.Fatal(err) }
//
//	app.Register(orm.NewModule(db, orm.DefaultPoolConfig))
//
// After installation the *gorm.DB is available in handlers:
//
//	app.GET("/users", func(c *astra.Ctx) error {
//	    db := orm.DB(c)
//	    var users []User
//	    return c.JSON(200, users)
//	})
type Module struct {
	db   *gorm.DB
	pool PoolConfig
}

// NewModule creates an ORM Module for db with the given pool settings.
// Pass orm.DefaultPoolConfig if you do not need to tune the pool.
func NewModule(db *gorm.DB, pool PoolConfig) *Module {
	return &Module{db: db, pool: pool}
}

// Name implements astra.Module.
func (m *Module) Name() string { return "orm" }

// Install implements astra.Module.
func (m *Module) Install(app *astra.App) error {
	if err := SetPool(m.db, m.pool); err != nil {
		return fmt.Errorf("orm: set pool: %w", err)
	}
	app.Use(Middleware(m.db))
	app.OnStop(func(_ context.Context) error {
		sqlDB, err := m.db.DB()
		if err != nil {
			return fmt.Errorf("orm: get sql.DB for close: %w", err)
		}
		return sqlDB.Close()
	})
	return nil
}

// DB returns the underlying *gorm.DB for use outside of request handlers
// (migrations, seeding, etc.).
func (m *Module) DB() *gorm.DB { return m.db }

// Ensure *Module satisfies astra.Module at compile time.
var _ astra.Module = (*Module)(nil)
