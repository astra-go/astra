// Package orm — tenant.go provides multi-tenant database isolation for GORM.
//
// # Isolation strategies
//
// Schema-per-tenant (PostgreSQL):
//
//	mgr, _ := orm.NewTenantDBManager(orm.TenantManagerConfig{Mode: orm.TenantModeSchema})
//	mgr.RegisterSchema("acme", db, "acme_schema")
//	mgr.RegisterSchema("globex", db, "globex_schema")
//
// Database-per-tenant:
//
//	mgr, _ := orm.NewTenantDBManager(orm.TenantManagerConfig{Mode: orm.TenantModeDatabase})
//	mgr.Register("acme", dbAcme)
//	mgr.Register("globex", dbGlobex)
//
// Shared DB + row-level security (discriminator column):
//
//	mgr, _ := orm.NewTenantDBManager(orm.TenantManagerConfig{
//	    Mode:     orm.TenantModeShared,
//	    TenantKey: "tenant_id",
//	})
//	mgr.Register("acme", sharedDB)   // RLS plugin auto-scopes all queries
//	mgr.Register("globex", sharedDB)
//
// # Middleware wiring
//
//	app.Use(middleware.Tenant(tenantConfig))
//	app.Use(mgr.Middleware())
//
//	// Handler — tenant DB auto-injected, RLS plugin scopes queries:
//	app.GET("/orders", func(c *astra.Ctx) error {
//	    db := orm.TenantDB(c)   // tenant-specific *gorm.DB
//	    orders, _ := orm.NewRepository[Order](db).FindAll(ctx, nil)
//	    return c.JSON(200, orders)  // tenant_id filter applied automatically
//	})
//
// # Row-level security (TenantModeShared)
//
// The RLS plugin intercepts GORM callbacks:
//   - BeforeFind:       injects WHERE tenant_id = ? on SELECT
//   - BeforeCreate:     auto-sets tenant_id column on INSERT
//   - BeforeUpdate:     adds tenant_id = ? to UPDATE WHERE clause (blocks cross-tenant writes)
//   - BeforeDelete:     adds tenant_id = ? to DELETE WHERE clause (blocks cross-tenant deletes)
//
// The plugin reads tenant ID from the request context (set by Middleware).
// No tenant in context → plugin is a no-op, preserving normal DB access.
package orm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/astra-go/astra"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TenantMode controls how tenant isolation is applied.
type TenantMode int

const (
	// TenantModeSchema uses per-tenant PostgreSQL search_path (or equivalent).
	// DB routing is by tenant → DB/schema mapping; no RLS plugin is applied.
	TenantModeSchema TenantMode = iota

	// TenantModeShared uses a shared database with a tenant_id discriminator
	// column. The RLS plugin is applied automatically on all tenant-registered DBs.
	TenantModeShared

	// TenantModeDatabase uses a completely separate database per tenant.
	// DB routing is purely by tenant → DB mapping; no RLS plugin needed.
	TenantModeDatabase
)

func (m TenantMode) String() string {
	switch m {
	case TenantModeSchema:
		return "schema"
	case TenantModeShared:
		return "shared"
	case TenantModeDatabase:
		return "database"
	default:
		return "unknown"
	}
}

// TenantManagerConfig configures TenantDBManager.
type TenantManagerConfig struct {
	// Mode determines the isolation strategy. Default: TenantModeShared.
	Mode TenantMode

	// TenantKey is the column name for row-level security in TenantModeShared.
	// Default: "tenant_id". All tenant-scoped tables must have this column.
	TenantKey string

	// SkipSystemTables when true causes the RLS plugin to skip tables that do not
	// have the TenantKey column. Default: true.
	SkipSystemTables bool

	// SkipTables is a set of table names the RLS plugin should never touch.
	// Useful for truly shared tables (lookup tables, migrations, etc.).
	SkipTables []string

	// SkipAutoSetColumns lists columns that the RLS plugin should not auto-populate
	// on INSERT (e.g. primary keys managed by the DB, trigger-populated columns).
	SkipAutoSetColumns []string
}

const defaultTenantKey = "tenant_id"

// ─── TenantDBManager ─────────────────────────────────────────────────────────

// TenantDBManager manages per-tenant *gorm.DB connections and applies
// row-level security based on the configured TenantMode.
//
// The zero value is not usable; construct with NewTenantDBManager.
type TenantDBManager struct {
	cfg TenantManagerConfig

	mu      sync.RWMutex
	tenants map[string]*gorm.DB // tenantID → DB

	// schemaName records the search_path for each tenant (TenantModeSchema)
	schemas map[string]string

	// The default/fallback DB (used for unknown tenants and system operations)
	defaultDB *gorm.DB

	// RLS plugin — registered on each DB registered in TenantModeShared
	rls *rlsPlugin

	// Tracks DBs that have already had callbacks registered
	muReg   sync.Mutex
	registered map[string]bool
}

// NewTenantDBManager validates cfg and returns a ready-to-use TenantDBManager.
func NewTenantDBManager(cfg TenantManagerConfig) (*TenantDBManager, error) {
	if cfg.TenantKey == "" {
		cfg.TenantKey = defaultTenantKey
	}
	skipTables := make(map[string]bool)
	for _, t := range cfg.SkipTables {
		skipTables[t] = true
	}
	skipAuto := make(map[string]bool)
	for _, c := range cfg.SkipAutoSetColumns {
		skipAuto[c] = true
	}

	m := &TenantDBManager{
		cfg:         cfg,
		tenants:     make(map[string]*gorm.DB),
		schemas:     make(map[string]string),
		rls: &rlsPlugin{
			tenantKey:        cfg.TenantKey,
			skipSystemTables: cfg.SkipSystemTables,
			skipTables:       skipTables,
			skipAutoSet:      skipAuto,
		},
		registered: make(map[string]bool),
	}
	return m, nil
}

// Register maps tenantID to db. In TenantModeShared the RLS plugin is registered
// on db (once per unique *gorm.DB pointer). In TenantModeDatabase and TenantModeSchema
// no plugin is registered.
func (m *TenantDBManager) Register(tenantID string, db *gorm.DB) error {
	if tenantID == "" {
		return fmt.Errorf("orm/tenant: Register: tenantID cannot be empty")
	}
	if db == nil {
		return fmt.Errorf("orm/tenant: Register: db cannot be nil for tenant %q", tenantID)
	}

	m.mu.Lock()
	m.tenants[tenantID] = db
	if m.cfg.Mode == TenantModeShared {
		m.registerRLSOnDB(db)
	}
	m.mu.Unlock()

	return nil
}

// RegisterSchema maps tenantID to db and records schemaName as the PostgreSQL
// search_path to use for this tenant. Use in TenantModeSchema when multiple
// tenants share a single *gorm.DB with different schemas:
//
//	db, _ := orm.Postgres("host=localhost dbname=app sslmode=disable")
//	mgr.RegisterSchema("acme", db, "acme_schema")
//	mgr.RegisterSchema("globex", db, "globex_schema")
//
// For TenantModeDatabase use Register instead.
func (m *TenantDBManager) RegisterSchema(tenantID string, db *gorm.DB, schemaName string) error {
	if err := m.Register(tenantID, db); err != nil {
		return err
	}
	m.mu.Lock()
	m.schemas[tenantID] = schemaName
	m.mu.Unlock()
	return nil
}

// RegisterDefault sets the fallback DB used when no tenant is matched.
// Used for unauthenticated routes and system tables.
func (m *TenantDBManager) RegisterDefault(db *gorm.DB) {
	m.mu.Lock()
	m.defaultDB = db
	m.mu.Unlock()
}

// DB returns the *gorm.DB registered for tenantID.
// Returns nil if tenantID is not registered and no default DB is set.
func (m *TenantDBManager) DB(tenantID string) *gorm.DB {
	m.mu.RLock()
	db, ok := m.tenants[tenantID]
	if ok {
		m.mu.RUnlock()
		return db
	}
	m.mu.RUnlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultDB
}

// DBForSchema returns the *gorm.DB for tenantID with the PostgreSQL search_path
// set to tenantID's registered schema (TenantModeSchema).
// Falls back to the registered DB without schema change if schemaName is empty
// or the DB is not registered.
func (m *TenantDBManager) DBForSchema(ctx context.Context, tenantID string) *gorm.DB {
	db := m.DB(tenantID)
	if db == nil {
		return nil
	}

	m.mu.RLock()
	schemaName := m.schemas[tenantID]
	m.mu.RUnlock()

	if schemaName == "" {
		return db.WithContext(ctx)
	}

	// SET LOCAL applies only to the current transaction; safe to use on
	// a fresh db.WithContext clone (no side effects on the connection pool).
	tx := db.WithContext(ctx).Session(&gorm.Session{}).
		Exec(fmt.Sprintf("SET LOCAL search_path TO %s", schemaName))
	if tx.Error != nil {
		return db.WithContext(ctx)
	}
	return tx
}

// Middleware returns an Astra middleware that:
//   - Reads tenant ID from c (set by middleware.Tenant())
//   - Resolves tenant → *gorm.DB via m.DB()
//   - Stores the scoped DB in c (via c.Set) and in ctx (via context)
//   - Propagates tenant ID into ctx via WithTenantID
//
// Must be registered AFTER middleware.Tenant().
//
//	app.Use(middleware.Tenant(...))
//	app.Use(mgr.Middleware())
func (m *TenantDBManager) Middleware() astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		tenantID := TenantIDFromCtxGORM(c)

		db := m.DB(tenantID)
		if db == nil {
			db = m.defaultDB
		}

		if db != nil {
			ctx := c.Request().Context()
			ctx = WithTenantID(ctx, tenantID)
			ctx = context.WithValue(ctx, _tenantDBValueKey{}, db)
			c.SetRequest(c.Request().WithContext(ctx))
			c.Set(tenantDBContextKey, db)
		}

		return nil
	}
}

// TenantIDFromCtxGORM reads the tenant ID stored in c by middleware.Tenant().
func TenantIDFromCtxGORM(c *astra.Ctx) string {
	v, ok := c.Get(defaultTenantContextKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// TxMiddleware is like TxMiddleware in gorm.go but tenant-aware: it begins a
// transaction on the tenant's DB and propagates BOTH the transaction and the
// tenant ID through the context.
//
// Commit: response status < 400, not aborted, no panic.
// Rollback: status ≥ 400, IsAborted(), or panic.
//
// Must be registered AFTER middleware.Tenant().
//
//	app.POST("/orders", tenantMgr.TxMiddleware(), createOrderHandler)
func (m *TenantDBManager) TxMiddleware() astra.MiddlewareFunc {
	return func(c *astra.Ctx) error {
		tenantID := TenantIDFromCtxGORM(c)

		db := m.DB(tenantID)
		if db == nil {
			db = m.defaultDB
		}

		if db == nil {
			c.Next()
			return nil
		}

		tx := db.WithContext(c.Request().Context()).Begin()
		if tx.Error != nil {
			return astra.NewHTTPError(500, "failed to begin transaction: "+tx.Error.Error())
		}

		txCtx := WithTx(c.Request().Context(), tx)
		txCtx = WithTenantID(txCtx, tenantID)
		txCtx = context.WithValue(txCtx, _tenantDBValueKey{}, tx)

		c.Set(gormDBKey, tx)
		c.Set(tenantDBContextKey, tx)
		c.SetRequest(c.Request().WithContext(txCtx))

		defer func() {
			if r := recover(); r != nil {
				_ = tx.Rollback().Error
				panic(r)
			}
		}()

		c.Next()

		if c.IsAborted() || c.Writer().Status() >= 400 {
			if err := tx.Rollback().Error; err != nil && err != gorm.ErrInvalidTransaction {
				_ = err
			}
			return nil
		}

		if err := tx.Commit().Error; err != nil {
			_ = tx.Rollback().Error
			return astra.NewHTTPError(500, "transaction commit failed: "+err.Error())
		}
		return nil
	}
}

// ─── Context keys ────────────────────────────────────────────────────────────

// _tenantDBValueKey is the context value key for the tenant-scoped *gorm.DB.
// It is an unexported struct to prevent collisions.
type _tenantDBValueKey struct{}

// defaultTenantContextKey is the context key used by middleware.Tenant.
// Exported here so TenantDBManager.Middleware can read it without importing
// the middleware package (avoiding circular dependency risk).
const defaultTenantContextKey = "tenant_id"

// WithTenantID stores tenantID in ctx so it can be read by helper functions.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, _tenantIDValueKey{}, tenantID)
}

type _tenantIDValueKey struct{}

// TenantIDFromCtx retrieves the tenant ID stored by WithTenantID or Middleware.
func TenantIDFromCtx(ctx context.Context) string {
	v := ctx.Value(_tenantIDValueKey{})
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ─── Public DB retrieval helpers ─────────────────────────────────────────────

// TenantDB retrieves the tenant-scoped *gorm.DB stored in c by Middleware.
// Returns nil when the middleware was not registered.
func TenantDB(c *astra.Ctx) *gorm.DB {
	v, _ := c.Get(tenantDBContextKey)
	db, _ := v.(*gorm.DB)
	return db
}

// TenantDBFromCtx retrieves the tenant-scoped *gorm.DB from ctx.
// Returns nil when not set.
func TenantDBFromCtx(ctx context.Context) *gorm.DB {
	v := ctx.Value(_tenantDBValueKey{})
	db, _ := v.(*gorm.DB)
	return db
}

// FromTenantCtx returns the tenant-scoped *gorm.DB from ctx, falling back to db.
// This is the recommended pattern in service/repository code:
//
//	repo := orm.NewRepository[Order](orm.FromTenantCtx(ctx, svc.baseDB))
func FromTenantCtx(ctx context.Context, db *gorm.DB) *gorm.DB {
	if d := TenantDBFromCtx(ctx); d != nil {
		return d
	}
	return db.WithContext(ctx)
}

// WithTenantDB stores db (a tenant-scoped *gorm.DB) in ctx.
// Use this to propagate the tenant-aware DB to downstream service calls:
//
//	ctx := orm.WithTenantDB(ctx, tenantMgr.DB(tenantID))
//	_ = myService.Do(ctx, ...)
func WithTenantDB(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, _tenantDBValueKey{}, db)
}

// context key used in *astra.Ctx (string, not struct, for c.Get compatibility)
const tenantDBContextKey = "orm:tenant_db"

// ─── GORM RLS Plugin ─────────────────────────────────────────────────────────

// rlsPlugin implements gorm.Plugin for TenantModeShared.
// It registers BeforeFind/BeforeCreate/BeforeUpdate/BeforeDelete callbacks
// that enforce row-level security by injecting tenant scoping.
type rlsPlugin struct {
	tenantKey        string // e.g. "tenant_id"
	skipSystemTables bool
	skipTables       map[string]bool
	skipAutoSet      map[string]bool
}

var _ gorm.Plugin = (*rlsPlugin)(nil) // compile-time guard: rlsPlugin must implement gorm.Plugin

func (p *rlsPlugin) Name() string { return "astra_rls_plugin" }

// tableName returns the table name from db.Statement, stripping any schema prefix.
func (p *rlsPlugin) tableName(db *gorm.DB) string {
	if db.Statement == nil {
		return ""
	}
	t := db.Statement.Table
	if idx := strings.LastIndexByte(t, '.'); idx >= 0 {
		t = t[idx+1:]
	}
	return t
}

// hasTenantColumn returns true if the table schema has the tenant_key column.
func (p *rlsPlugin) hasTenantColumn(db *gorm.DB) bool {
	if db.Statement == nil || db.Statement.Schema == nil {
		return false
	}
	return db.Statement.Schema.LookUpField(p.tenantKey) != nil
}

// tenantID returns the tenant ID from db.Request.Context() (propagated by Middleware),
// or "" when no tenant is in scope.
func (p *rlsPlugin) tenantID(db *gorm.DB) string {
	ctx := db.Statement.Context
	if ctx == nil {
		return ""
	}
	v := ctx.Value(_tenantIDValueKey{})
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// rlsBeforeFind injects WHERE tenant_id = ? on SELECT unless already present
// or the table is explicitly skipped.
func (p *rlsPlugin) rlsBeforeFind(db *gorm.DB) {
	// Unscoped() calls bypass RLS — caller explicitly opts out
	if db.Statement.Unscoped {
		return
	}

	table := p.tableName(db)
	if table == "" || p.skipTables[table] {
		return
	}
	if p.skipSystemTables && !p.hasTenantColumn(db) {
		return
	}

	tid := p.tenantID(db)
	if tid == "" {
		return // no tenant in context — plugin is a no-op
	}

	// Append the tenant filter to the WHERE clause.
	// GORM's AddClause merges with existing WHERE clauses gracefully.
	db.Statement.AddClause(clause.Where{
		Exprs: []clause.Expression{
			clause.Expr{SQL: p.tenantKey + " = ?", Vars: []any{tid}},
		},
	})
}

func (p *rlsPlugin) rlsBeforeCreate(db *gorm.DB) {
	table := p.tableName(db)
	if table == "" || p.skipTables[table] {
		return
	}
	if p.skipSystemTables && !p.hasTenantColumn(db) {
		return
	}
	if p.skipAutoSet[p.tenantKey] {
		return
	}

	tid := p.tenantID(db)
	if tid == "" {
		return
	}

	// Use SetColumn so GORM respects it in the INSERT VALUES clause.
	// GORM checks if the field is zero before overwriting, so this only
	// sets the column when the caller hasn't explicitly set it.
	db.Statement.SetColumn(p.tenantKey, tid)
}

func (p *rlsPlugin) rlsBeforeUpdate(db *gorm.DB) {
	if db.Statement.Unscoped {
		return
	}
	table := p.tableName(db)
	if table == "" || p.skipTables[table] {
		return
	}
	if p.skipSystemTables && !p.hasTenantColumn(db) {
		return
	}

	tid := p.tenantID(db)
	if tid == "" {
		return
	}

	// Append tenant_id = ? to UPDATE WHERE — only records owned by this
	// tenant can be modified. This blocks cross-tenant updates.
	db.Statement.AddClause(clause.Where{
		Exprs: []clause.Expression{
			clause.Expr{SQL: p.tenantKey + " = ?", Vars: []any{tid}},
		},
	})
}

func (p *rlsPlugin) rlsBeforeDelete(db *gorm.DB) {
	if db.Statement.Unscoped {
		return
	}
	table := p.tableName(db)
	if table == "" || p.skipTables[table] {
		return
	}
	if p.skipSystemTables && !p.hasTenantColumn(db) {
		return
	}

	tid := p.tenantID(db)
	if tid == "" {
		return
	}

	// Append tenant_id = ? to DELETE WHERE — prevents cross-tenant deletions.
	db.Statement.AddClause(clause.Where{
		Exprs: []clause.Expression{
			clause.Expr{SQL: p.tenantKey + " = ?", Vars: []any{tid}},
		},
	})
}

// Initialize implements gorm.Plugin. It is called by db.Use(p).
// We register all RLS callbacks here, before any query is executed.
//
// GORM v1.25 uses *gorm.DB (not *gorm.Scope) in callbacks.
// Callbacks are registered at specific points in the GORM callback chain:
//   - Query/Row callbacks: run before the SQL builder so tenant filters are in WHERE
//   - Create callbacks:     run before INSERT so tenant_id is populated
//   - Update/Delete:        run before the SQL builder so tenant guard is in WHERE
func (p *rlsPlugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()

	// ── Query (SELECT) ─────────────────────────────────────────────────────────
	// Register before gorm:query so the tenant filter is part of WHERE when
	// BuildQuerySQL generates the SQL.
	cb.Query().Before("gorm:query").
		Register("astra_rls:before_find", p.rlsBeforeFind)

	// Row() uses the same query path.
	cb.Row().Before("gorm:row").
		Register("astra_rls:before_find_row", p.rlsBeforeFind)

	// ── Create (INSERT) ─────────────────────────────────────────────────────────
	// Register before gorm:before_create (which resolves associations) so
	// SetColumn populates tenant_id before INSERT VALUES are built.
	cb.Create().Before("gorm:before_create").
		Register("astra_rls:before_create", p.rlsBeforeCreate)

	// ── Update ─────────────────────────────────────────────────────────────────
	cb.Update().Before("gorm:before_update").
		Register("astra_rls:before_update", p.rlsBeforeUpdate)

	// ── Delete ─────────────────────────────────────────────────────────────────
	cb.Delete().Before("gorm:before_delete").
		Register("astra_rls:before_delete", p.rlsBeforeDelete)

	return nil
}

// registerRLSOnDB registers the RLS callbacks on db exactly once (tracked by
// the pointer address of db to handle the case where the same *gorm.DB is
// registered for multiple tenants).
func (m *TenantDBManager) registerRLSOnDB(db *gorm.DB) {
	m.muReg.Lock()
	key := fmt.Sprintf("%p", db)
	if m.registered[key] {
		m.muReg.Unlock()
		return
	}
	m.registered[key] = true
	m.muReg.Unlock()

	// Use the plugin registration mechanism so GORM owns the lifecycle.
	_ = db.Use(m.rls)
}

// ─── TenantScope GORM helper ──────────────────────────────────────────────────

// TenantGORMCallback returns a GORM scope that injects WHERE tenant_id = ?.
// This is the functional equivalent of GORMTenantScope but reads from ctx:
//
//	repo.Scopes(orm.TenantGORMCallback(ctx)).FindAll(ctx, nil)
//
// Prefer GORMTenantScope(tenantID) when you already have the tenant ID as a string.
func TenantGORMCallback(ctx context.Context) func(*gorm.DB) *gorm.DB {
	tid := TenantIDFromCtx(ctx)
	return GORMTenantScope(tid)
}

// ─── TenantRepository ─────────────────────────────────────────────────────────

// TenantRepository is a generic, tenant-scoped repository.
// It satisfies contract.Repository[T] and automatically applies tenant scoping.
//
// All queries are filtered to the current tenant via GORMTenantScope.
// Transaction propagation is automatic: pass the txCtx from TxMiddleware as the
// first argument and FromTenantCtx will pick up the active transaction.
//
//	type ProductRepo struct{ *TenantRepository[domain.Product] }
//
//	func NewProductRepo(mgr *orm.TenantDBManager, tenantID string) *ProductRepo {
//	    return &ProductRepo{orm.NewTenantRepository[domain.Product](mgr, tenantID)}
//	}
//
// In service layer:
//
//	repo := orm.NewTenantRepository[Order](mgr, "")
//	orders, total, err := repo.FindAll(c.Request().Context(), nil)
type TenantRepository[T any] struct {
	mgr      *TenantDBManager
	tenantID string
	db       *gorm.DB
}

// NewTenantRepository creates a TenantRepository backed by mgr.
// If tenantID is non-empty it is used as the fixed default; otherwise the
// tenant is read from ctx at query time (supports dynamic per-request tenancy).
func NewTenantRepository[T any](mgr *TenantDBManager, tenantID string) *TenantRepository[T] {
	var baseDB *gorm.DB
	if tenantID != "" {
		baseDB = mgr.DB(tenantID)
	}
	return &TenantRepository[T]{
		mgr:      mgr,
		tenantID: tenantID,
		db:       baseDB,
	}
}

// WithCtx returns a shallow copy scoped to ctx (picks up active tx + dynamic tenant).
func (r *TenantRepository[T]) WithCtx(ctx context.Context) *TenantRepository[T] {
	tid := TenantIDFromCtx(ctx)
	if tid == "" {
		tid = r.tenantID
	}
	db := FromTenantCtx(ctx, r.db)
	if db == nil {
		db = r.db
	}
	return &TenantRepository[T]{
		mgr:      r.mgr,
		tenantID: tid,
		db:       db,
	}
}

// DB returns the underlying *gorm.DB for building custom queries.
func (r *TenantRepository[T]) DB() *gorm.DB { return r.db }

// Scopes adds GORM scopes on top of the tenant-scoped DB.
func (r *TenantRepository[T]) Scopes(fns ...func(*gorm.DB) *gorm.DB) *TenantRepository[T] {
	return &TenantRepository[T]{
		mgr:      r.mgr,
		tenantID: r.tenantID,
		db:       r.db.Scopes(fns...),
	}
}

// tenantDB returns the effective *gorm.DB for the current call, honouring
// active transactions and dynamic tenant context.
func (r *TenantRepository[T]) tenantDB(ctx context.Context) *gorm.DB {
	db := FromTenantCtx(ctx, r.db)
	if db == nil {
		db = r.db
	}
	return db
}

// tenantScope returns a GORM scope for the current tenant.
func (r *TenantRepository[T]) tenantScope(ctx context.Context) func(*gorm.DB) *gorm.DB {
	tid := TenantIDFromCtx(ctx)
	if tid == "" {
		tid = r.tenantID
	}
	return GORMTenantScope(tid)
}

// Create inserts entity. The RLS plugin sets tenant_id on BeforeCreate.
func (r *TenantRepository[T]) Create(ctx context.Context, entity *T) error {
	db := r.tenantDB(ctx).Scopes(r.tenantScope(ctx))
	return NewRepository[T](db).Create(ctx, entity)
}

// FindByID retrieves a record by primary key, scoped to the current tenant.
func (r *TenantRepository[T]) FindByID(ctx context.Context, id any) (*T, error) {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).FindByID(ctx, id)
}

// FindAll retrieves all records for the tenant with optional pagination.
func (r *TenantRepository[T]) FindAll(ctx context.Context, p *Page) ([]T, int64, error) {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).FindAll(ctx, p)
}

// FindWhere retrieves records matching query, scoped to the tenant.
func (r *TenantRepository[T]) FindWhere(ctx context.Context, query any, args ...any) ([]T, error) {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).FindWhere(ctx, query, args...)
}

// First returns the first record matching query, scoped to the tenant.
func (r *TenantRepository[T]) First(ctx context.Context, query any, args ...any) (*T, error) {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).First(ctx, query, args...)
}

// Count returns the number of records for the tenant.
func (r *TenantRepository[T]) Count(ctx context.Context, query any, args ...any) (int64, error) {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).Count(ctx, query, args...)
}

// Update saves all non-zero fields. The RLS plugin guards BeforeUpdate.
func (r *TenantRepository[T]) Update(ctx context.Context, entity *T) error {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).Update(ctx, entity)
}

// Updates applies a partial update. The RLS plugin guards BeforeUpdate.
func (r *TenantRepository[T]) Updates(ctx context.Context, id any, values any) error {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).Updates(ctx, id, values)
}

// Delete removes a record. The RLS plugin guards BeforeDelete.
func (r *TenantRepository[T]) Delete(ctx context.Context, id any) error {
	return NewRepository[T](r.tenantDB(ctx).Scopes(r.tenantScope(ctx))).Delete(ctx, id)
}

// RunTx executes fn inside a transaction on the tenant's DB. The tenant
// context is propagated into the transaction so the RLS plugin remains active.
//
//	orderRepo := orm.NewTenantRepository[Order](mgr, "")
//	_ = orderRepo.RunTx(ctx, func(r *orm.TenantRepository[Order]) error {
//	    return r.Create(ctx, &order)
//	})
func (r *TenantRepository[T]) RunTx(ctx context.Context, fn func(repo *TenantRepository[T]) error) error {
	db := r.tenantDB(ctx)
	tid := TenantIDFromCtx(ctx)
	if tid == "" {
		tid = r.tenantID
	}
	return RunTx(ctx, db, func(tx *gorm.DB) error {
		// Propagate tenant context into the transaction so RLS plugin stays active
		txCtx := WithTenantID(ctx, tid)
		txCtx = WithTenantDB(txCtx, tx)
		return fn(&TenantRepository[T]{
			mgr:      r.mgr,
			tenantID: tid,
			db:       tx,
		})
	})
}
