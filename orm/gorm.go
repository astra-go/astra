// Package orm provides GORM integration for Astra.
//
// # Quick start
//
//	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
//	orm.SetPool(db, orm.DefaultPoolConfig)
//
//	app.Use(orm.Middleware(db))
//
//	app.GET("/users", func(c *astra.Ctx) error {
//	    db := orm.DB(c)
//	    var users []User
//	    db.Find(&users)
//	    return c.JSON(200, users)
//	})
//
// # Transaction propagation
//
// TxMiddleware stores the transaction in both the *astra.Ctx and the request's
// context.Context. Service-layer code can retrieve it through FromCtx, enabling
// automatic propagation without any HTTP-layer coupling:
//
//	app.POST("/orders", orm.TxMiddleware(db), createOrderHandler)
//
//	func (svc *OrderSvc) Create(ctx context.Context, item *Item) error {
//	    db := orm.FromCtx(ctx, svc.db) // picks up the tx automatically
//	    return db.Create(item).Error
//	}
//
// # Multi-database
//
//	mgr := orm.NewManager()
//	mgr.Register("primary", primaryDB)
//	mgr.Register("replica", replicaDB)
//	app.Use(mgr.Middleware()) // injects "primary" via orm.DB(c)
package orm

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/astra-go/astra"
	"github.com/astra-go/astra/contract"
	"github.com/astra-go/astra/pagination"
	"gorm.io/gorm"
)

// ─── Context-based transaction propagation ────────────────────────────────────

type txCtxKey struct{}

// WithTx stores a transaction in ctx so service-layer code can receive it via
// FromCtx without depending on *astra.Ctx.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txCtxKey{}, tx)
}

// FromCtx returns the active transaction stored in ctx by TxMiddleware, or db
// scoped to ctx when no transaction is present.
//
// This is the recommended pattern for service/repository code:
//
//	func (svc *UserSvc) Create(ctx context.Context, u *User) error {
//	    return orm.FromCtx(ctx, svc.db).Create(u).Error
//	}
func FromCtx(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return db.WithContext(ctx)
}

// ─── Connection pool ──────────────────────────────────────────────────────────

// PoolConfig controls the underlying *sql.DB connection pool.
type PoolConfig struct {
	// MaxOpen is the maximum number of open connections to the database.
	// 0 means unlimited — always set a finite value in production.
	MaxOpen int
	// MaxIdle is the maximum number of idle connections kept in the pool.
	MaxIdle int
	// MaxLifetime is the maximum time a connection may be reused.
	// 0 = no limit; recommended: 1h–4h depending on your database timeout.
	MaxLifetime time.Duration
	// MaxIdleTime is the maximum time a connection may sit idle.
	// 0 = no limit; recommended: 30m–1h.
	MaxIdleTime time.Duration
}

// DefaultPoolConfig provides conservative defaults suitable for small production
// services. Tune MaxOpen based on your database's max_connections setting.
var DefaultPoolConfig = PoolConfig{
	MaxOpen:     25,
	MaxIdle:     5,
	MaxLifetime: time.Hour,
	MaxIdleTime: 30 * time.Minute,
}

// SetPool applies PoolConfig to db. Call immediately after gorm.Open.
//
//	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
//	if err := orm.SetPool(db, orm.DefaultPoolConfig); err != nil { log.Fatal(err) }
func SetPool(db *gorm.DB, cfg PoolConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("orm: get sql.DB: %w", err)
	}
	if cfg.MaxOpen > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	}
	if cfg.MaxIdle > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	}
	if cfg.MaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.MaxLifetime)
	}
	if cfg.MaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(cfg.MaxIdleTime)
	}
	return nil
}

// ─── Middleware ───────────────────────────────────────────────────────────────

const gormDBKey = "gorm:db"

// Middleware injects a *gorm.DB scoped to the request context into every request.
// Use DB(c) or FromCtx(c.Request().Context(), db) in handlers to retrieve it.
func Middleware(db *gorm.DB) astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		// Scope the DB to the request context so GORM plugins that read from
		// context (e.g. the OTel tracing hook) work correctly.
		c.Set(gormDBKey, db.WithContext(c.Request().Context()))
		return nil
	}
}

// DB retrieves the *gorm.DB injected by Middleware or TxMiddleware.
// Returns nil when neither middleware was registered.
func DB(c *astra.Ctx) *gorm.DB {
	v, _ := c.Get(gormDBKey)
	db, _ := v.(*gorm.DB)
	return db
}

// TxMiddleware wraps each request in a database transaction and propagates it
// via both the *astra.Ctx (DB(c)) and the request's context.Context (FromCtx).
//
// Commit conditions: response status < 400, request not aborted, no panic.
// Rollback conditions: status ≥ 400, IsAborted(), or any panic (re-panics after
// rollback so the recovery middleware can log it).
func TxMiddleware(db *gorm.DB) astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		tx := db.WithContext(c.Request().Context()).Begin()
		if tx.Error != nil {
			return astra.NewHTTPError(http.StatusInternalServerError,
				"failed to begin transaction: "+tx.Error.Error())
		}

		// Propagate the transaction through both channels so that handler code
		// using orm.DB(c) and service code using orm.FromCtx(ctx, db) both see
		// the same transaction without any explicit wiring.
		c.Set(gormDBKey, tx)
		c.SetRequest(c.Request().WithContext(WithTx(c.Request().Context(), tx)))

		defer func() {
			if r := recover(); r != nil {
				_ = tx.Rollback().Error
				panic(r) // re-panic so recovery middleware can log the stack
			}
		}()

		c.Next()

		if c.IsAborted() || c.Writer().Status() >= 400 {
			if err := tx.Rollback().Error; err != nil && err != gorm.ErrInvalidTransaction {
				_ = err // not fatal; GORM internals or caller will surface it
			}
			return nil
		}

		if err := tx.Commit().Error; err != nil {
			_ = tx.Rollback().Error
			return astra.NewHTTPError(http.StatusInternalServerError,
				"transaction commit failed: "+err.Error())
		}
		return nil
	}
}

// TX is retained for backwards compatibility. Prefer TxMiddleware.
func TX(db *gorm.DB) astra.MiddlewareFunc { return TxMiddleware(db) }

// ─── Multi-DB manager ─────────────────────────────────────────────────────────

// Manager manages multiple named *gorm.DB connections and exposes middleware
// that injects the appropriate DB into each request context.
//
//	mgr := orm.NewManager()
//	mgr.Register("primary", primaryDB)
//	mgr.Register("replica", replicaDB)
//	app.Use(mgr.Middleware())                   // default DB → orm.DB(c)
//	app.Use(mgr.MiddlewareFor("replica"))       // replica  → c.MustGet("gorm:db:replica")
type Manager struct {
	mu  sync.RWMutex
	dbs map[string]*gorm.DB
}

// NewManager creates an empty Manager.
func NewManager() *Manager {
	return &Manager{dbs: make(map[string]*gorm.DB)}
}

// Register adds db under name. The name "default" is also returned by Default().
func (m *Manager) Register(name string, db *gorm.DB) {
	m.mu.Lock()
	m.dbs[name] = db
	m.mu.Unlock()
}

// Get returns the named DB and whether it was found.
func (m *Manager) Get(name string) (*gorm.DB, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	db, ok := m.dbs[name]
	return db, ok
}

// Default returns the DB registered as "default", or the first registered DB,
// or nil if the Manager is empty.
func (m *Manager) Default() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if db, ok := m.dbs["default"]; ok {
		return db
	}
	for _, db := range m.dbs {
		return db
	}
	return nil
}

// Middleware returns a middleware that injects the default DB into the context
// under the standard "gorm:db" key (retrievable via orm.DB(c)).
func (m *Manager) Middleware() astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		if db := m.Default(); db != nil {
			c.Set(gormDBKey, db.WithContext(c.Request().Context()))
		}
		return nil
	}
}

// MiddlewareFor returns a middleware that injects the named DB under the context
// key "gorm:db:<name>". Retrieve it with c.MustGet("gorm:db:replica").(*gorm.DB).
func (m *Manager) MiddlewareFor(name string) astra.MiddlewareFunc {
	key := "gorm:db:" + name
	return func(c *astra.Ctx) error {
		if db, ok := m.Get(name); ok {
			c.Set(key, db.WithContext(c.Request().Context()))
		}
		return nil
	}
}

// ─── Health check ─────────────────────────────────────────────────────────────

// Ping checks the database connection by sending a low-cost PING query.
func Ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("orm: get sql.DB: %w", err)
	}
	return sqlDB.Ping()
}

// HealthHandler returns an Astra handler that reports database health as JSON.
func HealthHandler(db *gorm.DB) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		if err := Ping(db); err != nil {
			return c.JSON(http.StatusServiceUnavailable, astra.Map{
				"status": "unhealthy",
				"error":  err.Error(),
			})
		}
		return c.JSON(http.StatusOK, astra.Map{"status": "healthy"})
	}
}

// ─── Pagination ───────────────────────────────────────────────────────────────

// Page is an alias for contract.Page so existing code using orm.Page continues
// to compile without modification.
type Page = contract.Page

// DefaultPageSize is used when no page_size query param is specified.
const DefaultPageSize = 20

// MaxPageSize is the upper bound on page_size to prevent resource exhaustion.
const MaxPageSize = 100

// ParsePage reads ?page=1&page_size=20 from the request query params.
// Both values are clamped to sane ranges.
func ParsePage(c *astra.Ctx) Page {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(DefaultPageSize)))

	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	return Page{
		PageNum:  pageNum,
		PageSize: pageSize,
		Offset:   (pageNum - 1) * pageSize,
	}
}

// Paginate returns a GORM scope that applies LIMIT and OFFSET from p.
func Paginate(p Page) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(p.Offset).Limit(p.PageSize)
	}
}

// PageResponse is a standard paginated JSON response envelope.
type PageResponse struct {
	Data     any   `json:"data"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int64 `json:"pages"`
}

// NewPageResponse builds a PageResponse from query results.
func NewPageResponse(p Page, total int64, data any) PageResponse {
	pages := total / int64(p.PageSize)
	if total%int64(p.PageSize) != 0 {
		pages++
	}
	return PageResponse{
		Data:     data,
		Total:    total,
		Page:     p.PageNum,
		PageSize: p.PageSize,
		Pages:    pages,
	}
}

// GORMScope converts a [pagination.Request] into a GORM scope that applies
// LIMIT and OFFSET. Use this adapter instead of the removed req.GORMScope() method.
//
//	db.Scopes(orm.GORMScope(req)).Find(&users)
func GORMScope(req pagination.Request) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Offset(req.Offset()).Limit(req.Size)
	}
}

// GORMTenantScope returns a GORM scope that appends WHERE tenant_id = ?
// for the given tenant ID. Obtain the ID from the context via middleware.TenantID(c).
//
//	db.Scopes(orm.GORMTenantScope(middleware.TenantID(c))).Find(&orders)
func GORMTenantScope(tenantID string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if tenantID == "" {
			return db
		}
		return db.Where("tenant_id = ?", tenantID)
	}
}

// ─── Repository ───────────────────────────────────────────────────────────────

// Repository is a generic base repository for common CRUD operations.
// It implements contract.Repository[T] so business code can depend on the
// interface and be tested with a lightweight mock without a real database.
//
// Embed it in domain-specific repository structs to inherit the full API:
//
//	type UserRepo struct{ *orm.Repository[User] }
//
//	func NewUserRepo(db *gorm.DB) *UserRepo {
//	    return &UserRepo{Repository: orm.NewRepository[User](db)}
//	}
//
// Transaction propagation is automatic: pass the txCtx from TxRunner.RunTx
// (or TxMiddleware) as the first argument and orm.FromCtx will pick up the
// active transaction — no explicit plumbing required.
//
//	func (r *UserRepo) Create(ctx context.Context, u *User) error {
//	    return r.Repository.Create(ctx, u)  // transaction-aware
//	}
type Repository[T any] struct {
	db *gorm.DB
}

// Compile-time assertion: *Repository[T] must implement contract.Repository[T].
var _ contract.Repository[any] = (*Repository[any])(nil)

// NewRepository creates a new generic repository.
func NewRepository[T any](db *gorm.DB) *Repository[T] {
	return &Repository[T]{db: db}
}

// WithCtx returns a shallow copy of the repository whose underlying DB is
// scoped to ctx. If ctx holds an active transaction (set by TxMiddleware or
// TxRunner.RunTx), that transaction is used automatically.
func (r *Repository[T]) WithCtx(ctx context.Context) *Repository[T] {
	return &Repository[T]{db: FromCtx(ctx, r.db)}
}

// DB returns the underlying *gorm.DB for building custom queries.
func (r *Repository[T]) DB() *gorm.DB { return r.db }

// Scopes returns a shallow copy of the repository with the given GORM scopes
// pre-applied. Useful for reusable query filters.
//
//	repo.Scopes(orm.Paginate(page), activeOnly).FindAll(ctx, nil)
func (r *Repository[T]) Scopes(fns ...func(*gorm.DB) *gorm.DB) *Repository[T] {
	return &Repository[T]{db: r.db.Scopes(fns...)}
}

// Create inserts entity and returns any error.
// The active transaction in ctx (if any) is used automatically.
func (r *Repository[T]) Create(ctx context.Context, entity *T) error {
	return FromCtx(ctx, r.db).Create(entity).Error
}

// FindByID retrieves a record by primary key.
// Returns gorm.ErrRecordNotFound when the record does not exist.
func (r *Repository[T]) FindByID(ctx context.Context, id any) (*T, error) {
	var entity T
	if err := FromCtx(ctx, r.db).First(&entity, id).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// FindAll retrieves all records with optional pagination.
// Pass p to apply LIMIT/OFFSET; pass nil to fetch all rows (use with care on
// large tables). Returns (records, totalCount, error).
func (r *Repository[T]) FindAll(ctx context.Context, p *Page) ([]T, int64, error) {
	var entities []T
	var total int64

	db := FromCtx(ctx, r.db)
	q := db.Model(new(T))
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if p != nil {
		q = q.Scopes(Paginate(*p))
	}
	if err := q.Find(&entities).Error; err != nil {
		return nil, 0, err
	}
	return entities, total, nil
}

// FindWhere retrieves records matching the given condition.
// query may be a string ("status = ?"), a map, or a struct.
func (r *Repository[T]) FindWhere(ctx context.Context, query any, args ...any) ([]T, error) {
	var entities []T
	if err := FromCtx(ctx, r.db).Where(query, args...).Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// First returns the first record matching query ordered by primary key.
// Returns gorm.ErrRecordNotFound when no record matches.
func (r *Repository[T]) First(ctx context.Context, query any, args ...any) (*T, error) {
	var entity T
	if err := FromCtx(ctx, r.db).Where(query, args...).First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// Count returns the number of records matching query.
func (r *Repository[T]) Count(ctx context.Context, query any, args ...any) (int64, error) {
	var count int64
	if err := FromCtx(ctx, r.db).Model(new(T)).Where(query, args...).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Update saves all non-zero fields of entity (GORM Save semantics).
func (r *Repository[T]) Update(ctx context.Context, entity *T) error {
	return FromCtx(ctx, r.db).Save(entity).Error
}

// Updates applies a partial update to the record with the given primary key.
// values can be a map[string]any or a struct (only non-zero fields are updated).
func (r *Repository[T]) Updates(ctx context.Context, id any, values any) error {
	var entity T
	return FromCtx(ctx, r.db).Model(&entity).Where("id = ?", id).Updates(values).Error
}

// Delete removes the record with the given primary key.
func (r *Repository[T]) Delete(ctx context.Context, id any) error {
	var entity T
	return FromCtx(ctx, r.db).Delete(&entity, id).Error
}

// DeleteWhere removes all records matching the given condition.
// Use with caution: a bug in the query can delete unintended rows.
func (r *Repository[T]) DeleteWhere(ctx context.Context, query any, args ...any) error {
	var entity T
	return FromCtx(ctx, r.db).Where(query, args...).Delete(&entity).Error
}
